package quality

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/runner"
)

type fakeExecutor struct {
	results []runner.Result
	errors  []error
	seen    []runner.Invocation
}

func (f *fakeExecutor) Run(_ context.Context, inv runner.Invocation) (runner.Result, error) {
	f.seen = append(f.seen, inv)
	i := len(f.seen) - 1
	return f.results[i], f.errors[i]
}

func TestRegistryCollectsRequiredEvidence(t *testing.T) {
	executor := &fakeExecutor{results: []runner.Result{
		{ExitCode: 0, Stdout: "", Duration: time.Millisecond},
		{ExitCode: 2, Stderr: "vet finding", Duration: time.Millisecond},
		{ExitCode: 0, Stdout: "", Duration: time.Millisecond},
	}, errors: []error{nil, errors.New("exit code 2"), nil}}
	registry := NewRegistry(executor)
	for _, check := range []Check{
		{Key: "format", Binary: "format-tool", Required: true, RequireEmptyOut: true},
		{Key: "vet", Binary: "vet-tool", Required: true},
		{Key: "lint", Binary: "lint-tool", Required: false},
	} {
		if err := registry.Register(check); err != nil {
			t.Fatal(err)
		}
	}
	evidence := registry.Run(context.Background(), t.TempDir())
	if len(evidence) != 3 || evidence[0].Verdict != Pass || evidence[1].Verdict != Fail || evidence[2].Verdict != Pass {
		t.Fatalf("evidence=%+v", evidence)
	}
	if registry.RequiredPassed(evidence) {
		t.Fatal("failed required check reported success")
	}
	if filepath.IsAbs(executor.seen[0].Dir) == false {
		t.Fatal("runner did not receive workspace root")
	}
}

func TestRegistryBoundsFormattingOutputAndRejectsDuplicateKeys(t *testing.T) {
	executor := &fakeExecutor{results: []runner.Result{{ExitCode: 0, Stdout: "internal/file.go"}}, errors: []error{nil}}
	registry := NewRegistry(executor)
	check := Check{Key: "format", Binary: "gofmt", Required: true, RequireEmptyOut: true}
	if err := registry.Register(check); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(check); err == nil {
		t.Fatal("duplicate quality key accepted")
	}
	evidence := registry.Run(context.Background(), t.TempDir())
	if evidence[0].Verdict != Fail {
		t.Fatalf("evidence=%+v", evidence[0])
	}
}
