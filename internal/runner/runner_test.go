package runner

import (
	"context"
	"fmt"
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

func TestRunnerEscalatesAfterGroupLeaderExits(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "descendant-writes")
	pidFile := filepath.Join(dir, "descendant.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	script := `sh -c 'trap "" TERM; while :; do printf x >> "$1"; sleep 0.01; done' sh "$1" & echo $! > "$2"; exit 0`
	_, err := Run(ctx, Invocation{Binary: "/bin/sh", Args: []string{"-c", script, "sh", marker, pidFile}})
	if err == nil || !strings.Contains(err.Error(), "RUN_CANCELLED") {
		t.Fatalf("expected cancelled run after early group-leader exit, got %v", err)
	}
	before, err := os.Stat(marker)
	if err != nil || before.Size() == 0 {
		t.Fatalf("descendant did not write before escalation: info=%v err=%v", before, err)
	}
	time.Sleep(120 * time.Millisecond)
	after, err := os.Stat(marker)
	if err != nil || after.Size() != before.Size() {
		t.Fatalf("descendant kept writing after Run returned: before=%v after=%v err=%v", before.Size(), after, err)
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
	if !r.StdoutTruncated {
		t.Fatal("runner did not report stdout truncation to stream consumer")
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

func TestRunnerStreamsOnlyRetainedStdoutWhileProcessRuns(t *testing.T) {
	var streamed strings.Builder
	seenAt := time.Time{}
	r, err := Run(context.Background(), Invocation{Binary: "/bin/sh", Args: []string{"-c", "printf 'one'; sleep 0.05; printf 'two'"}, StreamStdout: func(chunk []byte) error {
		if seenAt.IsZero() {
			seenAt = time.Now()
		}
		streamed.Write(chunk)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != "onetwo" || !strings.Contains(r.Stdout, "onetwo") || seenAt.IsZero() || !seenAt.Before(r.FinishedAt) {
		t.Fatalf("stream=%q stdout=%q", streamed.String(), r.Stdout)
	}
}

func TestRunnerPreservesExitCodeWhenStreamParserFails(t *testing.T) {
	result, err := Run(context.Background(), Invocation{
		Binary: "/bin/sh", Args: []string{"-c", "printf invalid; exit 7"},
		StreamStdout: func([]byte) error { return fmt.Errorf("invalid provider stream") },
	})
	if err == nil || !strings.Contains(err.Error(), "PROVIDER_STREAM_INVALID") || result.ExitCode != 7 {
		t.Fatalf("stream failure lost process result: result=%+v err=%v", result, err)
	}
}

func TestRunnerPreservesProcessExitEvidenceOnCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := Run(ctx, Invocation{Binary: "/bin/sh", Args: []string{"-c", `trap 'exit 42' TERM; while :; do sleep 0.01; done`}})
	if err == nil || !strings.Contains(err.Error(), "RUN_CANCELLED") {
		t.Fatalf("expected cancellation, result=%+v err=%v", result, err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("cancellation lost the child exit status: %+v", result)
	}
}

func TestRunnerPreservesExitCodeOnStageTimeout(t *testing.T) {
	result, err := Run(context.Background(), Invocation{
		Binary: "/bin/sh", Args: []string{"-c", `trap 'exit 43' TERM; while :; do sleep 0.01; done`}, Timeout: minStageTimeout,
	})
	if err == nil || !strings.Contains(err.Error(), "STAGE_TIMEOUT") || result.ExitCode != 43 {
		t.Fatalf("timeout lost process exit status: result=%+v err=%v", result, err)
	}
}

func TestRunnerRejectsWorkspacePathReplacedBeforeInvocation(t *testing.T) {
	parent := t.TempDir()
	approved := filepath.Join(parent, "workspace")
	outside := t.TempDir()
	marker := filepath.Join(outside, "ran")
	if err := os.Mkdir(approved, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(approved, filepath.Join(parent, "renamed-workspace")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, approved); err != nil {
		t.Fatal(err)
	}
	approvedCanonical, err := filepath.EvalSymlinks(filepath.Join(parent, "renamed-workspace"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Run(context.Background(), Invocation{Binary: "/usr/bin/touch", Args: []string{marker}, Dir: approved, ExpectedWorkingDirectory: approvedCanonical})
	if err == nil || !strings.Contains(err.Error(), "WORKING_DIRECTORY_REFERENCE_DRIFT") {
		t.Fatalf("replaced workspace launched: %v", err)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("outside provider command ran: %v", err)
	}
}
