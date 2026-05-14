package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testCfg() config {
	return config{
		Addr:             ":0",
		JWTSecret:        "test-secret",
		JWTTokenTTL:      time.Minute,
		SubmissionSvcURL: "http://localhost:9999", // not started; unreachable by design
		RateRPS:          1000,
		RateBurst:        1000,
	}
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	// stub upstream — returns 200 for any request
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	cfg := testCfg()
	cfg.SubmissionSvcURL = upstream.URL

	// proxy to stub upstream via stdlib reverse proxy
	var stubProxy http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// forward to upstream directly (avoids proxy.New URL parsing in tests)
		req, _ := http.NewRequest(r.Method, upstream.URL+r.URL.Path, r.Body)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
	})

	ts := httptest.NewServer(buildHandler(cfg, stubProxy))
	t.Cleanup(ts.Close)
	return ts
}

func issueToken(t *testing.T, ts *httptest.Server, contestantID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"contestant_id": contestantID})
	resp, err := ts.Client().Post(ts.URL+"/auth/token", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token issue: want 200, got %d", resp.StatusCode)
	}
	var tr tokenResp
	json.NewDecoder(resp.Body).Decode(&tr)
	return tr.Token
}

func TestHealthzNoAuth(t *testing.T) {
	ts := testServer(t)
	resp, _ := ts.Client().Get(ts.URL + "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestTokenIssue(t *testing.T) {
	ts := testServer(t)
	tok := issueToken(t, ts, "c1")
	if tok == "" {
		t.Fatal("want non-empty token")
	}
}

func TestTokenIssueMissingID(t *testing.T) {
	ts := testServer(t)
	resp, _ := ts.Client().Post(ts.URL+"/auth/token", "application/json", bytes.NewReader([]byte(`{}`)))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestSubmissionsRequiresJWT(t *testing.T) {
	ts := testServer(t)
	resp, _ := ts.Client().Get(ts.URL + "/submissions")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestSubmissionsWithJWT(t *testing.T) {
	ts := testServer(t)
	tok := issueToken(t, ts, "c1")
	req, _ := http.NewRequest("GET", ts.URL+"/submissions", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestInvalidJWT(t *testing.T) {
	ts := testServer(t)
	req, _ := http.NewRequest("GET", ts.URL+"/submissions", nil)
	req.Header.Set("Authorization", "Bearer garbage.token.here")
	resp, _ := ts.Client().Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestXContestantIDHeader(t *testing.T) {
	// verify contestant ID is injected as header to upstream
	var gotHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Contestant-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := testCfg()
	var stubProxy http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Contestant-ID")
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(buildHandler(cfg, stubProxy))
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"contestant_id": "c42"})
	resp, _ := ts.Client().Post(ts.URL+"/auth/token", "application/json", bytes.NewReader(body))
	var tr tokenResp
	json.NewDecoder(resp.Body).Decode(&tr)
	resp.Body.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/submissions", nil)
	req.Header.Set("Authorization", "Bearer "+tr.Token)
	ts.Client().Do(req)

	if gotHeader != "c42" {
		t.Fatalf("want X-Contestant-ID=c42, got %q", gotHeader)
	}
}
