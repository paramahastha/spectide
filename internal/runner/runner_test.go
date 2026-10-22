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
