package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDashboardChildEnvironmentIsMinimized(t *testing.T) {
	env := dashboardChildEnvironment([]string{"HOME=/tmp/home", "PATH=/usr/bin", "OPENAI_API_KEY=must-not-pass", "AWS_SECRET_ACCESS_KEY=must-not-pass", "LD_PRELOAD=/tmp/inject.so"})
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "HOME=/tmp/home") || !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("required local environment was dropped: %v", env)
	}
	for _, secret := range []string{"OPENAI_API_KEY", "AWS_SECRET_ACCESS_KEY", "LD_PRELOAD"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("child environment retained %s: %v", secret, env)
		}
	}
}

func TestManagedDashboardCommandUsesExplicitArgumentsAndApprovedDirectory(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "must-not-pass")
	command := managedDashboardCommand("/bin/spectide", "/private/workspace/state.db", "127.0.0.1", "4747")
	want := []string{"/bin/spectide", "--db", "/private/workspace/state.db", "serve", "--host", "127.0.0.1", "--port", "4747"}
	if strings.Join(command.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("dashboard argv=%q want=%q", command.Args, want)
	}
	if command.Dir != os.TempDir() {
		t.Fatalf("dashboard child working directory=%q want temp directory %q", command.Dir, os.TempDir())
	}
	for _, value := range command.Env {
		if strings.HasPrefix(value, "OPENAI_API_KEY=") {
			t.Fatalf("dashboard environment retained provider secret: %s", value)
		}
	}
}

func TestEnsureDashboardStartsAndVerifiesManagedChild(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	dashboardURL := "http://127.0.0.1:" + strconv.Itoa(port)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "spectide")
	build := exec.Command("go", "build", "-o", executable, "./cmd/spectide")
	build.Dir = repoRoot
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("build disposable CLI: %v\n%s", buildErr, output)
	}

	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	a, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	leasePath := filepath.Join(t.TempDir(), "dashboard.lease")
	var child *exec.Cmd
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	gotURL, err := a.ensureDashboard(ctx, dbPath, dashboardURL, executable, func() (string, error) {
		return acquireDashboardLeaseAt(leasePath, time.Second)
	}, func(started *exec.Cmd) { child = started })
	if child != nil {
		t.Cleanup(func() {
			_ = child.Process.Kill()
			_ = child.Wait()
		})
	}
	if err != nil {
		t.Fatalf("managed child failed to become ready: %v", err)
	}
	if gotURL != dashboardURL {
		t.Fatalf("ready URL=%q want=%q", gotURL, dashboardURL)
	}
	if child == nil {
		t.Fatal("managed dashboard did not start a child process")
	}
	ready, err := ProbeDashboard(context.Background(), dashboardURL, mustWorkspaceID(t, a))
	if err != nil || !ready {
		t.Fatalf("managed child workspace probe ready=%v err=%v", ready, err)
	}
}

func mustWorkspaceID(t *testing.T, a *App) string {
	t.Helper()
	id, err := a.Store.WorkspaceID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestProbeDashboardRequiresExpectedWorkspaceIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"workspace_id":"workspace-a","version":"test","dashboard_url":"http://127.0.0.1:4747","local_only":true}`))
	}))
	defer server.Close()
	ready, err := ProbeDashboard(context.Background(), server.URL, "workspace-a")
	if err != nil || !ready {
		t.Fatalf("matching endpoint ready=%v err=%v", ready, err)
	}
	ready, err = ProbeDashboard(context.Background(), server.URL, "workspace-b")
	if err == nil || ready || !strings.Contains(err.Error(), "DASHBOARD_PORT_OCCUPIED") {
		t.Fatalf("foreign endpoint ready=%v err=%v", ready, err)
	}
}

func TestDashboardLeaseRecoversStaleFilesAndBoundsConcurrentWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "dashboard.lease")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := acquireDashboardLeaseAt(path, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = acquireDashboardLeaseAt(path, 60*time.Millisecond); err == nil || !strings.Contains(err.Error(), "DASHBOARD_LAUNCH_IN_PROGRESS") {
		t.Fatalf("competing launch did not wait for its bounded lease: %v", err)
	}
	if err = os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("stale-pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-21 * time.Second)
	if err = os.Chtimes(path, stale, stale); err != nil {
		t.Fatal(err)
	}
	recovered, err := acquireDashboardLeaseAt(path, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("stale lease was not recovered: %v", err)
	}
	if err = os.Remove(recovered); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardReadinessTimeoutStopsUnreadyChild(t *testing.T) {
	stopped := false
	err := waitForDashboard(context.Background(), func() (bool, error) { return false, nil }, 20*time.Millisecond, 2*time.Millisecond, func() { stopped = true })
	if err == nil || !strings.Contains(err.Error(), "DASHBOARD_READINESS_TIMEOUT") || !stopped {
		t.Fatalf("readiness timeout err=%v child_stopped=%v", err, stopped)
	}
}

func TestDefaultDBPathUsesPlatformLocationAndExplicitDBWins(t *testing.T) {
	got := DefaultDBPath()
	if !filepath.IsAbs(got) || !strings.Contains(got, "spectide") {
		t.Fatalf("default database path=%q", got)
	}
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		if want := filepath.Join(home, "Library", "Application Support", "spectide", "spectide.db"); got != want {
			t.Fatalf("macOS default path=%q want=%q", got, want)
		}
	}
	if runtime.GOOS == "linux" {
		dataHome := t.TempDir()
		t.Setenv("XDG_DATA_HOME", dataHome)
		if want := filepath.Join(dataHome, "spectide", "spectide.db"); DefaultDBPath() != want {
			t.Fatalf("Linux XDG default path=%q want=%q", DefaultDBPath(), want)
		}
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	legacyExpected := filepath.Join(configDir, "spectide", "spectide.db")
	if absolute, absErr := filepath.Abs(legacyExpected); absErr == nil && absolute != got {
		if LegacyDBPath() != absolute {
			t.Fatalf("legacy path=%q want=%q", LegacyDBPath(), absolute)
		}
	} else if LegacyDBPath() != "" {
		t.Fatalf("legacy path should be unavailable when it aliases the default: %s", LegacyDBPath())
	}
	explicit := filepath.Join(t.TempDir(), "isolated.db")
	a, err := Open(explicit)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err = os.Stat(explicit); err != nil {
		t.Fatalf("explicit --db path was ignored: %v", err)
	}
}

func TestDefaultDBPathResolverCoversLinuxMacOSAndLegacyPlatforms(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	configDir := filepath.Join(home, ".config")
	dataDir := filepath.Join(home, ".local", "share")
	cases := []struct {
		name, goos, dataHome, want string
	}{
		{"linux xdg", "linux", dataDir, filepath.Join(dataDir, "spectide", "spectide.db")},
		{"linux fallback", "linux", "relative-data", filepath.Join(home, ".local", "share", "spectide", "spectide.db")},
		{"mac application support", "darwin", "", filepath.Join(home, "Library", "Application Support", "spectide", "spectide.db")},
		{"other config location", "freebsd", "", filepath.Join(configDir, "spectide", "spectide.db")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultDBPathFor(tc.goos, home, configDir, tc.dataHome); got != tc.want {
				t.Fatalf("default path=%q want=%q", got, tc.want)
			}
		})
	}
}
