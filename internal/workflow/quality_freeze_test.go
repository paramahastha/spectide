package workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
	run, _, err := state.CreateRunWithConfiguration(ctx, "quality-freeze-run", project, "implement project", []byte(`{"frozen":"original"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/generated\ngo 1.27.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	engine := New(state)
	registry, err := engine.qualityRegistryForRun(ctx, run, deferredQualityConfiguration())
	if err != nil || len(registry.Checks()) != 4 || !hasQualityCheck(registry, "gofmt") || !hasQualityCheck(registry, "go_test") {
		t.Fatalf("derived Go checks=%+v err=%v", registry, err)
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
	registry, err = engine.qualityRegistryForRun(ctx, run, deferredQualityConfiguration())
	if err != nil || len(registry.Checks()) != 4 {
		t.Fatalf("restored frozen checks=%+v err=%v", registry, err)
	}
	if err = os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/changed\ngo 1.27.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.qualityRegistryForRun(ctx, run, deferredQualityConfiguration()); err == nil {
		t.Fatal("changed project manifest reused frozen derived checks")
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
