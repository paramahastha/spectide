package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/workflow"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestRejectsLookalikeOrigin(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/profiles", strings.NewReader(`{}`))
	r.Host = "127.0.0.1"
	r.Header.Set("Origin", "http://127.0.0.1.attacker.example")
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRejectsInvalidEventCursor(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/runs/not-a-run/events?after=-1", nil)
	r.Host = "127.0.0.1"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}
func TestCreateRun(t *testing.T) {
	s := testServer(t)
	project := t.TempDir()
	body, err := json.Marshal(map[string]string{"project": project, "prompt": "test"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(string(body)))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "request-key-123")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var first struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(string(body)))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "request-key-123")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var second struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &second)
	if w.Code != http.StatusOK || first.ID != second.ID {
		t.Fatalf("duplicate status=%d ids=%s/%s body=%s", w.Code, first.ID, second.ID, w.Body.String())
	}
	differentBody, err := json.Marshal(map[string]string{"project": project, "prompt": "different"})
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(string(differentBody)))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "request-key-123")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "IDEMPOTENCY_KEY_REUSED") {
		t.Fatalf("key mismatch status=%d body=%s", w.Code, w.Body.String())
	}
	_ = context.Background()
}

func TestDryRunPreviewsConfiguredRoutesWithoutCreatingRun(t *testing.T) {
	s := testServer(t)
	project := t.TempDir()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(`{"project":"`+project+`","prompt":"inspect","dry_run":true}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "preview-request-123")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"execution":"NOT_RUN"`) || !strings.Contains(w.Body.String(), `"decisions":[`) {
		t.Fatalf("preview status=%d body=%s", w.Code, w.Body.String())
	}
	runs, err := s.Store.Runs(context.Background())
	if err != nil || len(runs) != 0 {
		t.Fatalf("dry run created records: runs=%d err=%v", len(runs), err)
	}
}

func TestDryRunRejectsProjectConfigSymlinkEscape(t *testing.T) {
	s := testServer(t)
	project, outside := t.TempDir(), t.TempDir()
	if err := config.Save(config.ProjectPath(outside), config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".spectide"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(config.ProjectPath(outside), config.ProjectPath(project)); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(`{"project":"`+project+`","prompt":"inspect","dry_run":true}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "preview-request-456")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "PERMISSION_DENIED") {
		t.Fatalf("symlink escape status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDryRunRejectsWorkspaceOutsideConfiguredScopes(t *testing.T) {
	s := testServer(t)
	project, allowed := t.TempDir(), t.TempDir()
	cfg := config.Default()
	cfg.AllowedScopes = []string{allowed}
	if err := config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"project": project, "prompt": "inspect", "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(string(body)))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "preview-scope-123")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "PERMISSION_DENIED") {
		t.Fatalf("outside-scope preview status=%d body=%s", w.Code, w.Body.String())
	}
	runs, err := s.Store.Runs(context.Background())
	if err != nil || len(runs) != 0 {
		t.Fatalf("rejected preview created records: runs=%d err=%v", len(runs), err)
	}
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

func TestProjectConfigAndApprovalEndpoints(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	body := `{"path":"` + root + `","name":"fixture"}`
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/projects", strings.NewReader(body))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("project=%d %s", w.Code, w.Body.String())
	}
	var project struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &project); err != nil || project.ID == "" {
		t.Fatalf("project=%+v err=%v", project, err)
	}
	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/projects/"+project.ID+"/config", nil)
	r.Host = "127.0.0.1"
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"routing_mode":"hybrid"`) {
		t.Fatalf("config=%d %s", w.Code, w.Body.String())
	}

	run, err := s.Store.CreateRun(context.Background(), root, "approval test")
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs/"+run.ID+"/approvals", strings.NewReader(`{"action":"workspace write"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("approval=%d %s", w.Code, w.Body.String())
	}
	var approval struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &approval); err != nil || approval.ID == "" {
		t.Fatalf("approval=%+v err=%v", approval, err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs/"+run.ID+"/approvals/"+approval.ID, strings.NewReader(`{"decision":"approved"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "APPROVED") {
		t.Fatalf("resolve=%d %s", w.Code, w.Body.String())
	}
}

func TestProjectConfigRejectsRegisteredPathSymlinkSwap(t *testing.T) {
	s := testServer(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/projects", strings.NewReader(`{"path":"`+root+`","name":"fixture"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var project struct {
		ID string `json:"id"`
	}
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &project) != nil {
		t.Fatalf("project=%d %s", w.Code, w.Body.String())
	}
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/projects/"+project.ID+"/config", nil)
	r.Host = "127.0.0.1"
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "PROJECT_PATH_CHANGED") {
		t.Fatalf("swap=%d %s", w.Code, w.Body.String())
	}
}

func TestAPIRejectsOversizedAndUnkeyedRunRequests(t *testing.T) {
	s := testServer(t)
	for _, test := range []struct {
		name string
		body string
		key  string
	}{
		{name: "missing key", body: `{"project":"demo","prompt":"test"}`},
		{name: "oversized", body: `{"name":"` + strings.Repeat("x", 20<<10) + `"}`, key: "request-key-123"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(test.body))
			r.Host = "127.0.0.1"
			r.Header.Set("X-SpecTide-CSRF", s.CSRF)
			if test.key != "" {
				r.Header.Set("Idempotency-Key", test.key)
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"error"`) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

type brokenStreamWriter struct {
	header  http.Header
	written bool
}

func (w *brokenStreamWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (*brokenStreamWriter) WriteHeader(int) {}
func (w *brokenStreamWriter) Write([]byte) (int, error) {
	w.written = true
	return 0, errors.New("client disconnected")
}
func (*brokenStreamWriter) Flush() {}

func TestSSEStopsWritingWhenClientDisconnects(t *testing.T) {
	s := testServer(t)
	run, err := s.Store.CreateRun(context.Background(), "demo", "sse test")
	if err != nil {
		t.Fatal(err)
	}
	w := &brokenStreamWriter{}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/runs/"+run.ID+"/events", nil)
	r.Host = "127.0.0.1"
	s.events(w, r, run.ID)
	if !w.written {
		t.Fatal("SSE writer did not attempt persisted replay")
	}
}
