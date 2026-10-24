package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectConfigSaveLoadIsPrivateAndValidated(t *testing.T) {
	path := ProjectPath(t.TempDir())
	c := Default()
	c.Project = "example"
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "example" {
		t.Fatalf("config=%+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	bad := c
	bad.RoutingMode = "automatic-ish"
	if Validate(bad) == nil {
		t.Fatal("invalid routing mode accepted")
	}
}

func TestSaveDoesNotFollowPredictableTemporaryFileSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("outside-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	if err := os.Symlink(outside, path+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(outside)
	if err != nil || string(contents) != "outside-original" {
		t.Fatalf("external temp target changed: %q err=%v", contents, err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("config was not atomically saved: %v", err)
	}
}

func TestConfigPersistenceRedactsDisplayNameAndRejectsSecretIdentities(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	displaySecret := "config-display-secret-123456"
	cfg := Default()
	cfg.Project = "workspace password=" + displaySecret
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), displaySecret) || !strings.Contains(string(contents), "[REDACTED]") {
		t.Fatalf("saved config leaked display metadata: %s", contents)
	}
	loaded, err := Load(path)
	if err != nil || strings.Contains(loaded.Project, displaySecret) || !strings.Contains(loaded.Project, "[REDACTED]") {
		t.Fatalf("loaded config=%+v err=%v", loaded, err)
	}
	if err := Save(filepath.Join(dir, "token="+displaySecret+".json"), Default()); err == nil || !strings.Contains(err.Error(), "CONFIG_PATH_SECRET_REFERENCE_REJECTED") {
		t.Fatalf("secret-bearing config path was accepted: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"profile identity": func(c *Config) { c.DefaultRoute.Profile = "work api_key=" + displaySecret },
		"model identity":   func(c *Config) { c.DefaultRoute.Model = "model token=" + displaySecret },
		"workspace scope":  func(c *Config) { c.AllowedScopes = []string{"/workspace/secret=" + displaySecret} },
	} {
		t.Run(name, func(t *testing.T) {
			bad := Default()
			mutate(&bad)
			if err := Save(filepath.Join(t.TempDir(), "config.json"), bad); err == nil || !strings.Contains(err.Error(), "CONFIG_") {
				t.Fatalf("secret-bearing configuration identity was accepted: %v", err)
			}
		})
	}
}

func TestMissingConfigGetsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || c.RoutingMode != "hybrid" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestManualHybridAndAutomaticRouting(t *testing.T) {
	c := Default()
	c.RoutingMode = "manual"
	c.DefaultRoute = Route{}
	if _, err := c.Resolve("plan", "task"); err == nil {
		t.Fatal("manual mode silently used a default")
	}
	c.StageRoutes = map[string]Route{"plan": {Provider: "codex", Profile: "work"}}
	d, err := c.Resolve("plan", "task")
	if err != nil || d.Route.Profile != "work" || d.Explanation != "explicit manual stage route" {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
	c = Default()
	c.StageRoutes = map[string]Route{"plan": {Provider: "codex", Profile: "work"}}
	d, err = c.Resolve("plan", "task")
	if err != nil || d.Route.Provider != "codex" {
		t.Fatalf("hybrid decision=%+v err=%v", d, err)
	}
	c.RoutingMode = "automatic"
	d, err = c.Resolve("analyze", "security migration")
	if err != nil || !strings.Contains(d.Explanation, "high-risk") {
		t.Fatalf("automatic decision=%+v err=%v", d, err)
	}
}

func TestLoadEffectiveAppliesGlobalThenProjectOverrides(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global", "config.json")
	projectPath := filepath.Join(dir, "project", "config.json")
	global := Default()
	global.DefaultRoute = Route{Provider: "codex", Profile: "global", Model: "gpt-default"}
	global.StageRoutes = map[string]Route{"plan": {Provider: "codex", Profile: "global", Model: "gpt-plan"}, "tests": {Provider: "codex", Profile: "global"}}
	global.AllowedScopes = []string{"/global-scope"}
	if err := Save(globalPath, global); err != nil {
		t.Fatal(err)
	}
	project := Default()
	project.DefaultRoute = Route{Provider: "fake", Profile: "project", Model: "local-default"}
	project.StageRoutes = map[string]Route{"plan": {Provider: "fake", Profile: "project", Model: "local-plan"}}
	project.AllowedScopes = []string{"/project-scope"}
	if err := Save(projectPath, project); err != nil {
		t.Fatal(err)
	}
	effective, err := LoadEffective(globalPath, projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if effective.DefaultRoute.Profile != "project" || effective.StageRoutes["plan"].Model != "local-plan" {
		t.Fatalf("project route did not override global: %+v", effective)
	}
	if effective.StageRoutes["tests"].Profile != "global" {
		t.Fatalf("unoverridden global stage route was lost: %+v", effective.StageRoutes)
	}
	if len(effective.AllowedScopes) != 1 || effective.AllowedScopes[0] != "/project-scope" {
		t.Fatalf("project scopes did not override global scopes: %+v", effective.AllowedScopes)
	}
}

func TestLoadEffectiveUsesDefaultsWhenLayersAreAbsent(t *testing.T) {
	cfg, err := LoadEffective(filepath.Join(t.TempDir(), "missing-global.json"), filepath.Join(t.TempDir(), "missing-project.json"))
	if err != nil || cfg.RoutingMode != "hybrid" || cfg.DefaultRoute.Provider != "fake" {
		t.Fatalf("effective config=%+v err=%v", cfg, err)
	}
}

func TestExecutionSettingsResolveDefaultsAndOverrides(t *testing.T) {
	cfg := Default()
	effective := cfg.EffectiveExecution()
	if effective.DefaultStageTimeoutSeconds != 1800 || effective.AnalyzeRemediationLimit != 3 || effective.TestRepairLimit != 4 || effective.ConvergenceRoundLimit != 5 {
		t.Fatalf("execution defaults=%+v", effective)
	}
	for _, stage := range StageKeys {
		if effective.StageTimeoutSeconds[stage] != 1800 {
			t.Fatalf("default timeout for %s=%d", stage, effective.StageTimeoutSeconds[stage])
		}
	}
	cfg.Execution = ExecutionSettings{DefaultStageTimeoutSeconds: 900, StageTimeoutSeconds: map[string]int{"analyze": 45}, AnalyzeRemediationLimit: 2, TestRepairLimit: 3, ConvergenceRoundLimit: 4}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	effective = cfg.EffectiveExecution()
	if effective.DefaultStageTimeoutSeconds != 900 || effective.StageTimeoutSeconds["analyze"] != 45 || effective.StageTimeoutSeconds["plan"] != 900 || effective.AnalyzeRemediationLimit != 2 || effective.TestRepairLimit != 3 || effective.ConvergenceRoundLimit != 4 {
		t.Fatalf("execution overrides=%+v", effective)
	}
}

func TestExecutionSettingsRejectTimeoutAndLoopLimitOutOfRange(t *testing.T) {
	cases := map[string]ExecutionSettings{
		"zero stage timeout":       {StageTimeoutSeconds: map[string]int{"plan": 0}},
		"timeout below one second": {DefaultStageTimeoutSeconds: -1},
		"timeout above four hours": {DefaultStageTimeoutSeconds: MaxStageTimeoutSeconds + 1},
		"unknown stage timeout":    {StageTimeoutSeconds: map[string]int{"not-a-stage": 60}},
		"analyze rounds":           {AnalyzeRemediationLimit: 4},
		"test repairs":             {TestRepairLimit: 5},
		"convergence rounds":       {ConvergenceRoundLimit: 6},
	}
	for name, execution := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			cfg.Execution = execution
			if err := Validate(cfg); err == nil {
				t.Fatal("out-of-range execution setting accepted")
			}
		})
	}
}

func TestExecutionTimeoutAcceptsDocumentedBoundaryValues(t *testing.T) {
	for _, seconds := range []int{MinStageTimeoutSeconds, MaxStageTimeoutSeconds} {
		t.Run(fmt.Sprintf("%d seconds", seconds), func(t *testing.T) {
			cfg := Default()
			cfg.Execution = ExecutionSettings{DefaultStageTimeoutSeconds: seconds}
			if err := Validate(cfg); err != nil {
				t.Fatalf("documented timeout boundary rejected: %v", err)
			}
		})
	}
}

func TestLoadEffectiveMergesExecutionOverrides(t *testing.T) {
	root := t.TempDir()
	globalPath, projectPath := filepath.Join(root, "global.json"), filepath.Join(root, "project.json")
	global := Default()
	global.Execution = ExecutionSettings{DefaultStageTimeoutSeconds: 600, StageTimeoutSeconds: map[string]int{"analyze": 120}, AnalyzeRemediationLimit: 2}
	project := Default()
	project.Execution = ExecutionSettings{StageTimeoutSeconds: map[string]int{"analyze": 90}, TestRepairLimit: 3}
	if err := Save(globalPath, global); err != nil {
		t.Fatal(err)
	}
	if err := Save(projectPath, project); err != nil {
		t.Fatal(err)
	}
	effective, err := LoadEffective(globalPath, projectPath)
	if err != nil {
		t.Fatal(err)
	}
	settings := effective.EffectiveExecution()
	if settings.DefaultStageTimeoutSeconds != 600 || settings.StageTimeoutSeconds["analyze"] != 90 || settings.StageTimeoutSeconds["plan"] != 600 || settings.AnalyzeRemediationLimit != 2 || settings.TestRepairLimit != 3 || settings.ConvergenceRoundLimit != 5 {
		t.Fatalf("merged execution settings=%+v", settings)
	}
}

func TestFallbackRoutesRequireExplicitAuthorizationAndValidBoundaries(t *testing.T) {
	cfg := Default()
	cfg.DefaultRoute.Fallback = []Route{{Provider: "codex", Profile: "backup"}}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "FALLBACK_NOT_AUTHORIZED") {
		t.Fatalf("unauthorized fallback validation error=%v", err)
	}
	cfg.DefaultRoute.FallbackAuthorized = true
	if err := Validate(cfg); err != nil {
		t.Fatalf("authorized fallback rejected: %v", err)
	}
	decision, err := cfg.Resolve("implement", "task")
	if err != nil || !decision.Route.FallbackAuthorized || len(decision.Route.Fallback) != 1 {
		t.Fatalf("fallback intent not retained in route decision: decision=%+v err=%v", decision, err)
	}
	cfg.DefaultRoute.Fallback[0].Fallback = []Route{{Provider: "fake", Profile: "nested"}}
	if err = Validate(cfg); err == nil || !strings.Contains(err.Error(), "NESTED_FALLBACK_UNSUPPORTED") {
		t.Fatalf("nested fallback validation error=%v", err)
	}
}
