package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/buildinfo"
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

func TestServeStartupReconcilesAbandonedRunWithoutStartingAnother(t *testing.T) {
	if os.Getenv("SPECTIDE_SERVE_RECOVERY_HELPER") == "1" {
		root := Root()
		root.SetArgs([]string{"--db", os.Getenv("SPECTIDE_SERVE_RECOVERY_DB"), "serve", "--port", "0"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		return
	}
	db := filepath.Join(t.TempDir(), "state.db")
	project := t.TempDir()
	state, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	run, err := state.CreateRun(context.Background(), project, "abandoned before daemon restart")
	if err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(context.Background(), run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(context.Background(), `INSERT INTO run_owners(run_id,owner_id,heartbeat_at) VALUES(?,?,?)`, run.ID, "terminated-owner", stale); err != nil {
		t.Fatal(err)
	}
	pausedProject := t.TempDir()
	pausedRun, err := state.CreateRun(context.Background(), pausedProject, "cancel a paused run after restart")
	if err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(context.Background(), pausedRun.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if _, err = state.CreateWorkflowApprovalAndPause(context.Background(), pausedRun.ID, "implement", "write implementation changes"); err != nil {
		t.Fatal(err)
	}
	if err = state.AcquireWorkspaceLockOwned(context.Background(), pausedProject, pausedRun.ID, "terminated-paused-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.RequestRunControl(context.Background(), pausedRun.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	if _, err = state.DB.ExecContext(context.Background(), `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, pausedRun.ID); err != nil {
		t.Fatal(err)
	}
	createdRun, reused, err := state.CreateRunWithConfigurationOwned(context.Background(), "created-before-worker", t.TempDir(), "recover a launch before RUNNING", []byte(`{}`), "terminated-launch-owner")
	if err != nil || reused {
		t.Fatalf("created launch=%+v reused=%v err=%v", createdRun, reused, err)
	}
	if _, err = state.DB.ExecContext(context.Background(), `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, createdRun.ID); err != nil {
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestServeStartupReconcilesAbandonedRunWithoutStartingAnother$")
	command.Env = append(os.Environ(), "SPECTIDE_SERVE_RECOVERY_HELPER=1", "SPECTIDE_SERVE_RECOVERY_DB="+db)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			<-done
		}
	}()
	reader, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		got, readErr := reader.GetRun(context.Background(), run.ID)
		cancelled, cancelErr := reader.GetRun(context.Background(), pausedRun.ID)
		created, createdErr := reader.GetRun(context.Background(), createdRun.ID)
		if readErr == nil && got.State == model.Blocked && cancelErr == nil && cancelled.State == model.Cancelled && createdErr == nil && created.State == model.Blocked {
			return
		}
		select {
		case <-done:
			t.Fatalf("serve exited before recovery: output=%s", output.String())
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("serve startup did not recover abandoned run without a new run: output=%s", output.String())
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

func TestVersionCommandUsesSharedBuildVersion(t *testing.T) {
	previous := buildinfo.Version
	buildinfo.Version = "9.8.7-test"
	t.Cleanup(func() { buildinfo.Version = previous })
	output, err := executeCLI(t, filepath.Join(t.TempDir(), "version.db"), "version")
	if err != nil || strings.TrimSpace(output) != "spectide 9.8.7-test" {
		t.Fatalf("CLI version=%q err=%v", output, err)
	}
}

func TestVerifyUsesProjectSpecificQualityDiscovery(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is unavailable")
	}
	project := t.TempDir()
	packageJSON := []byte(`{"scripts":{"test":"echo project-check"}}`)
	if err := os.WriteFile(filepath.Join(project, "package.json"), packageJSON, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, filepath.Join(t.TempDir(), "cli.db"), "verify", "--project", project)
	if err != nil || !strings.Contains(output, `"key":"node_tests"`) || strings.Contains(output, `"key":"unit_tests"`) {
		t.Fatalf("verify did not use target project checks: output=%s err=%v", output, err)
	}
}

func TestInitRejectsEscapedProjectMetadataDirectory(t *testing.T) {
	project, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(project, ".spectide")); err != nil {
		t.Fatal(err)
	}
	_, err := executeCLI(t, filepath.Join(t.TempDir(), "cli.db"), "init", "--project", project)
	if err == nil || !strings.Contains(err.Error(), "escapes canonical root") {
		t.Fatalf("init accepted external metadata directory: %v", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("init wrote outside project root: entries=%v err=%v", entries, readErr)
	}
}

func TestProjectListRedactsPersistedDisplayName(t *testing.T) {
	db := filepath.Join(t.TempDir(), "state.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	secret := "cli-project-secret-123456"
	if err := s.RegisterProject(context.Background(), filepath.Join(t.TempDir(), "workspace"), "workspace password="+secret, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, db, "projects", "list")
	if err != nil || strings.Contains(output, secret) || !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("project list leaked or failed to redact display metadata: output=%s err=%v", output, err)
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

func TestConfigPreviewAndValidateUseLaunchAuthorizationWithoutStartingRuns(t *testing.T) {
	type scenario struct {
		name      string
		configure func(string) (config.Config, []model.Profile)
		want      string
	}
	scenarios := []scenario{
		{
			name: "unauthorized direct route",
			configure: func(project string) (config.Config, []model.Profile) {
				cfg := config.Default()
				cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "direct"}
				return cfg, []model.Profile{{Name: "direct", Provider: "codex", Executable: "codex", AllowedScopes: []string{t.TempDir()}}}
			},
			want: "PROFILE_NOT_ALLOWED",
		},
		{
			name: "missing profile",
			configure: func(string) (config.Config, []model.Profile) {
				cfg := config.Default()
				cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "missing"}
				return cfg, nil
			},
			want: "PROFILE_NOT_FOUND",
		},
		{
			name: "unsupported capability",
			configure: func(project string) (config.Config, []model.Profile) {
				cfg := config.Default()
				cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "unprobed"}
				return cfg, []model.Profile{{Name: "unprobed", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}}}
			},
			want: "PROFILE_CAPABILITY_NOT_PROBED",
		},
		{
			name: "unauthorized fallback",
			configure: func(string) (config.Config, []model.Profile) {
				cfg := config.Default()
				cfg.DefaultRoute = config.Route{Provider: "fake", Profile: "fake-demo", FallbackAuthorized: true, Fallback: []config.Route{{Provider: "claude", Profile: "backup"}}}
				return cfg, []model.Profile{{Name: "backup", Provider: "claude", Executable: "claude", AllowedScopes: []string{t.TempDir()}}}
			},
			want: "FALLBACK_PROFILE_NOT_ALLOWED",
		},
	}
	for _, test := range scenarios {
		t.Run(test.name, func(t *testing.T) {
			project := t.TempDir()
			cfg, profiles := test.configure(project)
			configPath := config.ProjectPath(project)
			if err := config.Save(configPath, cfg); err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(t.TempDir(), "preview.db")
			state, err := store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			for _, profile := range profiles {
				if _, err = state.SaveProfile(context.Background(), profile); err != nil {
					state.Close()
					t.Fatal(err)
				}
			}
			if err = state.Close(); err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"preview", "validate"} {
				output, runErr := executeCLI(t, db, "config", command, "--project", project, "--file", configPath)
				if ExitCode(runErr) != ExitBlocked || !strings.Contains(output, test.want) || !strings.Contains(output, `"execution":"NOT_RUN"`) || !strings.Contains(output, `"blocked_reasons":[`) {
					t.Fatalf("%s did not report launch blocker: output=%s err=%v", command, output, runErr)
				}
			}
			state, err = store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			defer state.Close()
			if runs, err := state.Runs(context.Background()); err != nil || len(runs) != 0 {
				t.Fatalf("configuration check created runs=%d err=%v", len(runs), err)
			}
		})
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
