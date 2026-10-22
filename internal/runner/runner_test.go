package runner

import (
	"context"
	"strings"
	"testing"
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
