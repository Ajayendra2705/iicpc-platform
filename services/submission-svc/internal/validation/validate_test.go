package validation

import (
	"testing"

	"github.com/iicpc/platform/services/submission-svc/internal/store"
)

func TestValidateRequest(t *testing.T) {
	cases := []struct {
		name         string
		contestantID string
		entrypoint   string
		lang         store.Language
		wantErr      error
	}{
		{"happy", "team1", "/app/orderbook", store.LangGo, nil},
		{"empty contestant", "", "/app/x", store.LangGo, ErrEmptyContestantID},
		{"bad lang", "team1", "/app/x", store.Language("py"), ErrUnsupportedLanguage},
		{"empty entrypoint", "team1", "  ", store.LangRust, ErrEmptyEntrypoint},
		{"path traversal", "team1", "/app/../../etc/passwd", store.LangCPP, ErrInvalidEntrypoint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateRequest(tc.contestantID, tc.entrypoint, tc.lang)
			if got != tc.wantErr {
				t.Errorf("got %v want %v", got, tc.wantErr)
			}
		})
	}
}
