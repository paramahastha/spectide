package quality

import (
	"context"
	"errors"
	"os"
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

func TestDiscoverUsesGoProjectChecksWithoutSpecTideLayoutAssumptions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/other\ngo 1.27.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := Discover(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	checks := registry.Checks()
	if len(checks) != 4 || checks[0].Key != "gofmt" || checks[0].Args[0] != "-l" || checks[0].Args[1] != "." || checks[3].Args[1] != "./..." {
		t.Fatalf("Go checks=%+v", checks)
	}
}

func TestDiscoverUsesNodeProjectScriptsAndNoGoCommands(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"scripts":{"test":"vitest run","build":"vite build"}}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := Discover(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	checks := registry.Checks()
	if len(checks) != 2 || checks[0].Key != "node_tests" || checks[0].Binary != "npm" || checks[0].Args[0] != "test" || checks[1].Key != "node_build" {
		t.Fatalf("Node checks=%+v", checks)
	}
}

func TestDiscoverUsesPythonPytestMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\nname='fixture'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := Discover(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	checks := registry.Checks()
	if len(checks) != 1 || checks[0].Key != "python_tests" || checks[0].Binary != "python3" || len(checks[0].Args) != 3 || checks[0].Args[1] != "pytest" {
		t.Fatalf("Python checks=%+v", checks)
	}
}

func TestDiscoverReportsMissingScriptsCommandsAndUnsupportedProjectsAsBlocked(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := Discover(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := registry.Run(context.Background(), root)
	if len(evidence) != 1 || evidence[0].Verdict != Blocked || registry.RequiredPassed(evidence) {
		t.Fatalf("missing node script=%+v", evidence)
	}
	root = t.TempDir()
	registry, err = Discover(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence = registry.Run(context.Background(), root)
	if len(evidence) != 1 || evidence[0].Verdict != Blocked || evidence[0].Summary == "" {
		t.Fatalf("unsupported project=%+v", evidence)
	}
	missing := NewRegistry(nil)
	if err = missing.Register(Check{Key: "missing", Binary: "spectide-no-such-quality-command", Required: true}); err != nil {
		t.Fatal(err)
	}
	evidence = missing.Run(context.Background(), root)
	if len(evidence) != 1 || evidence[0].Verdict != Blocked {
		t.Fatalf("missing command=%+v", evidence)
	}
}
