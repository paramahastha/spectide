package workflow

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/quality"
	"github.com/spectide/spectide/internal/store"
)

func deferredQualityConfiguration() effectiveRunConfiguration {
	return effectiveRunConfiguration{
		QualitySelectionPolicy: "DISCOVER_AFTER_IMPLEMENTATION",
		RequiredChecks: []requiredCheckReference{{
			Key: "project_quality", Binary: "spectide-unsupported-quality", Required: true,
			TimeoutSeconds: 120, UnavailableReason: "no supported project manifest or quality integration was detected",
		}},
	}
}

func TestDerivedQualityChecksFreezeAfterImplementationWithoutChangingRunConfiguration(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	project := t.TempDir()
	canonicalProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	deferred := deferredQualityConfiguration()
	deferred.QualityWorkspaceRoot = canonicalProject
	run, _, err := state.CreateRunWithConfiguration(ctx, "quality-freeze-run", project, "implement project", []byte(`{"frozen":"original"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/generated\ngo 1.27.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	engine := New(state)
	registry, err := engine.qualityRegistryForRun(ctx, run, deferred)
	if err != nil || len(registry.Checks()) != 4 || !hasQualityCheck(registry, "gofmt") || !hasQualityCheck(registry, "go_test") {
		t.Fatalf("derived Go checks=%+v err=%v", registry, err)
	}
	if registry.WorkingDirectory() != canonicalProject {
		t.Fatalf("deferred registry lost approved root: got=%q want=%q", registry.WorkingDirectory(), canonicalProject)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil || configuration.ConfigJSON != `{"frozen":"original"}` {
		t.Fatalf("runtime discovery mutated immutable configuration: %+v err=%v", configuration, err)
	}
	events, err := state.Events(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var derived int
	for _, event := range events {
		if event.Type == "quality.checks.derived" {
			derived++
			if event.Payload["provenance"] != "workspace project manifests discovered after implementation" || event.Payload["manifest_sha256"] == "" {
				t.Fatalf("derived check event lacks provenance or manifest hash: %+v", event.Payload)
			}
		}
	}
	if derived != 1 {
		t.Fatalf("derived check event count=%d", derived)
	}
	registry, err = engine.qualityRegistryForRun(ctx, run, deferred)
	if err != nil || len(registry.Checks()) != 4 {
		t.Fatalf("restored frozen checks=%+v err=%v", registry, err)
	}
	if registry.WorkingDirectory() != canonicalProject {
		t.Fatalf("restored registry lost approved root: got=%q want=%q", registry.WorkingDirectory(), canonicalProject)
	}
	if err = os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/changed\ngo 1.27.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.qualityRegistryForRun(ctx, run, deferred); err == nil {
		t.Fatal("changed project manifest reused frozen derived checks")
	}
}

func TestReconstructedQualityRegistryRejectsWorkspacePathReplacement(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is unavailable")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	marker := filepath.Join(outside, "check-ran")
	packageJSON, err := json.Marshal(map[string]any{"scripts": map[string]string{"test": "node -e \"require('fs').writeFileSync('" + marker + "','ran')\""}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "package.json"), packageJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	discovered, err := quality.Discover(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	decisions := make(map[string]config.Decision, len(stages))
	for _, stage := range stages {
		decisions[stage] = config.Decision{Stage: stage, Route: config.Route{Provider: "fake", Profile: "demo"}, Mode: "hybrid"}
	}
	effective := buildEffectiveRunConfiguration(config.Default(), decisions, nil, discovered)
	encoded, err := json.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := parseEffectiveRunConfiguration(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := decoded.qualityRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(root, filepath.Join(parent, "approved-workspace")); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	evidence := registry.Run(context.Background(), root)
	if len(evidence) != 1 || evidence[0].Verdict == quality.Pass {
		t.Fatalf("reconstructed check accepted the replacement workspace: %+v", evidence)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("quality command executed through the outside symlink: %v", err)
	}
}

func hasQualityCheck(registry *quality.Registry, key string) bool {
	for _, check := range registry.Checks() {
		if check.Key == key {
			return true
		}
	}
	return false
}

func TestDeferredQualityDiscoveryKeepsUnsupportedProjectsBlocked(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	project := t.TempDir()
	run, _, err := state.CreateRunWithConfiguration(ctx, "quality-unsupported-run", project, "implement project", []byte(`{"frozen":"original"}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := New(state).qualityRegistryForRun(ctx, run, deferredQualityConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	evidence := registry.Run(ctx, project)
	if registry.RequiredPassed(evidence) || len(evidence) != 1 || evidence[0].Verdict != quality.Blocked {
		t.Fatalf("unsupported project was treated as verified: %+v", evidence)
	}
	events, err := state.Events(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var checks []map[string]any
	for _, event := range events {
		if event.Type == "quality.checks.derived" {
			encoded, _ := json.Marshal(event.Payload["checks"])
			_ = json.Unmarshal(encoded, &checks)
		}
	}
	if len(checks) != 1 || checks[0]["unavailable_reason"] == "" {
		t.Fatalf("unsupported layout was not durably frozen as blocked: %+v", checks)
	}
}

func TestFrozenQualityManifestDriftBlocksBeforeRegistryUse(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "frozen-manifest.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	project := t.TempDir()
	if err = os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"test":"vitest run"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	run, _, err := state.CreateRunWithConfiguration(ctx, "frozen-manifest-run", project, "verify", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := qualityManifestHash(project)
	if err != nil {
		t.Fatal(err)
	}
	effective := buildEffectiveRunConfiguration(config.Default(), map[string]config.Decision{}, nil)
	effective.QualityManifestSHA256 = hash
	if _, err = New(state).qualityRegistryForRun(ctx, run, effective); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"test":"echo changed"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = New(state).qualityRegistryForRun(ctx, run, effective); err == nil || !strings.Contains(err.Error(), "QUALITY_CHECK_MANIFEST_DRIFT") {
		t.Fatalf("changed frozen manifest was accepted: %v", err)
	}
}
