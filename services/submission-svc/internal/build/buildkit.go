package build

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/storage"
	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/store"
)

// Default extraction caps. A 50MB compressed archive should never expand
// beyond ~500MB; anything larger is almost certainly a decompression bomb.
const (
	defaultMaxExtractedBytes int64 = 500 * 1024 * 1024
	defaultMaxFileBytes      int64 = 100 * 1024 * 1024
)

// BuildKit downloads the contestant archive from object storage, extracts it
// safely, invokes `docker build` against the language-appropriate Dockerfile
// from sandbox-images/, then pushes the resulting image to a local registry.
type BuildKit struct {
	repo            store.Repository
	storage         storage.ObjectStore
	log             *slog.Logger
	queue           chan job
	sandboxImageDir string
	registryAddr    string
	imageRepoPrefix string
	buildTimeout    time.Duration
	pushAttempts    int
	maxExtractBytes int64
	maxFileBytes    int64
	scanner         ImageScanner
	sandbox         SandboxStarter

	// runCmd is injectable so tests can stub out exec.Command.
	runCmd func(ctx context.Context, log *slog.Logger, name string, args ...string) error

	mu       sync.Mutex
	inFlight map[string]context.CancelFunc
}

// ImageScanner runs a security scan against a built image reference. Failure
// returns a human-readable reason. Implementations may no-op if a scanner is
// not configured.
type ImageScanner interface {
	Scan(ctx context.Context, imageRef string) error
}

type BuildKitConfig struct {
	Repo              store.Repository
	Storage           storage.ObjectStore
	Logger            *slog.Logger
	SandboxImageDir   string        // path to sandbox-images/
	RegistryAddr      string        // e.g. "localhost:5000"
	ImageRepoPrefix   string        // e.g. "contestants"
	BuildTimeout      time.Duration // 0 -> 5 minutes
	PushAttempts      int           // 0 -> 3
	MaxExtractedBytes int64         // 0 -> 500 MiB
	MaxFileBytes      int64         // 0 -> 100 MiB
	Scanner           ImageScanner  // nil -> skip
	Sandbox           SandboxStarter // nil -> skip
}

func NewBuildKit(cfg BuildKitConfig) *BuildKit {
	if cfg.BuildTimeout == 0 {
		cfg.BuildTimeout = 5 * time.Minute
	}
	if cfg.ImageRepoPrefix == "" {
		cfg.ImageRepoPrefix = "contestants"
	}
	if cfg.PushAttempts == 0 {
		cfg.PushAttempts = 3
	}
	if cfg.MaxExtractedBytes == 0 {
		cfg.MaxExtractedBytes = defaultMaxExtractedBytes
	}
	if cfg.MaxFileBytes == 0 {
		cfg.MaxFileBytes = defaultMaxFileBytes
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &BuildKit{
		repo:            cfg.Repo,
		storage:         cfg.Storage,
		log:             log.With("component", "build.buildkit"),
		queue:           make(chan job, 64),
		sandboxImageDir: cfg.SandboxImageDir,
		registryAddr:    cfg.RegistryAddr,
		imageRepoPrefix: cfg.ImageRepoPrefix,
		buildTimeout:    cfg.BuildTimeout,
		pushAttempts:    cfg.PushAttempts,
		maxExtractBytes: cfg.MaxExtractedBytes,
		maxFileBytes:    cfg.MaxFileBytes,
		scanner:         cfg.Scanner,
		sandbox:         cfg.Sandbox,
		runCmd:          execRunCmd,
		inFlight:        make(map[string]context.CancelFunc),
	}
}

func (b *BuildKit) Enqueue(submissionID string) error {
	select {
	case b.queue <- job{submissionID: submissionID}:
		return nil
	default:
		return ErrQueueFull
	}
}

func (b *BuildKit) Run(ctx context.Context) {
	b.log.Info("worker started")
	for {
		select {
		case <-ctx.Done():
			b.cancelAll()
			return
		case j := <-b.queue:
			b.process(ctx, j)
		}
	}
}

func (b *BuildKit) cancelAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, cancel := range b.inFlight {
		b.log.Info("cancelling in-flight build", "submission_id", id)
		cancel()
	}
}

func (b *BuildKit) process(parentCtx context.Context, j job) {
	ctx, cancel := context.WithTimeout(parentCtx, b.buildTimeout)
	defer cancel()

	b.mu.Lock()
	b.inFlight[j.submissionID] = cancel
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.inFlight, j.submissionID)
		b.mu.Unlock()
	}()

	log := b.log.With("submission_id", j.submissionID)

	sub, ok := b.repo.Get(j.submissionID)
	if !ok {
		log.Warn("submission not found")
		return
	}

	if err := b.repo.UpdateStatus(j.submissionID, store.StatusBuilding, "", ""); err != nil {
		log.Error("update status building", "err", err)
		return
	}

	imageRef, err := b.runBuild(ctx, log, sub)
	if err != nil {
		log.Error("build failed", "err", err)
		_ = b.repo.UpdateStatus(j.submissionID, store.StatusBuildFailed, "", truncate(err.Error(), 4096))
		return
	}

	if err := b.repo.UpdateStatus(j.submissionID, store.StatusReady, imageRef, ""); err != nil {
		log.Error("update status ready", "err", err)
		return
	}
	log.Info("build complete", "image_uri", imageRef)

	if b.sandbox != nil {
		if err := b.sandbox.Start(ctx, j.submissionID, sub.ContestantID, imageRef); err != nil {
			log.Error("sandbox start failed", "err", err)
		}
	}
}

func (b *BuildKit) runBuild(ctx context.Context, log *slog.Logger, sub store.Submission) (string, error) {
	dockerfile, err := b.dockerfileFor(sub.Language)
	if err != nil {
		return "", err
	}

	workDir, err := os.MkdirTemp("", "iicpc-build-"+sub.ID+"-")
	if err != nil {
		return "", fmt.Errorf("mkdir tempdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	srcDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir srcdir: %w", err)
	}

	if err := b.downloadAndExtract(ctx, sub.SourceURI, srcDir); err != nil {
		return "", fmt.Errorf("fetch source: %w", err)
	}

	imageRef := fmt.Sprintf("%s/%s/%s:latest", b.registryAddr, b.imageRepoPrefix, sub.ID)
	entrypoint := sub.Entrypoint
	if entrypoint == "" {
		entrypoint = "/app/main"
	}

	if err := b.runCmd(ctx, log, "docker", "build",
		"-f", dockerfile,
		"-t", imageRef,
		"--build-arg", "ENTRYPOINT_PATH="+entrypoint,
		srcDir,
	); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}

	if err := b.pushWithRetry(ctx, log, imageRef); err != nil {
		return "", fmt.Errorf("docker push: %w", err)
	}

	if b.scanner != nil {
		if err := b.scanner.Scan(ctx, imageRef); err != nil {
			return "", fmt.Errorf("image scan: %w", err)
		}
	}

	return imageRef, nil
}

func (b *BuildKit) pushWithRetry(ctx context.Context, log *slog.Logger, imageRef string) error {
	var last error
	backoff := time.Second
	for attempt := 1; attempt <= b.pushAttempts; attempt++ {
		err := b.runCmd(ctx, log, "docker", "push", imageRef)
		if err == nil {
			return nil
		}
		last = err
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return err
		}
		if attempt == b.pushAttempts {
			break
		}
		log.Warn("push attempt failed, retrying", "attempt", attempt, "err", err, "backoff", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
		backoff *= 2
	}
	return last
}

func (b *BuildKit) dockerfileFor(lang store.Language) (string, error) {
	var name string
	switch lang {
	case store.LangGo:
		name = "Dockerfile.go"
	case store.LangRust:
		name = "Dockerfile.rust"
	case store.LangCPP:
		name = "Dockerfile.cpp"
	default:
		return "", fmt.Errorf("no sandbox image for language %q", lang)
	}
	return filepath.Join(b.sandboxImageDir, name), nil
}

func (b *BuildKit) downloadAndExtract(ctx context.Context, sourceURI, dst string) error {
	key, err := keyFromSourceURI(sourceURI)
	if err != nil {
		return err
	}

	rc, _, err := b.storage.Get(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()

	gz, err := gzip.NewReader(rc)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	// Cap total decompressed bytes to defeat zip-bomb-style archives.
	limited := &io.LimitedReader{R: gz, N: b.maxExtractBytes + 1}
	tr := tar.NewReader(limited)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		if err := extractTarEntry(tr, hdr, dst, b.maxFileBytes); err != nil {
			return err
		}
	}
	if limited.N <= 0 {
		return fmt.Errorf("archive exceeds max extracted size %d bytes", b.maxExtractBytes)
	}
	return nil
}

func extractTarEntry(tr *tar.Reader, hdr *tar.Header, dst string, maxFileBytes int64) error {
	clean := filepath.Clean(hdr.Name)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return fmt.Errorf("tar entry escapes archive root: %s", hdr.Name)
	}
	target := filepath.Join(dst, clean)

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", target, err)
		}
	case tar.TypeReg:
		if hdr.Size > maxFileBytes {
			return fmt.Errorf("tar entry %s exceeds per-file cap %d bytes", hdr.Name, maxFileBytes)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir parent of %s: %w", target, err)
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755|0o644)
		if err != nil {
			return fmt.Errorf("open %s: %w", target, err)
		}
		if _, err := io.CopyN(f, tr, hdr.Size); err != nil && err != io.EOF {
			_ = f.Close()
			return fmt.Errorf("write %s: %w", target, err)
		}
		_ = f.Close()
	case tar.TypeSymlink, tar.TypeLink:
		// Ignore links: contestants don't need them, and they widen the
		// extraction attack surface.
	default:
		// Quietly skip unknown types (FIFOs, devices, etc.).
	}
	return nil
}

func keyFromSourceURI(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse source uri: %w", err)
	}
	if u.Scheme != "s3" {
		return "", fmt.Errorf("unsupported source scheme %q", u.Scheme)
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}

func execRunCmd(ctx context.Context, log *slog.Logger, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = logWriter{log: log, stream: "stdout", cmd: name}
	cmd.Stderr = logWriter{log: log, stream: "stderr", cmd: name}
	return cmd.Run()
}

type logWriter struct {
	log    *slog.Logger
	stream string
	cmd    string
}

func (w logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		w.log.Info("subprocess", "cmd", w.cmd, "stream", w.stream, "line", line)
	}
	return len(p), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
