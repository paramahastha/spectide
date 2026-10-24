package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/model"
)

var DefaultDashboardURL = "http://127.0.0.1:4747"

// EnsureDashboard reuses only a loopback endpoint that proves it belongs to
// this workspace. A started child receives only explicit command arguments.
func (a *App) EnsureDashboard(ctx context.Context, dbPath string) (string, error) {
	return a.ensureDashboard(ctx, dbPath, DefaultDashboardURL, "", acquireDashboardLease, nil)
}

// ensureDashboard isolates the launch mechanics so the real child process can
// be exercised on a disposable loopback port in acceptance tests.
func (a *App) ensureDashboard(ctx context.Context, dbPath, dashboardURL, executable string, acquireLease func() (string, error), onStart func(*exec.Cmd)) (string, error) {
	workspaceID, err := a.Store.WorkspaceID(ctx)
	if err != nil {
		return "", err
	}
	parsedURL, err := url.Parse(dashboardURL)
	if err != nil || parsedURL.Scheme != "http" || parsedURL.Hostname() != "127.0.0.1" || parsedURL.Port() == "" {
		return "", fmt.Errorf("DASHBOARD_URL_INVALID")
	}
	probe := func() (bool, error) { return ProbeDashboard(ctx, dashboardURL, workspaceID) }
	if ready, probeErr := probe(); ready || probeErr != nil {
		if probeErr != nil {
			return "", probeErr
		}
		return dashboardURL, nil
	}
	lease, err := acquireLease()
	if err != nil {
		return "", err
	}
	defer os.Remove(lease)
	if ready, probeErr := probe(); ready || probeErr != nil {
		if probeErr != nil {
			return "", probeErr
		}
		return dashboardURL, nil
	}
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}
	child := managedDashboardCommand(executable, dbPath, parsedURL.Hostname(), parsedURL.Port())
	child.Stdin = nil
	child.Stdout = io.Discard
	child.Stderr = io.Discard
	if err = child.Start(); err != nil {
		return "", fmt.Errorf("DASHBOARD_START_FAILED: %w", err)
	}
	if onStart != nil {
		onStart(child)
	}
	stopChild := func() { _ = child.Process.Kill() }
	if err = waitForDashboard(ctx, probe, 10*time.Second, 100*time.Millisecond, stopChild); err != nil {
		return "", err
	}
	return dashboardURL, nil
}

func waitForDashboard(ctx context.Context, probe func() (bool, error), timeout, interval time.Duration, stop func()) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ready, err := probe(); ready {
			return nil
		} else if err != nil {
			stop()
			return err
		}
		select {
		case <-ctx.Done():
			stop()
			return ctx.Err()
		case <-deadline.C:
			stop()
			return fmt.Errorf("DASHBOARD_READINESS_TIMEOUT: matching workspace did not become ready")
		case <-ticker.C:
		}
	}
}

func managedDashboardCommand(executable, dbPath, host, port string) *exec.Cmd {
	command := exec.Command(executable, "--db", dbPath, "serve", "--host", host, "--port", port)
	command.Dir = os.TempDir()
	command.Env = dashboardChildEnvironment(os.Environ())
	return command
}

// ProbeDashboard verifies that a loopback HTTP endpoint reports the expected
// workspace identity. Non-HTTP or foreign endpoints are never treated as ours.
func ProbeDashboard(ctx context.Context, baseURL, expectedWorkspaceID string) (bool, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/workspace/status", nil)
	client := &http.Client{Timeout: 350 * time.Millisecond}
	response, err := client.Do(request)
	if err != nil {
		return false, nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("DASHBOARD_PORT_OCCUPIED: workspace ownership could not be confirmed; use serve --port for an isolated dashboard")
	}
	var status model.WorkspaceStatus
	if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&status) != nil || status.WorkspaceID != expectedWorkspaceID || !status.LocalOnly {
		return false, fmt.Errorf("DASHBOARD_PORT_OCCUPIED: workspace ownership could not be confirmed; use serve --port for an isolated dashboard")
	}
	return true, nil
}

func dashboardChildEnvironment(source []string) []string {
	allowed := map[string]bool{"HOME": true, "USERPROFILE": true, "APPDATA": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true, "PATH": true, "SYSTEMROOT": true}
	result := make([]string, 0, len(allowed))
	for _, item := range source {
		key, _, ok := strings.Cut(item, "=")
		if ok && allowed[key] {
			result = append(result, item)
		}
	}
	return result
}

func acquireDashboardLease() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("DASHBOARD_RUNTIME_UNAVAILABLE")
	}
	dir := filepath.Join(base, "spectide", "runtime")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("DASHBOARD_RUNTIME_UNAVAILABLE")
	}
	if err = os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("DASHBOARD_RUNTIME_UNAVAILABLE")
	}
	path := filepath.Join(dir, "dashboard-4747.lease")
	return acquireDashboardLeaseAt(path, 10*time.Second)
}

func acquireDashboardLeaseAt(path string, wait time.Duration) (string, error) {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		file, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr == nil {
			_, _ = file.WriteString(strconv.Itoa(os.Getpid()))
			_ = file.Close()
			return path, nil
		}
		if !os.IsExist(createErr) {
			return "", fmt.Errorf("DASHBOARD_RUNTIME_UNAVAILABLE")
		}
		info, statErr := os.Stat(path)
		if statErr == nil && time.Since(info.ModTime()) > 20*time.Second {
			_ = os.Remove(path)
			continue
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", fmt.Errorf("DASHBOARD_LAUNCH_IN_PROGRESS: matching dashboard startup is taking longer than expected")
}
