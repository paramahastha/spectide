package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
)

func TestEffectiveRunConfigurationFreezesBoundsChecksAndProfileReferences(t *testing.T) {
	cfg := config.Default()
	cfg.Execution = config.ExecutionSettings{StageTimeoutSeconds: map[string]int{"plan": 75}, AnalyzeRemediationLimit: 2}
	profile := model.Profile{
		Name: "work", Provider: "codex", Executable: "/opt/codex", ConfigDirectory: "/Users/example/.codex",
		AllowedScopes: []string{"/workspace", "/other"},
		Capability: &model.ProfileCapability{
			Provider: "codex", ExecutableFound: true, ApprovedExecutablePath: "/opt/codex", ExecutableSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ApprovedConfigDirectory: "/Users/example/.codex", Authenticated: "AUTHENTICATED", ReadOnlyExecution: true,
			WorkspaceWrite: true, ModelOverride: true, ModelCatalogStatus: "VERIFIED", ModelValues: []string{"model-a"}, SafetyModes: []string{"read-only", "workspace-write"}, Version: "codex 1.2", ObservedAt: time.Date(2026, 10, 22, 1, 2, 3, 0, time.UTC),
		},
	}
	decisions := make(map[string]config.Decision, len(stages))
	for _, stage := range stages {
		route := config.Route{Provider: "fake", Profile: "fake-demo"}
		if stage == "plan" {
			route = config.Route{Provider: "codex", Profile: "work", Model: "model-a"}
		}
		decisions[stage] = config.Decision{Stage: stage, Route: route, Mode: "hybrid", Explanation: "fixture"}
	}
	effective := buildEffectiveRunConfiguration(cfg, decisions, []model.Profile{profile})
	encoded, err := json.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := parseEffectiveRunConfiguration(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Execution.DefaultStageTimeoutSeconds != config.DefaultStageTimeoutSeconds || decoded.Execution.StageTimeoutSeconds["plan"] != 75 || decoded.Execution.StageTimeoutSeconds["analyze"] != config.DefaultStageTimeoutSeconds {
		t.Fatalf("effective timeout bounds=%+v", decoded.Execution)
	}
	if got, want := decoded.loopLimits(), (LoopLimits{AnalyzeRemediations: 2, TestRepairs: 4, ConvergenceRounds: 5}); got != want {
		t.Fatalf("effective loop limits=%+v want=%+v", got, want)
	}
	if len(decoded.RequiredChecks) != 4 || decoded.RequiredChecks[0].TimeoutSeconds < 1 {
		t.Fatalf("required check definitions were not frozen: %+v", decoded.RequiredChecks)
	}
	pinned, ok := decoded.profile("work")
	if !ok || !profileMatchesReference(profile, pinned) || pinned.ExecutableSHA256 != profile.Capability.ExecutableSHA256 {
		t.Fatalf("approved profile reference was not frozen: %+v", pinned)
	}
	drifted := profile
	drifted.Executable = "/opt/replaced-codex"
	if profileMatchesReference(drifted, pinned) {
		t.Fatal("profile executable drift matched immutable run configuration")
	}
	drifted = profile
	drifted.Capability = &model.ProfileCapability{}
	if profileMatchesReference(drifted, pinned) {
		t.Fatal("profile capability drift matched immutable run configuration")
	}
	drifted = profile
	drifted.Capability.ModelValues = []string{"changed-model"}
	if profileMatchesReference(drifted, pinned) {
		t.Fatal("verified model catalog drift matched immutable run configuration")
	}
}

func TestEffectiveRunConfigurationFreezesDiscoveredProjectQualityChecks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"test":"vitest run"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := quality.Discover(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	decisions := map[string]config.Decision{}
	for _, stage := range stages {
		route := config.Route{Provider: "fake", Profile: "demo"}
		decisions[stage] = config.Decision{Stage: stage, Route: route, Mode: "hybrid"}
	}
	effective := buildEffectiveRunConfiguration(cfg, decisions, nil, registry)
	encoded, err := json.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := parseEffectiveRunConfiguration(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	checks, err := restored.qualityRegistry()
	if err != nil {
		t.Fatal(err)
	}
	got := checks.Checks()
	if len(got) != 1 || got[0].Key != "node_tests" || got[0].UnavailableReason != "" || got[0].Binary != "npm" {
		t.Fatalf("project checks were not frozen/restored: %+v", got)
	}
}

func TestLegacyRunConfigurationUsesProductDefaults(t *testing.T) {
	effective, err := parseEffectiveRunConfiguration(`[]`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := effective.loopLimits(), DefaultLoopLimits(); got != want {
		t.Fatalf("legacy loop defaults=%+v want=%+v", got, want)
	}
	if effective.Execution.StageTimeoutSeconds["plan"] != config.DefaultStageTimeoutSeconds {
		t.Fatalf("legacy stage timeout=%+v", effective.Execution)
	}
}
