package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iicpc/platform/services/submission-svc/internal/store"
)

type fakeStorage struct {
	calls int
}

func (f *fakeStorage) Put(_ context.Context, key string, _ io.Reader, _ int64, _ string) (string, error) {
	f.calls++
	return "s3://test/" + key, nil
}

type fakeBuilder struct {
	enqueued []string
}

func (f *fakeBuilder) Enqueue(id string) { f.enqueued = append(f.enqueued, id) }

func newTestServer(t *testing.T) (*Server, *fakeStorage, *fakeBuilder, store.Repository) {
	t.Helper()
	st := &fakeStorage{}
	bu := &fakeBuilder{}
	repo := store.NewMemory()
	srv := New(Config{
		MaxArchiveBytes: 1024 * 1024,
		Storage:         st,
		Submissions:     repo,
		Builder:         bu,
	})
	return srv, st, bu, repo
}

func multipartBody(t *testing.T, fields map[string]string, fileName string, fileBody []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if fileName != "" {
		fw, err := w.CreateFormFile("archive", fileName)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(fileBody)
	}
	_ = w.Close()
	return buf, w.FormDataContentType()
}

func TestCreateSubmissionHappy(t *testing.T) {
	srv, st, bu, repo := newTestServer(t)
	body, ct := multipartBody(t, map[string]string{
		"contestant_id": "team1",
		"language":      "go",
		"entrypoint":    "/app/orderbook",
	}, "src.tar.gz", []byte("fake"))

	req := httptest.NewRequest(http.MethodPost, "/submissions", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if st.calls != 1 {
		t.Errorf("storage.Put called %d times", st.calls)
	}
	if len(bu.enqueued) != 1 {
		t.Errorf("builder.Enqueue called %d times", len(bu.enqueued))
	}
	subs := repo.List("team1", 0)
	if len(subs) != 1 || subs[0].Language != store.LangGo {
		t.Errorf("repo state: %+v", subs)
	}
}

func TestCreateSubmissionRejectsBadLang(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	body, ct := multipartBody(t, map[string]string{
		"contestant_id": "team1",
		"language":      "py",
		"entrypoint":    "/x",
	}, "src.tar.gz", []byte("x"))

	req := httptest.NewRequest(http.MethodPost, "/submissions", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unsupported language") {
		t.Errorf("body: %s", rr.Body.String())
	}
}

func TestGetSubmissionNotFound(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/submissions/missing", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestListSubmissions(t *testing.T) {
	srv, _, _, repo := newTestServer(t)
	_ = repo.Create(store.Submission{ID: "1", ContestantID: "a", Language: store.LangGo})
	_ = repo.Create(store.Submission{ID: "2", ContestantID: "b", Language: store.LangCPP})

	req := httptest.NewRequest(http.MethodGet, "/submissions?contestant_id=a", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Submissions []store.Submission `json:"submissions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Submissions) != 1 || resp.Submissions[0].ID != "1" {
		t.Errorf("got %+v", resp.Submissions)
	}
}
