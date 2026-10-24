package quality

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestDiscoveredChecksCarryPinnedCanonicalWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/pinned\ngo 1.27.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{results: make([]runner.Result, 4), errors: make([]error, 4)}
	registry, err := Discover(root, executor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registry.RunControlled(context.Background(), root, nil, nil); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, invocation := range executor.seen {
		if invocation.ExpectedWorkingDirectory != canonical {
			t.Fatalf("quality invocation was not pinned: %+v want %q", invocation, canonical)
		}
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

func TestDiscoverRejectsManifestSymlinksEscapingProjectRoot(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(external, []byte(`{"scripts":{"test":"echo external"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "package.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root, nil); err == nil || !strings.Contains(err.Error(), "QUALITY_CHECK_MANIFEST_PATH_INVALID") {
		t.Fatalf("escaped package manifest accepted: %v", err)
	}
}

func TestDiscoverReevaluatesFreshGoAndNodeProjectManifests(t *testing.T) {
	goRoot := t.TempDir()
	initial, err := Discover(goRoot, nil)
	if err != nil || len(initial.Checks()) != 1 || initial.Checks()[0].Key != "project_quality" {
		t.Fatalf("initial empty project checks=%+v err=%v", initial, err)
	}
	if err = os.WriteFile(filepath.Join(goRoot, "go.mod"), []byte("module example.test/fresh\ngo 1.27.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	derivedGo, err := Discover(goRoot, nil)
	if err != nil || len(derivedGo.Checks()) != 4 || derivedGo.Checks()[0].Key != "gofmt" {
		t.Fatalf("derived Go checks=%+v err=%v", derivedGo.Checks(), err)
	}

	nodeRoot := t.TempDir()
	if err = os.WriteFile(filepath.Join(nodeRoot, "package.json"), []byte(`{"scripts":{"test":"vitest run","build":"vite build"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(nodeRoot, "pnpm-lock.yaml"), []byte("lockfileVersion: '9.0'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	derivedNode, err := Discover(nodeRoot, nil)
	if err != nil || len(derivedNode.Checks()) != 2 || derivedNode.Checks()[0].Binary != "pnpm" || derivedNode.Checks()[1].Binary != "pnpm" {
		t.Fatalf("derived Node checks=%+v err=%v", derivedNode.Checks(), err)
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
