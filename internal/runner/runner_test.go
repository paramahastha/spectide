package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerDoesNotUseShellAndRedacts(t *testing.T) {
	r, e := Run(context.Background(), Invocation{Binary: "/bin/echo", Args: []string{"api_key=abcdefghijklmno"}})
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(r.Stdout, "abcdefghijklmno") {
		t.Fatalf("unredacted %q", r.Stdout)
	}
}

func TestRunnerCancellationCleansUpProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child-finished")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, Invocation{Binary: "/bin/sh", Args: []string{"-c", "sleep 0.4; touch " + marker}})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	time.Sleep(500 * time.Millisecond)
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child process survived cancellation and created marker: %v", err)
	}
}
func TestRunnerRejectsRelativeDir(t *testing.T) {
	_, e := Run(context.Background(), Invocation{Binary: "echo", Dir: "."})
	if e == nil {
		t.Fatal("expected rejection")
	}
}

func TestRunnerBoundsOutputAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	r, err := Run(ctx, Invocation{Binary: "/usr/bin/yes", Args: []string{"x"}})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if len(r.Stdout) > maxOutputBytes+128 {
		t.Fatalf("stdout exceeded cap: %d", len(r.Stdout))
	}
	if !strings.Contains(r.Stdout, "output truncated") {
		t.Fatal("truncation marker missing")
	}
}

func TestRunnerCapsStdoutAndStderrAtOneMiB(t *testing.T) {
	r, err := Run(context.Background(), Invocation{
		Binary: "/bin/sh",
		Args:   []string{"-c", `yes x | head -c 2097152; yes y | head -c 2097152 >&2`},
	})
	if err != nil {
		t.Fatalf("large-output command failed: %v", err)
	}
	for name, output := range map[string]string{"stdout": r.Stdout, "stderr": r.Stderr} {
		if len(output) > maxOutputBytes+128 {
			t.Errorf("%s exceeded cap: %d bytes", name, len(output))
		}
		if !strings.Contains(output, "output truncated at 1 MiB") {
			t.Errorf("%s truncation marker missing", name)
		}
	}
}

func TestRunnerStageTimeoutBounds(t *testing.T) {
	for _, timeout := range []time.Duration{-time.Second, minStageTimeout - time.Nanosecond, maxStageTimeout + time.Nanosecond} {
		_, err := Run(context.Background(), Invocation{Binary: "/bin/echo", Timeout: timeout})
		if err == nil || !strings.Contains(err.Error(), "INVALID_STAGE_TIMEOUT") {
			t.Fatalf("timeout %s: expected bounded timeout rejection, got %v", timeout, err)
		}
	}
	for _, timeout := range []time.Duration{minStageTimeout, maxStageTimeout} {
		if _, err := Run(context.Background(), Invocation{Binary: "/bin/echo", Timeout: timeout}); err != nil {
			t.Fatalf("timeout boundary %s was rejected: %v", timeout, err)
		}
	}
	if defaultStageTimeout != 30*time.Minute {
		t.Fatalf("default stage timeout = %s, want 30m", defaultStageTimeout)
	}
}

func TestRunnerStageTimeoutIsReported(t *testing.T) {
	_, err := Run(context.Background(), Invocation{Binary: "/bin/sleep", Args: []string{"5"}, Timeout: minStageTimeout})
	if err == nil || !strings.Contains(err.Error(), "STAGE_TIMEOUT") {
		t.Fatalf("expected stage timeout, got %v", err)
	}
}

func TestRunnerSendsGracefulTerminationBeforeEscalation(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "terminated-gracefully")
	_, err := Run(context.Background(), Invocation{
		Binary:  "/bin/sh",
		Args:    []string{"-c", `trap 'touch "$1"; exit 0' TERM; while :; do :; done`, "sh", marker},
		Timeout: minStageTimeout,
	})
	if err == nil || !strings.Contains(err.Error(), "STAGE_TIMEOUT") {
		t.Fatalf("expected stage timeout, got %v", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("process did not handle graceful termination before escalation: %v", statErr)
	}
}
