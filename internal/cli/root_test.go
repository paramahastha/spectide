package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
)

func executeCLI(t *testing.T, db string, args ...string) (string, error) {
	t.Helper()
	root := Root()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(append([]string{"--db", db}, args...))
	err := root.Execute()
	return output.String(), err
}

func TestCLIJSONPresentationRedactsNestedSecretValues(t *testing.T) {
	var output bytes.Buffer
	secret := "cli-synthetic-secret-123456"
	if err := writeJSON(&output, map[string]any{"report": map[string]any{"evidence": "api_key=" + secret, "token": secret}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), secret) || !strings.Contains(output.String(), "[REDACTED]") {
		t.Fatalf("CLI JSON leaked or failed to redact secret: %s", output.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("redacted CLI output is invalid JSON: %v", err)
	}
}

func TestCLIReportPresentationRedactsPersistedEvidence(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, err := s.CreateRun(ctx, "demo", "inspect")
	if err != nil {
		t.Fatal(err)
	}
	gateSecret, approvalSecret := "cli-gate-secret-123456", "cli-approval-secret-654321"
	if err = s.Gate(ctx, run.ID, "required_tests", "FAIL", "api_key="+gateSecret); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: "token=" + approvalSecret}); err != nil {
		t.Fatal(err)
	}
	report, err := s.Report(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = writeJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), gateSecret) || strings.Contains(output.String(), approvalSecret) || !strings.Contains(output.String(), "[REDACTED]") {
		t.Fatalf("CLI report leaked evidence secrets or omitted redaction: %s", output.String())
	}
}

func TestCLIStatusLabelsExecutionFromImmutableRunRoutes(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "state.db")
	state, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := state.CreateRunWithConfiguration(ctx, "", "/workspace", "mixed execution", []byte(`{"schema_version":1,"stages":[{"stage":"analyze","decision":{"route":{"provider":"codex","profile":"live"}}},{"stage":"implement","decision":{"route":{"provider":"fake","profile":"offline"}}}]}`))
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, db, "status", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"execution":"MIXED"`) || !strings.Contains(output, `"verdict":"BLOCKED"`) {
		t.Fatalf("CLI omitted immutable execution/acceptance labels: %s", output)
	}
}

func TestRunStateExitCodesAreStable(t *testing.T) {
	cases := []struct {
		state model.RunState
		code  int
	}{
		{state: model.Blocked, code: ExitBlocked},
		{state: model.Failed, code: ExitFailed},
		{state: model.Cancelled, code: ExitCancelled},
		{state: model.Completed, code: ExitCommand},
	}
	for _, tc := range cases {
		if got := ExitCode(runStateError(tc.state)); got != tc.code {
			t.Errorf("state %s exit code=%d, want %d", tc.state, got, tc.code)
		}
	}
	if got := ExitCode(errors.New("invalid command")); got != ExitCommand {
		t.Fatalf("generic command error exit code=%d, want %d", got, ExitCommand)
	}
	if got := ExitCode(nil); got != ExitSuccess {
		t.Fatalf("success exit code=%d, want %d", got, ExitSuccess)
	}
}

func TestUnprobedLiveRouteIsDeniedBeforeRun(t *testing.T) {
	project := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute.Provider = "codex"
	cfg.DefaultRoute.Profile = "local-codex"
	if err := config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "spectide.db")
	state, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfile(context.Background(), model.Profile{Name: "local-codex", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}}); err != nil {
		state.Close()
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, db, "run", "--project", project, "inspect this project")
	if got := ExitCode(err); got != ExitCommand || !strings.Contains(err.Error(), "PROFILE_CAPABILITY_NOT_PROBED") {
		t.Fatalf("unprobed live route error=%v exit code=%d", err, got)
	}
	if !strings.Contains(output, "PROFILE_CAPABILITY_NOT_PROBED") {
		t.Fatalf("unprobed live route did not report capability denial: %s", output)
	}
	state, err = store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	if runs, e := state.Runs(context.Background()); e != nil || len(runs) != 0 {
		t.Fatalf("denied route created runs=%d err=%v", len(runs), e)
	}
}

func TestProjectRegisterCanonicalizesAndLists(t *testing.T) {
	db := filepath.Join(t.TempDir(), "spectide.db")
	projectDir := t.TempDir()
	canonicalPath, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, db, "projects", "register", projectDir)
	if err != nil {
		t.Fatal(err)
	}
	var registered struct {
		CanonicalPath string `json:"canonical_path"`
	}
	if err = json.Unmarshal([]byte(output), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.CanonicalPath != canonicalPath {
		t.Fatalf("registered canonical path=%q, want %q", registered.CanonicalPath, canonicalPath)
	}
	output, err = executeCLI(t, db, "projects", "list")
	if err != nil || !strings.Contains(output, canonicalPath) {
		t.Fatalf("projects list=%q err=%v", output, err)
	}
}

func TestApprovalCommandsCreateListAndResolve(t *testing.T) {
	db := filepath.Join(t.TempDir(), "spectide.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(context.Background(), t.TempDir(), "approval command test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, db, "approvals", "create", run.ID, "write workspace")
	if err != nil {
		t.Fatal(err)
	}
	var approval model.Approval
	if err = json.Unmarshal([]byte(output), &approval); err != nil || approval.ID == "" {
		t.Fatalf("approval=%+v err=%v output=%s", approval, err, output)
	}
	if _, err = executeCLI(t, db, "approvals", "resolve", approval.ID, "approved"); err != nil {
		t.Fatal(err)
	}
	output, err = executeCLI(t, db, "approvals", "list", run.ID)
	if err != nil || !strings.Contains(output, "APPROVED") {
		t.Fatalf("approval list=%q err=%v", output, err)
	}
	if _, err = executeCLI(t, db, "approvals", "resolve", approval.ID, "DENIED"); err == nil {
		t.Fatal("resolved approval was mutable")
	}
}

func TestProfileAndModelProbeDoNotClaimAuthentication(t *testing.T) {
	db := filepath.Join(t.TempDir(), "spectide.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveProfile(context.Background(), model.Profile{Name: "offline-echo", Provider: "codex", Executable: "/bin/echo", AllowedScopes: []string{"/tmp"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, db, "profiles", "probe", "offline-echo")
	if err != nil || !strings.Contains(output, `"authenticated":"UNKNOWN"`) || !strings.Contains(output, `"version":"--version"`) {
		t.Fatalf("profile probe=%q err=%v", output, err)
	}
	output, err = executeCLI(t, db, "models", "probe", "offline-echo")
	if err != nil || !strings.Contains(output, `"authentication":"UNKNOWN"`) || !strings.Contains(output, `"model_catalog":"UNAVAILABLE"`) {
		t.Fatalf("model probe=%q err=%v", output, err)
	}
	s, err = store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	profiles, err := s.Profiles(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0].Capability == nil || profiles[0].Capability.Authenticated != "UNKNOWN" {
		t.Fatalf("persisted probe capability=%+v err=%v", profiles, err)
	}
}
