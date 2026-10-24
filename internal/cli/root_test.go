package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/app"
	"github.com/spectide/spectide/internal/buildinfo"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
)

func executeCLI(t *testing.T, db string, args ...string) (string, error) {
	return executeCLIInput(t, db, "", args...)
}

func executeCLIInput(t *testing.T, db, input string, args ...string) (string, error) {
	t.Helper()
	root := Root()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(append([]string{"--db", db}, args...))
	err := root.Execute()
	return output.String(), err
}

func TestLegacyImportDeclineDoesNotCopyOrChangeSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "legacy.db")
	destination := filepath.Join(dir, "new", "workspace.db")
	original := []byte("legacy source must remain untouched")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	confirmed, err := confirmLegacyImport(bufio.NewReader(strings.NewReader("no\n")), &output, source, destination)
	if err != nil || confirmed {
		t.Fatalf("declined import confirmed=%v err=%v", confirmed, err)
	}
	if !strings.Contains(output.String(), source) || !strings.Contains(output.String(), destination) {
		t.Fatalf("confirmation prompt omitted paths: %s", output.String())
	}
	got, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("decline changed source: %q err=%v", got, err)
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("decline modified destination: %v", err)
	}
}

func TestSetupDeclinedLeavesProjectConfigAndWorkspaceUnchanged(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err = os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	db := filepath.Join(t.TempDir(), "workspace.db")
	_, err = executeCLIInput(t, db, "no\n", "setup")
	if err == nil || !strings.Contains(err.Error(), "SETUP_DECLINED") {
		t.Fatalf("declined setup error=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".spectide", "config.json")); !os.IsNotExist(statErr) {
		t.Fatalf("declined setup created configuration: %v", statErr)
	}
	if _, statErr := os.Stat(db); !os.IsNotExist(statErr) {
		t.Fatalf("declined setup created workspace: %v", statErr)
	}
}

func TestSetupConfirmsConfigThenReusesDashboardAndIsIdempotent(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err = os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	db := filepath.Join(t.TempDir(), "workspace.db")
	legacyState, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	legacyState.Close()
	state, err := app.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	statusServer := httptest.NewServer(state.API.Handler())
	previousURL := app.DefaultDashboardURL
	app.DefaultDashboardURL = statusServer.URL
	state.API.DashboardURL = statusServer.URL
	t.Cleanup(func() { app.DefaultDashboardURL = previousURL; statusServer.Close() })
	output, err := executeCLIInput(t, db, "yes\n", "setup")
	if err != nil || !strings.Contains(output, "SpecTide dashboard ready: "+statusServer.URL) {
		t.Fatalf("setup output=%s err=%v", output, err)
	}
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	configPath := config.ProjectPath(canonical)
	firstConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("confirmed setup did not create project config: %v", err)
	}
	output, err = executeCLIInput(t, db, "yes\n", "setup")
	if err != nil || !strings.Contains(output, "SpecTide dashboard ready") {
		t.Fatalf("repeat setup output=%s err=%v", output, err)
	}
	secondConfig, err := os.ReadFile(configPath)
	if err != nil || string(firstConfig) != string(secondConfig) {
		t.Fatalf("repeat setup changed project config: %v", err)
	}
	projects, err := state.Store.Projects(context.Background())
	if err != nil || len(projects) != 1 {
		t.Fatalf("repeat setup project count=%d err=%v", len(projects), err)
	}
}

func TestServePublishesActualBoundLoopbackURL(t *testing.T) {
	if os.Getenv("SPECTIDE_SERVE_URL_HELPER") == "1" {
		root := Root()
		root.SetArgs([]string{"--db", os.Getenv("SPECTIDE_SERVE_URL_DB"), "serve", "--port", "0"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		return
	}
	db := filepath.Join(t.TempDir(), "serve.db")
	command := exec.Command(os.Args[0], "-test.run=^TestServePublishesActualBoundLoopbackURL$")
	command.Env = append(os.Environ(), "SPECTIDE_SERVE_URL_HELPER=1", "SPECTIDE_SERVE_URL_DB="+db)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("serve did not publish its URL: %q err=%v", line, err)
	}
	const prefix = "SpecTide listening at http://127.0.0.1:"
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("serve URL line=%q", line)
	}
	port, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("serve published invalid port in %q: %v", line, err)
	}
	response, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/healthz")
	if err != nil {
		t.Fatalf("published URL is unreachable: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("published URL returned status %d", response.StatusCode)
	}
}

func TestServeCustomPortCollisionExplainsIsolation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	output, err := executeCLI(t, filepath.Join(t.TempDir(), "serve.db"), "serve", "--port", strconv.Itoa(port))
	if err == nil || !strings.Contains(err.Error(), "CUSTOM_PORT_OCCUPIED") || !strings.Contains(err.Error(), "choose another loopback port") {
		t.Fatalf("custom port collision output=%s err=%v", output, err)
	}
}

func TestSecondDefaultServeReportsMatchingWorkspaceDashboard(t *testing.T) {
	blocker, bindErr := net.Listen("tcp", "127.0.0.1:4747")
	if bindErr == nil {
		defer blocker.Close()
	} else if !strings.Contains(strings.ToLower(bindErr.Error()), "address already in use") {
		t.Skipf("cannot prepare default-port collision fixture: %v", bindErr)
	}
	db := filepath.Join(t.TempDir(), "serve.db")
	state, err := app.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	statusServer := httptest.NewServer(state.API.Handler())
	previousURL := app.DefaultDashboardURL
	app.DefaultDashboardURL = statusServer.URL
	state.API.DashboardURL = statusServer.URL
	t.Cleanup(func() { app.DefaultDashboardURL = previousURL; statusServer.Close() })
	output, err := executeCLI(t, db, "serve")
	if err != nil || !strings.Contains(output, "already has a verified dashboard at "+statusServer.URL) || !strings.Contains(output, "spectide dashboard") {
		t.Fatalf("second serve output=%s err=%v", output, err)
	}
}

func TestDashboardReusesVerifiedWorkspaceForOneHundredInvocations(t *testing.T) {
	db := filepath.Join(t.TempDir(), "state.db")
	previousURL := app.DefaultDashboardURL
	state, err := app.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var statusRequests atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/workspace/status" {
			statusRequests.Add(1)
		}
		state.API.Handler().ServeHTTP(w, r)
	})
	server := httptest.NewServer(handler)
	app.DefaultDashboardURL = server.URL
	state.API.DashboardURL = server.URL
	t.Cleanup(func() { app.DefaultDashboardURL = previousURL; server.Close() })

	timings := make([]time.Duration, 0, 100)
	for i := 0; i < 100; i++ {
		start := time.Now()
		output, runErr := executeCLI(t, db, "dashboard")
		timings = append(timings, time.Since(start))
		if runErr != nil || !strings.Contains(output, server.URL) {
			t.Fatalf("invocation %d output=%q err=%v", i+1, output, runErr)
		}
	}
	underFiveSeconds := 0
	var total, fastest, slowest time.Duration
	for _, duration := range timings {
		total += duration
		if fastest == 0 || duration < fastest {
			fastest = duration
		}
		if duration > slowest {
			slowest = duration
		}
		if duration < 5*time.Second {
			underFiveSeconds++
		}
	}
	if underFiveSeconds < 99 {
		t.Fatalf("%d/100 dashboard invocations reused the verified URL within five seconds", underFiveSeconds)
	}
	if got := statusRequests.Load(); got != 100 {
		t.Fatalf("verified status probes=%d; want 100 (an invocation may have attempted an unnecessary launch)", got)
	}
	t.Logf("dashboard invocations=100 reused=%d failures=%d under_5s=%d duplicate_launches=0 min=%s mean=%s max=%s", statusRequests.Load(), 0, underFiveSeconds, fastest, total/100, slowest)
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

func TestWorkflowListAndExplainAreJSONAndOutputControlsArePredictable(t *testing.T) {
	list, err := executeCLI(t, filepath.Join(t.TempDir(), "workflow.db"), "workflow", "list")
	if err != nil || !strings.Contains(list, `"stages":["constitution","specify"`) || strings.Contains(list, "\u001b[") {
		t.Fatalf("workflow list=%q err=%v", list, err)
	}
	var listed struct {
		Stages []string `json:"stages"`
	}
	if err = json.Unmarshal([]byte(list), &listed); err != nil || len(listed.Stages) != len(config.StageKeys) {
		t.Fatalf("workflow list JSON invalid: stages=%d err=%v", len(listed.Stages), err)
	}
	explain, err := executeCLI(t, filepath.Join(t.TempDir(), "workflow.db"), "workflow", "explain")
	if err != nil || !strings.Contains(explain, `"owner":"local"`) || !strings.Contains(explain, `"read_only":true`) {
		t.Fatalf("workflow explain=%q err=%v", explain, err)
	}
	quiet, err := executeCLI(t, filepath.Join(t.TempDir(), "workflow.db"), "--quiet", "--no-color", "workflow", "list")
	if err != nil || quiet != "" {
		t.Fatalf("quiet output=%q err=%v", quiet, err)
	}
	verbose, err := executeCLI(t, filepath.Join(t.TempDir(), "workflow.db"), "--verbose", "workflow", "list")
	if err != nil || !strings.Contains(verbose, "spectide: command=spectide workflow list") || !strings.Contains(verbose, `"stages"`) {
		t.Fatalf("verbose output=%q err=%v", verbose, err)
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
	canonical, canonErr := filepath.EvalSymlinks(project)
	if canonErr != nil {
		state.Close()
		t.Fatal(canonErr)
	}
	if err = state.RegisterProject(context.Background(), canonical, filepath.Base(canonical), []string{canonical}); err != nil {
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
