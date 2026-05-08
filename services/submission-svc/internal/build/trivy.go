package build

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// TrivyScanner shells out to the `trivy` CLI to scan a built image for
// HIGH/CRITICAL vulnerabilities. If `trivy` isn't on PATH, NewTrivyScanner
// returns nil so the build pipeline silently skips scanning. Production
// installs trivy as a sidecar / build-job step.
type TrivyScanner struct {
	severities string
	log        *slog.Logger
}

func NewTrivyScanner(log *slog.Logger) *TrivyScanner {
	if _, err := exec.LookPath("trivy"); err != nil {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return &TrivyScanner{
		severities: "HIGH,CRITICAL",
		log:        log.With("component", "build.trivy"),
	}
}

func (t *TrivyScanner) Scan(ctx context.Context, imageRef string) error {
	cmd := exec.CommandContext(ctx, "trivy", "image",
		"--severity", t.severities,
		"--exit-code", "1",
		"--no-progress",
		"--format", "table",
		imageRef,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("trivy reported issues: %s", strings.TrimSpace(string(out)))
	}
	t.log.Info("trivy clean", "image", imageRef)
	return nil
}
