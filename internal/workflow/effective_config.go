package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
)

type effectiveRunConfiguration struct {
	SchemaVersion  int                        `json:"schema_version"`
	Stages         []effectiveStage           `json:"stages"`
	Execution      config.ExecutionSettings   `json:"execution"`
	Profiles       []approvedProfileReference `json:"profiles"`
	RequiredChecks []requiredCheckReference   `json:"required_checks"`
}

type effectiveStage struct {
	Stage     string          `json:"stage"`
	Decision  config.Decision `json:"decision"`
	Simulated bool            `json:"simulated"`
}

type approvedProfileReference struct {
	Name                    string    `json:"name"`
	Provider                string    `json:"provider"`
	Executable              string    `json:"executable"`
	ConfigDirectory         string    `json:"config_directory,omitempty"`
	AllowedScopes           []string  `json:"allowed_scopes"`
	ApprovedExecutablePath  string    `json:"approved_executable_path"`
	ExecutableSHA256        string    `json:"executable_sha256"`
	ApprovedConfigDirectory string    `json:"approved_config_directory"`
	Authenticated           string    `json:"authenticated"`
	ReadOnlyExecution       bool      `json:"read_only_execution"`
	WorkspaceWrite          bool      `json:"workspace_write"`
	ModelOverride           bool      `json:"model_override"`
	EffortOverride          string    `json:"effort_override,omitempty"`
	CapabilityVersion       string    `json:"capability_version,omitempty"`
	CapabilityObservedAt    time.Time `json:"capability_observed_at,omitempty"`
}

type requiredCheckReference struct {
	Key             string   `json:"key"`
	Binary          string   `json:"binary"`
	Args            []string `json:"args"`
	Required        bool     `json:"required"`
	RequireEmptyOut bool     `json:"require_empty_output"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
}

func buildEffectiveRunConfiguration(cfg config.Config, decisions map[string]config.Decision, profiles []model.Profile) effectiveRunConfiguration {
	effective := effectiveRunConfiguration{SchemaVersion: 1, Execution: cfg.EffectiveExecution()}
	for _, stage := range stages {
		decision := decisions[stage]
		effective.Stages = append(effective.Stages, effectiveStage{Stage: stage, Decision: decision, Simulated: decision.Route.Provider == "fake"})
	}
	usedProfiles := make(map[string]bool)
	for _, decision := range decisions {
		if decision.Route.Provider != "fake" {
			usedProfiles[decision.Route.Profile] = true
		}
		for _, fallback := range decision.Route.Fallback {
			usedProfiles[fallback.Profile] = true
		}
	}
	for _, profile := range profiles {
		if !usedProfiles[profile.Name] || profile.Capability == nil {
			continue
		}
		capability := profile.Capability
		reference := approvedProfileReference{
			Name: profile.Name, Provider: profile.Provider, Executable: profile.Executable, ConfigDirectory: profile.ConfigDirectory,
			AllowedScopes: append([]string(nil), profile.AllowedScopes...), ApprovedExecutablePath: capability.ApprovedExecutablePath,
			ExecutableSHA256: capability.ExecutableSHA256, ApprovedConfigDirectory: capability.ApprovedConfigDirectory,
			Authenticated: capability.Authenticated, ReadOnlyExecution: capability.ReadOnlyExecution,
			WorkspaceWrite: capability.WorkspaceWrite, ModelOverride: capability.ModelOverride,
			EffortOverride: capability.EffortOverride, CapabilityVersion: capability.Version,
			CapabilityObservedAt: capability.ObservedAt,
		}
		sort.Strings(reference.AllowedScopes)
		effective.Profiles = append(effective.Profiles, reference)
	}
	sort.Slice(effective.Profiles, func(i, j int) bool { return effective.Profiles[i].Name < effective.Profiles[j].Name })
	for _, check := range quality.DefaultRegistry(nil).Checks() {
		effective.RequiredChecks = append(effective.RequiredChecks, requiredCheckReference{
			Key: check.Key, Binary: check.Binary, Args: append([]string(nil), check.Args...), Required: check.Required,
			RequireEmptyOut: check.RequireEmptyOut, TimeoutSeconds: int(check.Timeout / time.Second),
		})
	}
	return effective
}

func parseEffectiveRunConfiguration(encoded string) (effectiveRunConfiguration, error) {
	var effective effectiveRunConfiguration
	if err := json.Unmarshal([]byte(encoded), &effective); err == nil && effective.SchemaVersion == 1 {
		if len(effective.Stages) != len(stages) {
			return effectiveRunConfiguration{}, fmt.Errorf("WORKSPACE_DRIFT: immutable run configuration is incomplete")
		}
		if err := validateEffectiveRunConfiguration(effective); err != nil {
			return effectiveRunConfiguration{}, err
		}
		return effective, nil
	}
	// Legacy array snapshots remain readable for existing simulated checkpoints.
	var legacy []effectiveStage
	if err := json.Unmarshal([]byte(encoded), &legacy); err != nil || (len(legacy) != 0 && len(legacy) != len(stages)) {
		return effectiveRunConfiguration{}, fmt.Errorf("WORKSPACE_DRIFT: immutable run configuration is invalid")
	}
	effective = effectiveRunConfiguration{Stages: legacy}
	effective.Execution = config.Default().EffectiveExecution()
	for _, check := range quality.DefaultRegistry(nil).Checks() {
		effective.RequiredChecks = append(effective.RequiredChecks, requiredCheckReference{
			Key: check.Key, Binary: check.Binary, Args: append([]string(nil), check.Args...), Required: check.Required,
			RequireEmptyOut: check.RequireEmptyOut, TimeoutSeconds: int(check.Timeout / time.Second),
		})
	}
	return effective, nil
}

func validateEffectiveRunConfiguration(effective effectiveRunConfiguration) error {
	if effective.Execution.DefaultStageTimeoutSeconds < config.MinStageTimeoutSeconds || effective.Execution.DefaultStageTimeoutSeconds > config.MaxStageTimeoutSeconds {
		return fmt.Errorf("WORKSPACE_DRIFT: immutable stage timeout is invalid")
	}
	limits := effective.Execution
	if limits.AnalyzeRemediationLimit < 1 || limits.AnalyzeRemediationLimit > 3 || limits.TestRepairLimit < 1 || limits.TestRepairLimit > 4 || limits.ConvergenceRoundLimit < 1 || limits.ConvergenceRoundLimit > 5 {
		return fmt.Errorf("WORKSPACE_DRIFT: immutable loop bounds are invalid")
	}
	for _, stage := range stages {
		seconds := effective.Execution.StageTimeoutSeconds[stage]
		if seconds < config.MinStageTimeoutSeconds || seconds > config.MaxStageTimeoutSeconds {
			return fmt.Errorf("WORKSPACE_DRIFT: immutable timeout for %s is invalid", stage)
		}
	}
	for index, stage := range effective.Stages {
		if stage.Stage != stages[index] || stage.Decision.Stage != stage.Stage || stage.Decision.Route.Provider == "" || stage.Simulated != (stage.Decision.Route.Provider == "fake") {
			return fmt.Errorf("WORKSPACE_DRIFT: immutable routing decision for %s is invalid", stages[index])
		}
		if stage.Decision.Route.Provider != "fake" {
			profile, ok := effective.profile(stage.Decision.Route.Profile)
			if !ok || profile.Provider != stage.Decision.Route.Provider {
				return fmt.Errorf("WORKSPACE_DRIFT: immutable profile reference for %s is missing", stage.Stage)
			}
		}
	}
	return nil
}

func (effective effectiveRunConfiguration) decisions() map[string]config.Decision {
	decisions := make(map[string]config.Decision, len(effective.Stages))
	for _, stage := range effective.Stages {
		decisions[stage.Stage] = stage.Decision
	}
	return decisions
}

func (effective effectiveRunConfiguration) loopLimits() LoopLimits {
	return LoopLimits{AnalyzeRemediations: effective.Execution.AnalyzeRemediationLimit, TestRepairs: effective.Execution.TestRepairLimit, ConvergenceRounds: effective.Execution.ConvergenceRoundLimit}
}

func (effective effectiveRunConfiguration) profile(name string) (approvedProfileReference, bool) {
	for _, profile := range effective.Profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return approvedProfileReference{}, false
}

func profileMatchesReference(profile model.Profile, expected approvedProfileReference) bool {
	if profile.Name != expected.Name || profile.Provider != expected.Provider || profile.Executable != expected.Executable || profile.ConfigDirectory != expected.ConfigDirectory || profile.Capability == nil {
		return false
	}
	scopes := append([]string(nil), profile.AllowedScopes...)
	sort.Strings(scopes)
	if len(scopes) != len(expected.AllowedScopes) {
		return false
	}
	for i := range scopes {
		if scopes[i] != expected.AllowedScopes[i] {
			return false
		}
	}
	capability := profile.Capability
	return capability.ApprovedExecutablePath == expected.ApprovedExecutablePath && capability.ExecutableSHA256 == expected.ExecutableSHA256 &&
		capability.ApprovedConfigDirectory == expected.ApprovedConfigDirectory && capability.Authenticated == expected.Authenticated &&
		capability.ReadOnlyExecution == expected.ReadOnlyExecution && capability.WorkspaceWrite == expected.WorkspaceWrite &&
		capability.ModelOverride == expected.ModelOverride && capability.EffortOverride == expected.EffortOverride &&
		capability.Version == expected.CapabilityVersion && capability.ObservedAt.Equal(expected.CapabilityObservedAt)
}

func (effective effectiveRunConfiguration) qualityRegistry() (*quality.Registry, error) {
	registry := quality.NewRegistry(nil)
	for _, check := range effective.RequiredChecks {
		if check.TimeoutSeconds < 1 || check.TimeoutSeconds > config.MaxStageTimeoutSeconds {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: immutable check timeout is invalid")
		}
		if err := registry.Register(quality.Check{Key: check.Key, Binary: check.Binary, Args: append([]string(nil), check.Args...), Required: check.Required, RequireEmptyOut: check.RequireEmptyOut, Timeout: time.Duration(check.TimeoutSeconds) * time.Second}); err != nil {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: immutable quality check is invalid")
		}
	}
	if len(effective.RequiredChecks) == 0 {
		return nil, fmt.Errorf("WORKSPACE_DRIFT: immutable quality checks are missing")
	}
	return registry, nil
}
