package build

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iicpc/platform/services/submission-svc/internal/storage"
	"github.com/iicpc/platform/services/submission-svc/internal/store"
)

// BuildKit downloads the contestant archive from object storage, extracts it,
// invokes `docker build` against the language-appropriate Dockerfile from
// sandbox-images/, then pushes the resulting image to a local registry.
//
// `docker` must be available on $PATH. The host Docker daemon must be able to
// reach the registry at RegistryAddr without TLS (typical for
// `localhost:5000`).
type BuildKit struct {
	repo            store.Repository
	storage         storage.ObjectStore
	queue           chan job
	sandboxImageDir string
	registryAddr    string
	imageRepoPrefix string
	buildTimeout    time.Duration
}

type BuildKitConfig struct {
	Repo            store.Repository
	Storage         storage.ObjectStore
	SandboxImageDir string        // path to sandbox-images/
	RegistryAddr    string        // e.g. "localhost:5000"
	ImageRepoPrefix string        // e.g. "contestants"
	BuildTimeout    time.Duration // 0 -> 5 minutes
}

func NewBuildKit(cfg BuildKitConfig) *BuildKit {
	if cfg.BuildTimeout == 0 {
		cfg.BuildTimeout = 5 * time.Minute
	}
	if cfg.ImageRepoPrefix == "" {
		cfg.ImageRepoPrefix = "contestants"
	}
	return &BuildKit{
		repo:            cfg.Repo,
		storage:         cfg.Storage,
		queue:           make(chan job, 64),
		sandboxImageDir: cfg.SandboxImageDir,
		registryAddr:    cfg.RegistryAddr,
		imageRepoPrefix: cfg.ImageRepoPrefix,
		buildTimeout:    cfg.BuildTimeout,
	}
}

func (b *BuildKit) Enqueue(submissionID string) {
	select {
	case b.queue <- job{submissionID: submissionID}:
	default:
		log.Printf("build queue full, dropping submission %s", submissionID)
	}
}

func (b *BuildKit) Run(ctx context.Context) {
	log.Println("build worker (buildkit) started")
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-b.queue:
			b.process(ctx, j)
		}
	}
}

func (b *BuildKit) process(parentCtx context.Context, j job) {
	ctx, cancel := context.WithTimeout(parentCtx, b.buildTimeout)
	defer cancel()

	sub, ok := b.repo.Get(j.submissionID)
	if !ok {
		log.Printf("build: submission %s not found", j.submissionID)
		return
	}

	if err := b.repo.UpdateStatus(j.submissionID, store.StatusBuilding, "", ""); err != nil {
		log.Printf("update status building: %v", err)
		return
	}

	imageRef, err := b.runBuild(ctx, sub)
	if err != nil {
		log.Printf("build failed for %s: %v", j.submissionID, err)
		_ = b.repo.UpdateStatus(j.submissionID, store.StatusBuildFailed, "", truncate(err.Error(), 4096))
		return
	}

	if err := b.repo.UpdateStatus(j.submissionID, store.StatusReady, imageRef, ""); err != nil {
		log.Printf("update status ready: %v", err)
		return
	}
	log.Printf("build complete for %s -> %s", j.submissionID, imageRef)
}

func (b *BuildKit) runBuild(ctx context.Context, sub store.Submission) (string, error) {
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

	if err := runCmd(ctx, "docker", "build",
		"-f", dockerfile,
		"-t", imageRef,
		"--build-arg", "ENTRYPOINT_PATH="+entrypoint,
		srcDir,
	); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}

	if err := runCmd(ctx, "docker", "push", imageRef); err != nil {
		return "", fmt.Errorf("docker push: %w", err)
	}

	return imageRef, nil
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

// downloadAndExtract fetches a tar.gz archive from the object store and
// extracts it into dst. SourceURI is "s3://bucket/key"; the bucket portion is
// already encoded in the storage client, so we only need the key.
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

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		if err := extractTarEntry(tr, hdr, dst); err != nil {
			return err
		}
	}
	return nil
}

func extractTarEntry(tr *tar.Reader, hdr *tar.Header, dst string) error {
	clean := filepath.Clean(hdr.Name)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return fmt.Errorf("tar entry escapes archive root: %s", hdr.Name)
	}
	target := filepath.Join(dst, clean)

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(target, os.FileMode(hdr.Mode)&0o755|0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", target, err)
		}
	case tar.TypeReg, tar.TypeRegA:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir parent of %s: %w", target, err)
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755|0o644)
		if err != nil {
			return fmt.Errorf("open %s: %w", target, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			return fmt.Errorf("write %s: %w", target, err)
		}
		_ = f.Close()
	case tar.TypeSymlink, tar.TypeLink:
		// Ignore links; contestants do not need them, and they widen the
		// extraction attack surface.
		log.Printf("skipping link entry %s", hdr.Name)
	default:
		log.Printf("skipping unknown tar entry %s (type %d)", hdr.Name, hdr.Typeflag)
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

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = logWriter{prefix: name + " out"}
	cmd.Stderr = logWriter{prefix: name + " err"}
	return cmd.Run()
}

type logWriter struct{ prefix string }

func (w logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		log.Printf("[%s] %s", w.prefix, line)
	}
	return len(p), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
