package auth

import (
	"strings"
	"testing"
	"time"
)

const testSecret = "test-secret-key"

func TestIssueAndValidate(t *testing.T) {
	tok, err := Issue("c1", testSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Validate(tok, testSecret)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if c.ContestantID != "c1" {
		t.Fatalf("want c1, got %s", c.ContestantID)
	}
}

func TestExpiredToken(t *testing.T) {
	tok, _ := Issue("c1", testSecret, -time.Second)
	_, err := Validate(tok, testSecret)
	if err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestWrongSecret(t *testing.T) {
	tok, _ := Issue("c1", testSecret, time.Minute)
	_, err := Validate(tok, "other-secret")
	if err != ErrInvalid {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestMalformedToken(t *testing.T) {
	for _, bad := range []string{"", "a", "a.b", "a.b.c.d"} {
		_, err := Validate(bad, testSecret)
		if err != ErrInvalid {
			t.Fatalf("input %q: want ErrInvalid, got %v", bad, err)
		}
	}
}

func TestTamperedPayload(t *testing.T) {
	tok, _ := Issue("c1", testSecret, time.Minute)
	parts := strings.Split(tok, ".")
	// flip last byte of payload
	p := []byte(parts[1])
	p[len(p)-1] ^= 0xff
	parts[1] = string(p)
	_, err := Validate(strings.Join(parts, "."), testSecret)
	if err != ErrInvalid {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}
