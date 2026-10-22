package runner

import (
	"context"
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
