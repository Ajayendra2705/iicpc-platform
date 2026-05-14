package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("invalid token")
	ErrExpired = errors.New("token expired")
)

type Claims struct {
	ContestantID string `json:"contestant_id"`
	ExpiresAt    int64  `json:"exp"`
	IssuedAt     int64  `json:"iat"`
}

type ctxKey struct{}

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

func Issue(contestantID, secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	payload, err := json.Marshal(Claims{
		ContestantID: contestantID,
		ExpiresAt:    now.Add(ttl).Unix(),
		IssuedAt:     now.Unix(),
	})
	if err != nil {
		return "", err
	}
	msg := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	return msg + "." + sign(msg, secret), nil
}

func Validate(token, secret string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalid
	}
	msg := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(sign(msg, secret)), []byte(parts[2])) {
		return nil, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalid
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, ErrInvalid
	}
	if time.Now().Unix() > c.ExpiresAt {
		return nil, ErrExpired
	}
	return &c, nil
}

func Middleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		c, err := Validate(raw, secret)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-Contestant-ID", c.ContestantID)
		ctx := context.WithValue(r.Context(), ctxKey{}, c.ContestantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ContestantIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

func sign(msg, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
