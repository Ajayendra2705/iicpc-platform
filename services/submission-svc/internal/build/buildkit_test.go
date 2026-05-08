package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyFromSourceURI(t *testing.T) {
	cases := []struct {
		name    string
		uri     string
		want    string
		wantErr bool
	}{
		{"happy", "s3://submissions/team1/abc/source.tar.gz", "team1/abc/source.tar.gz", false},
		{"deep", "s3://b/x/y/z.tgz", "x/y/z.tgz", false},
		{"non-s3", "https://example.com/file", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := keyFromSourceURI(tc.uri)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short: got %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcde" {
		t.Errorf("long: got %q", got)
	}
}

type fakeStorage struct {
	archive []byte
}

func (f *fakeStorage) Put(_ context.Context, _ string, _ io.Reader, _ int64, _ string) (string, error) {
	return "s3://test/key", nil
}

func (f *fakeStorage) Get(_ context.Context, _ string) (io.ReadCloser, int64, error) {
	return io.NopCloser(bytes.NewReader(f.archive)), int64(len(f.archive)), nil
}

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDownloadAndExtractHappy(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{
		"go.mod":  []byte("module x\n"),
		"main.go": []byte("package main\nfunc main(){}\n"),
	})

	bk := NewBuildKit(BuildKitConfig{
		Storage: &fakeStorage{archive: archive},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	dst := t.TempDir()
	if err := bk.downloadAndExtract(context.Background(), "s3://b/key", dst); err != nil {
		t.Fatalf("extract: %v", err)
	}
	for _, f := range []string{"go.mod", "main.go"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestDownloadAndExtractRejectsTooLarge(t *testing.T) {
	big := bytes.Repeat([]byte("A"), 200)
	archive := makeTarGz(t, map[string][]byte{"big.txt": big})

	bk := NewBuildKit(BuildKitConfig{
		Storage:           &fakeStorage{archive: archive},
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxExtractedBytes: 50,
		MaxFileBytes:      1000,
	})

	err := bk.downloadAndExtract(context.Background(), "s3://b/key", t.TempDir())
	if err == nil {
		t.Fatal("expected error for oversized archive")
	}
	// LimitedReader cuts the tar mid-stream once the budget is hit. We accept
	// either our explicit "exceeds max extracted size" message or the tar
	// reader's "unexpected EOF" — both prove the cap was effective.
	msg := err.Error()
	if !strings.Contains(msg, "exceeds max extracted size") &&
		!strings.Contains(msg, "unexpected EOF") &&
		!strings.Contains(msg, "exceeds per-file cap") {
		t.Errorf("got: %v", err)
	}
}

func TestDownloadAndExtractRejectsPerFileCap(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"big.txt": bytes.Repeat([]byte("A"), 200)})

	bk := NewBuildKit(BuildKitConfig{
		Storage:           &fakeStorage{archive: archive},
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxExtractedBytes: 10000,
		MaxFileBytes:      50,
	})

	err := bk.downloadAndExtract(context.Background(), "s3://b/key", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "exceeds per-file cap") {
		t.Fatalf("expected per-file cap error, got %v", err)
	}
}

func TestDownloadAndExtractRejectsPathTraversal(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"../etc/passwd": []byte("x")})

	bk := NewBuildKit(BuildKitConfig{
		Storage: &fakeStorage{archive: archive},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	err := bk.downloadAndExtract(context.Background(), "s3://b/key", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "escapes archive root") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestEnqueueQueueFull(t *testing.T) {
	bk := NewBuildKit(BuildKitConfig{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	for i := 0; i < cap(bk.queue); i++ {
		if err := bk.Enqueue("x"); err != nil {
			t.Fatalf("unexpected err at fill: %v", err)
		}
	}
	if err := bk.Enqueue("x"); err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestPushWithRetrySucceedsOnSecondAttempt(t *testing.T) {
	var attempts atomic.Int32
	bk := NewBuildKit(BuildKitConfig{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		PushAttempts: 3,
	})
	bk.runCmd = func(_ context.Context, _ *slog.Logger, _ string, _ ...string) error {
		n := attempts.Add(1)
		if n < 2 {
			return io.ErrUnexpectedEOF
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bk.pushWithRetry(ctx, slog.Default(), "img"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d", attempts.Load())
	}
}
