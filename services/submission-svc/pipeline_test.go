//go:build integration

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"log/slog"

	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/build"
	httpsrv "github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/server"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/storage"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/store"
)

// spySandbox records every Start call for assertions.
type spySandbox struct {
	mu    sync.Mutex
	calls []sandboxCall
}

type sandboxCall struct {
	SubmissionID string
	ContestantID string
	ImageURI     string
}

func (s *spySandbox) Start(_ context.Context, subID, contID, img string) error {
	s.mu.Lock()
	s.calls = append(s.calls, sandboxCall{subID, contID, img})
	s.mu.Unlock()
	return nil
}

func (s *spySandbox) lastCall() (sandboxCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) == 0 {
		return sandboxCall{}, false
	}
	return s.calls[len(s.calls)-1], true
}

// makeTarGz builds a minimal tar.gz archive containing one file.
func makeTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	content := []byte("package main\nfunc main(){}\n")
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "main.go", Size: int64(len(content)), Mode: 0o644})
	tw.Write(content)
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func buildTestServer(t *testing.T, spy *spySandbox) *httptest.Server {
	t.Helper()
	repo := store.NewMemory()
	objStore := storage.NewMemory()
	logger := slog.Default()

	stub := build.NewStubWithSandbox(repo, logger, spy)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go stub.Run(ctx)

	srv := httpsrv.New(httpsrv.Config{
		MaxArchiveBytes: 50 * 1024 * 1024,
		Storage:         objStore,
		Submissions:     repo,
		Builder:         stub,
		Logger:          logger,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func uploadSubmission(t *testing.T, ts *httptest.Server, archive []byte) string {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("contestant_id", "c1")
	mw.WriteField("language", "go")
	mw.WriteField("entrypoint", "/app/main")
	fw, _ := mw.CreateFormFile("archive", "source.tar.gz")
	fw.Write(archive)
	mw.Close()

	resp, err := ts.Client().Post(ts.URL+"/submissions", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("POST /submissions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 202, got %d: %s", resp.StatusCode, raw)
	}
	var sub store.Submission
	json.NewDecoder(resp.Body).Decode(&sub)
	return sub.ID
}

func pollUntilReady(t *testing.T, ts *httptest.Server, id string) store.Submission {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, _ := ts.Client().Get(fmt.Sprintf("%s/submissions/%s", ts.URL, id))
		var sub store.Submission
		json.NewDecoder(resp.Body).Decode(&sub)
		resp.Body.Close()
		if sub.Status == store.StatusReady {
			return sub
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("submission %s did not reach ready within deadline", id)
	return store.Submission{}
}

func TestPipelineUploadBuildSandbox(t *testing.T) {
	spy := &spySandbox{}
	ts := buildTestServer(t, spy)
	archive := makeTarGz(t)

	id := uploadSubmission(t, ts, archive)
	sub := pollUntilReady(t, ts, id)

	if sub.ImageURI == "" {
		t.Fatal("want non-empty image_uri on ready submission")
	}

	// Verify sandbox-runner was called with the right parameters.
	call, ok := spy.lastCall()
	if !ok {
		t.Fatal("sandbox Start never called after build")
	}
	if call.SubmissionID != id {
		t.Errorf("sandbox got submission_id=%q, want %q", call.SubmissionID, id)
	}
	if call.ContestantID != "c1" {
		t.Errorf("sandbox got contestant_id=%q, want c1", call.ContestantID)
	}
	if call.ImageURI != sub.ImageURI {
		t.Errorf("sandbox got image_uri=%q, want %q", call.ImageURI, sub.ImageURI)
	}
}

func TestPipelineHealthz(t *testing.T) {
	ts := buildTestServer(t, &spySandbox{})
	resp, err := ts.Client().Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestPipelineSubmissionNotFound(t *testing.T) {
	ts := buildTestServer(t, &spySandbox{})
	resp, _ := ts.Client().Get(ts.URL + "/submissions/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestPipelineRejectsBadArchive(t *testing.T) {
	ts := buildTestServer(t, &spySandbox{})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("contestant_id", "c1")
	mw.WriteField("language", "go")
	mw.WriteField("entrypoint", "/app/main")
	fw, _ := mw.CreateFormFile("archive", "source.tar.gz")
	fw.Write([]byte("not a gzip file"))
	mw.Close()

	resp, _ := ts.Client().Post(ts.URL+"/submissions", mw.FormDataContentType(), &body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for non-gzip archive, got %d", resp.StatusCode)
	}
}
