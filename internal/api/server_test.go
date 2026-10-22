package api

import (
	"context"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/workflow"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, e := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return New(s, workflow.New(s))
}
func TestRejectsNonLoopbackHost(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodGet, "http://attacker.example/api/v1/healthz", nil)
	r.Host = "attacker.example"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}
func TestRunNeedsCSRF(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", nil)
	r.Host = "127.0.0.1"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}
func TestCreateRun(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(`{"project":"demo","prompt":"test"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	_ = context.Background()
}

func TestCreateAndListProfile(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/profiles", strings.NewReader(`{"name":"fake","provider":"fake","executable":"fake","allowed_scopes":["personal-public"]}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d: %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/profiles", nil)
	r.Host = "127.0.0.1"
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "fake") {
		t.Fatalf("list=%d: %s", w.Code, w.Body.String())
	}
}
