package validation

import (
	"errors"
	"strings"

	"github.com/Ajayendra2705/iicpc-platform/services/submission-svc/internal/store"
)

var (
	ErrUnsupportedLanguage = errors.New("unsupported language")
	ErrEmptyEntrypoint     = errors.New("entrypoint required")
	ErrInvalidEntrypoint   = errors.New("invalid entrypoint path")
	ErrEmptyContestantID   = errors.New("contestant_id required")
)

func ValidateRequest(contestantID, entrypoint string, lang store.Language) error {
	if strings.TrimSpace(contestantID) == "" {
		return ErrEmptyContestantID
	}
	if !lang.Valid() {
		return ErrUnsupportedLanguage
	}
	ep := strings.TrimSpace(entrypoint)
	if ep == "" {
		return ErrEmptyEntrypoint
	}
	if strings.Contains(ep, "..") {
		return ErrInvalidEntrypoint
	}
	return nil
}
