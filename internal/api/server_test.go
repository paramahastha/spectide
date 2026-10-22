package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/runner"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/workflow"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestAPIJSONAndErrorResponsesRedactNestedSecrets(t *testing.T) {
	secret := "api-synthetic-secret-123456"
	jsonResponse := httptest.NewRecorder()
	jsonOut(jsonResponse, http.StatusOK, map[string]any{"report": map[string]any{"evidence": "token=" + secret, "api_key": secret}})
	if strings.Contains(jsonResponse.Body.String(), secret) || !strings.Contains(jsonResponse.Body.String(), "[REDACTED]") {
		t.Fatalf("API JSON leaked or failed to redact secret: %s", jsonResponse.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonResponse.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("redacted API output is invalid JSON: %v", err)
	}
	errorResponse := httptest.NewRecorder()
	apiError(errorResponse, http.StatusBadRequest, "TEST_ERROR", "password="+secret)
	if strings.Contains(errorResponse.Body.String(), secret) || !strings.Contains(errorResponse.Body.String(), "[REDACTED]") {
		t.Fatalf("API error leaked or failed to redact secret: %s", errorResponse.Body.String())
	}
}

func TestAPIErrorEnvelopeIncludesCorrelatedRequestIdentity(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/runs?limit=invalid", nil)
	r.Host = "127.0.0.1"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var body struct {
		Error struct {
			Code      string         `json:"code"`
			Message   string         `json:"message"`
			Details   map[string]any `json:"details"`
			RequestID string         `json:"request_id"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusBadRequest || body.Error.Code != "INVALID_PAGE" || len(body.Error.RequestID) != 32 || body.Error.RequestID != w.Header().Get("X-Request-ID") || body.Error.RequestID != body.RequestID || body.Error.Details == nil {
		t.Fatalf("request-correlated error response=%+v header=%q status=%d", body, w.Header().Get("X-Request-ID"), w.Code)
	}
}

func TestRunHistoryPaginationIsBoundedAndReportsMetadata(t *testing.T) {
	s := testServer(t)
	for i := 0; i < 105; i++ {
		if _, err := s.Store.CreateRun(context.Background(), filepath.Join(t.TempDir(), "project"), fmt.Sprintf("run-%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	call := func(target string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+target, nil)
		r.Host = "127.0.0.1"
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	first := call("/api/v1/runs")
	var page []model.Run
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if first.Code != http.StatusOK || len(page) != 100 || first.Header().Get("X-Total-Count") != "105" || first.Header().Get("X-Page-Limit") != "100" || first.Header().Get("X-Page-Offset") != "0" || first.Header().Get("X-Has-More") != "true" {
		t.Fatalf("default page count=%d status=%d headers=%v", len(page), first.Code, first.Header())
	}
	second := call("/api/v1/runs?limit=10&offset=100")
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if second.Code != http.StatusOK || len(page) != 5 || second.Header().Get("X-Has-More") != "false" {
		t.Fatalf("last page count=%d status=%d headers=%v", len(page), second.Code, second.Header())
	}
	if invalid := call("/api/v1/runs?limit=101"); invalid.Code != http.StatusBadRequest {
		t.Fatalf("oversized page status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestCapabilitiesEndpointExposesOnlyPersistedNonSecretEvidence(t *testing.T) {
	s := testServer(t)
	profile := model.Profile{Name: "safe-codex", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{"/workspace"}}
	if _, err := s.Store.SaveProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	reference, err := security.ResolveExecutableReference(profile.Executable)
	if err != nil {
		t.Fatal(err)
	}
	capabilityEvidence := model.ProfileCapability{Provider: "codex", ExecutableFound: true, ApprovedExecutablePath: reference.Path, ExecutableSHA256: reference.SHA256, Authenticated: "AUTHENTICATED", ReadOnlyExecution: true, WorkspaceWrite: true, ModelCatalogStatus: "VERIFIED", ModelValues: []string{"gpt-test"}, EffortValues: []string{"high"}, ObservedAt: time.Now().UTC()}
	if _, err = s.Store.SaveProfileCapability(context.Background(), profile.Name, capabilityEvidence); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/capabilities", nil)
	r.Host = "127.0.0.1"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"authenticated":"AUTHENTICATED"`) || !strings.Contains(w.Body.String(), `"models":["gpt-test"]`) || !strings.Contains(w.Body.String(), `"fake":"SIMULATED_READY"`) || strings.Contains(w.Body.String(), "/private/provider-bin") || strings.Contains(w.Body.String(), strings.Repeat("a", 64)) {
		t.Fatalf("capability response status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestConfigurationValidationEndpointUsesRoutePreviewWithoutRunCreation(t *testing.T) {
	s := testServer(t)
	project := t.TempDir()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/config/validate", strings.NewReader(`{"project":"`+project+`","prompt":"inspect"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"valid":true`) || !strings.Contains(w.Body.String(), `"execution":"NOT_RUN"`) {
		t.Fatalf("validation status=%d body=%s", w.Code, w.Body.String())
	}
	if runs, err := s.Store.Runs(context.Background()); err != nil || len(runs) != 0 {
		t.Fatalf("validation created runs=%d err=%v", len(runs), err)
	}
}

func TestAPIRejectsWrongMethodsAndExtraResourceSegments(t *testing.T) {
	s := testServer(t)
	run, err := s.Store.CreateRun(context.Background(), t.TempDir(), "route methods")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/v1/runs/" + run.ID, http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/runs/" + run.ID + "/report", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/runs/" + run.ID + "/events", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/runs/" + run.ID + "/cancel/extra", http.StatusNotFound},
		{http.MethodPost, "/api/v1/capabilities", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/not-a-resource", http.StatusNotFound},
	} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1"+tc.path, nil)
		r.Host = "127.0.0.1"
		r.Header.Set("X-SpecTide-CSRF", s.CSRF)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s status=%d want=%d body=%s", tc.method, tc.path, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestRunStageArtifactAndReportProjectionsAreExplicit(t *testing.T) {
	s := testServer(t)
	run, err := s.Store.CreateRun(context.Background(), t.TempDir(), "project projections")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AddStage(context.Background(), model.Stage{ID: "projection-stage", RunID: run.ID, Key: "analyze", Position: 1, State: model.Pending, Provider: "fake", Profile: "offline", Model: "", Effort: "", Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.SaveArtifact(context.Background(), model.Artifact{RunID: run.ID, Stage: "analyze", Path: "specs/001/spec.md", Kind: "markdown", SHA256: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, expected string }{
		{"stages", `"key":"analyze"`}, {"artifacts", `"path":"specs/001/spec.md"`}, {"report", `"acceptance":`},
	} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/runs/"+run.ID+"/"+tc.path, nil)
		r.Host = "127.0.0.1"
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.expected) {
			t.Errorf("%s projection status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
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

func TestSecretPromptIsRedactedAcrossRunAPIReportAndSSE(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := testServer(t)
	project := t.TempDir()
	secret := "synthetic-api-token-1234567890"
	prompt := "Build a status page with api_key=" + secret
	body, err := json.Marshal(map[string]string{"project": project, "prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(string(body)))
	request.Host = "127.0.0.1"
	request.Header.Set("X-SpecTide-CSRF", server.CSRF)
	request.Header.Set("Idempotency-Key", "secret-prompt-123")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) || !strings.Contains(response.Body.String(), "[REDACTED]") {
		t.Fatalf("create response leaked or failed to redact prompt secret: %s", response.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("created run=%+v err=%v", created, err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		run, getErr := server.Store.GetRun(context.Background(), created.ID)
		if getErr == nil && run.State == model.Completed {
			break
		}
		if getErr == nil && (run.State == model.Blocked || run.State == model.Failed) {
			t.Fatalf("offline API run did not complete: %+v", run)
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, err := server.Store.GetRun(context.Background(), created.ID)
	if err != nil || run.State != model.Completed || strings.Contains(run.Prompt, secret) || !strings.Contains(run.Prompt, "[REDACTED]") {
		t.Fatalf("persisted run=%+v err=%v", run, err)
	}
	gateSecret, approvalSecret := "api-gate-secret-123456", "api-approval-secret-654321"
	if err = server.Store.Gate(context.Background(), created.ID, "secret_check", "FAIL", "api_key="+gateSecret); err != nil {
		t.Fatal(err)
	}
	if _, err = server.Store.CreateApproval(context.Background(), model.Approval{RunID: created.ID, Action: "token=" + approvalSecret}); err != nil {
		t.Fatal(err)
	}
	reportRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/runs/"+created.ID+"/report", nil)
	reportRequest.Host = "127.0.0.1"
	reportResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(reportResponse, reportRequest)
	if reportResponse.Code != http.StatusOK || strings.Contains(reportResponse.Body.String(), secret) || strings.Contains(reportResponse.Body.String(), gateSecret) || strings.Contains(reportResponse.Body.String(), approvalSecret) {
		t.Fatalf("report status=%d body=%s", reportResponse.Code, reportResponse.Body.String())
	}
	streamContext, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	streamRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/runs/"+created.ID+"/events?after=0", nil).WithContext(streamContext)
	streamRequest.Host = "127.0.0.1"
	streamResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(streamResponse, streamRequest)
	if streamResponse.Code != http.StatusOK || strings.Contains(streamResponse.Body.String(), secret) {
		t.Fatalf("event stream status=%d body=%s", streamResponse.Code, streamResponse.Body.String())
	}
	var rawPrompt string
	if err = server.Store.DB.QueryRowContext(context.Background(), `SELECT prompt FROM runs WHERE id=?`, created.ID).Scan(&rawPrompt); err != nil || strings.Contains(rawPrompt, secret) {
		t.Fatalf("database prompt leaked secret %q err=%v", rawPrompt, err)
	}
}

func TestRestartResumeDriftIsBlockedThroughAPI(t *testing.T) {
	server := testServer(t)
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspaceFile := filepath.Join(project, "preserve.txt")
	if err = os.WriteFile(workspaceFile, []byte("keep this evidence\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run, err := server.Store.CreateRun(context.Background(), project, "interrupted API recovery")
	if err != nil {
		t.Fatal(err)
	}
	if err = server.Store.AcquireWorkspaceLockOwned(context.Background(), project, run.ID, "expired-api-owner"); err != nil {
		t.Fatal(err)
	}
	if err = server.Store.SetRunState(context.Background(), run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = server.Store.SetRunState(context.Background(), run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = server.Store.Event(context.Background(), run.ID, "constitution", "run.paused", "info", map[string]any{"stage": "constitution"}); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = server.Store.DB.ExecContext(context.Background(), `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs/"+run.ID+"/resume", nil)
	request.Host = "127.0.0.1"
	request.Header.Set("X-SpecTide-CSRF", server.CSRF)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "WORKSPACE_DRIFT") {
		t.Fatalf("restart API status=%d body=%s", response.Code, response.Body.String())
	}
	got, err := server.Store.GetRun(context.Background(), run.ID)
	if err != nil || got.State != model.Blocked {
		t.Fatalf("run=%+v err=%v", got, err)
	}
	content, err := os.ReadFile(workspaceFile)
	if err != nil || string(content) != "keep this evidence\n" {
		t.Fatalf("API recovery overwrote workspace %q err=%v", content, err)
	}
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

func TestDryRunBlocksLiveRouteWithoutAuthorizedStoredProfile(t *testing.T) {
	s := testServer(t)
	project := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "missing-profile"}
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
	r.Header.Set("Idempotency-Key", "preview-profile-123")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "PROFILE_NOT_FOUND") || !strings.Contains(w.Body.String(), `"execution":"NOT_RUN"`) || !strings.Contains(w.Body.String(), `"blocked_reasons":[`) {
		t.Fatalf("unauthorized route preview status=%d body=%s", w.Code, w.Body.String())
	}
	validation := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/config/validate", strings.NewReader(`{"project":"`+project+`","prompt":"inspect"}`))
	validation.Host = "127.0.0.1"
	validation.Header.Set("X-SpecTide-CSRF", s.CSRF)
	validationResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(validationResponse, validation)
	if validationResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(validationResponse.Body.String(), "PROFILE_NOT_FOUND") || !strings.Contains(validationResponse.Body.String(), `"execution":"NOT_RUN"`) {
		t.Fatalf("config validation did not match preview blocker: status=%d body=%s", validationResponse.Code, validationResponse.Body.String())
	}
	if runs, err := s.Store.Runs(context.Background()); err != nil || len(runs) != 0 {
		t.Fatalf("blocked preview/validation created runs=%d err=%v", len(runs), err)
	}
}

func TestDryRunRejectsLiveProfileWithoutPersistedCapability(t *testing.T) {
	s := testServer(t)
	project := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "local-codex"}
	if err := config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.SaveProfile(context.Background(), model.Profile{Name: "local-codex", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}}); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"project": project, "prompt": "inspect", "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(string(body)))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "preview-capability-123")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "PROFILE_CAPABILITY_NOT_PROBED") {
		t.Fatalf("preview status=%d body=%s", w.Code, w.Body.String())
	}
	if runs, err := s.Store.Runs(context.Background()); err != nil || len(runs) != 0 {
		t.Fatalf("capability-rejected preview created runs=%d err=%v", len(runs), err)
	}
}

func TestDryRunRejectsFallbackOutsideProjectBoundaryWithoutCreatingRun(t *testing.T) {
	s := testServer(t)
	project := t.TempDir()
	outside := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute = config.Route{
		Provider: "codex", Profile: "primary", FallbackAuthorized: true,
		Fallback: []config.Route{{Provider: "claude", Profile: "backup"}},
	}
	if err := config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []model.Profile{
		{Name: "primary", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}},
		{Name: "backup", Provider: "claude", Executable: "claude", AllowedScopes: []string{outside}},
	} {
		if _, err := s.Store.SaveProfile(context.Background(), profile); err != nil {
			t.Fatal(err)
		}
	}
	body, err := json.Marshal(map[string]any{"project": project, "prompt": "inspect", "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs", strings.NewReader(string(body)))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	r.Header.Set("Idempotency-Key", "preview-fallback-scope-123")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "FALLBACK_PROFILE_NOT_ALLOWED") {
		t.Fatalf("cross-boundary fallback preview status=%d body=%s", w.Code, w.Body.String())
	}
	if runs, err := s.Store.Runs(context.Background()); err != nil || len(runs) != 0 {
		t.Fatalf("rejected fallback preview created runs=%d err=%v", len(runs), err)
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
	otherRun, err := s.Store.CreateRun(context.Background(), root, "different run")
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs/"+otherRun.ID+"/approvals/"+approval.ID, strings.NewReader(`{"decision":"approved"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("mismatched run approval=%d %s", w.Code, w.Body.String())
	}
	pending, err := s.Store.Approval(context.Background(), run.ID, approval.ID)
	if err != nil || pending.Status != model.ApprovalPending {
		t.Fatalf("mismatched URL altered approval=%+v err=%v", pending, err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs/"+run.ID+"/approvals/"+approval.ID, strings.NewReader(`{"decision":"approved"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "APPROVED") {
		t.Fatalf("resolve=%d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/runs/"+run.ID+"/approvals/"+approval.ID, strings.NewReader(`{"decision":"approved"}`))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate approval=%d %s", w.Code, w.Body.String())
	}
}

func TestProjectAndArtifactSecretsStayRedactedThroughAPIAndReport(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	projectSecret := "api-project-secret-123456"
	body := `{"path":"` + root + `","name":"workspace password=` + projectSecret + `"}`
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/projects", strings.NewReader(body))
	r.Host = "127.0.0.1"
	r.Header.Set("X-SpecTide-CSRF", s.CSRF)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated || strings.Contains(w.Body.String(), projectSecret) || !strings.Contains(w.Body.String(), "[REDACTED]") {
		t.Fatalf("project response leaked or failed to redact display name: status=%d body=%s", w.Code, w.Body.String())
	}
	projects, err := s.Store.Projects(context.Background())
	if err != nil || len(projects) != 1 || strings.Contains(projects[0].DisplayName, projectSecret) {
		t.Fatalf("project persistence leaked display name: projects=%+v err=%v", projects, err)
	}
	run, err := s.Store.CreateRun(context.Background(), root, "report fixture")
	if err != nil {
		t.Fatal(err)
	}
	artifactSecret := "api-artifact-secret-654321"
	if _, err := s.Store.SaveArtifact(context.Background(), model.Artifact{RunID: run.ID, Stage: "specify", Path: "specs/spec.md", Kind: "source token=" + artifactSecret, SHA256: strings.Repeat("c", 64)}); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/runs/"+run.ID+"/report", nil)
	r.Host = "127.0.0.1"
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), artifactSecret) || !strings.Contains(w.Body.String(), "[REDACTED]") {
		t.Fatalf("report response leaked or failed to redact artifact metadata: status=%d body=%s", w.Code, w.Body.String())
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

func TestSSEReceivesProviderStreamEventBeforeProcessExits(t *testing.T) {
	s := testServer(t)
	run, err := s.Store.CreateRun(context.Background(), t.TempDir(), "stream provider activity")
	if err != nil {
		t.Fatal(err)
	}
	prior, err := s.Store.Events(context.Background(), run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var after int64
	for _, event := range prior {
		if event.Seq > after {
			after = event.Seq
		}
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	response, err := server.Client().Get(fmt.Sprintf("%s/api/v1/runs/%s/events?after=%d", server.URL, run.ID, after))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("SSE status=%d", response.StatusCode)
	}
	stream := adapters.NewOutputStream(adapters.Codex)
	stream.SetEventCallback(func(event adapters.Event) {
		if eventErr := s.Store.Event(context.Background(), run.ID, "plan", "provider.event", "info", map[string]any{
			"provider_event_type": event.Type, "attempt_id": "attempt-live", "summary": event.Summary,
		}); eventErr != nil {
			t.Errorf("persist provider event: %v", eventErr)
		}
	})
	processDone := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(context.Background(), runner.Invocation{
			Binary: "/bin/sh", Args: []string{"-c", `printf '%s\n' '{"type":"item.started","item":{"type":"agent_message"}}'; sleep 0.8; printf '%s\n' '{"type":"turn.completed","last_agent_message":"finished"}'`},
			Dir: t.TempDir(), Timeout: 3 * time.Second, StreamStdout: stream.Feed,
		})
		if closeErr := stream.Close(); runErr == nil {
			runErr = closeErr
		}
		processDone <- runErr
	}()
	scanner := bufio.NewScanner(response.Body)
	deadline := time.After(600 * time.Millisecond)
	var received bool
	for !received {
		line := make(chan string, 1)
		scanDone := make(chan bool, 1)
		go func() { scanDone <- scanner.Scan(); line <- scanner.Text() }()
		select {
		case ok := <-scanDone:
			if !ok {
				t.Fatalf("SSE ended before provider event: %v", scanner.Err())
			}
			frame := <-line
			if strings.Contains(frame, `"provider_event_type":"codex.item.started"`) {
				received = true
			}
		case <-deadline:
			t.Fatal("SSE client did not receive provider activity while the process was active")
		}
	}
	select {
	case err = <-processDone:
		t.Fatalf("provider process exited before its event was observed: %v", err)
	default:
	}
	if err = <-processDone; err != nil {
		t.Fatal(err)
	}
}
