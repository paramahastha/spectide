package routing

import (
	"fmt"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/workspace"
)

// ValidateProfileRoutes ensures every configured non-simulated route resolves
// to a matching stored profile authorized for the canonical project. It does
// not claim the profile is authenticated or capable of live stage execution.
func ValidateProfileRoutes(cfg config.Config, project string, profiles []model.Profile) error {
	byName := make(map[string]model.Profile, len(profiles))
	for _, profile := range profiles {
		byName[profile.Name] = profile
	}
	for _, stage := range config.StageKeys {
		decision, err := cfg.Resolve(stage, "")
		if err != nil {
			return err
		}
		if decision.Route.Provider == "fake" {
			continue
		}
		profile, ok := byName[decision.Route.Profile]
		if !ok {
			return fmt.Errorf("PROFILE_NOT_FOUND: %s for %s", decision.Route.Profile, stage)
		}
		if profile.Provider != decision.Route.Provider {
			return fmt.Errorf("PROFILE_PROVIDER_MISMATCH: %s for %s", decision.Route.Profile, stage)
		}
		if _, err = workspace.Canonical(project, profile.AllowedScopes); err != nil {
			return fmt.Errorf("PROFILE_NOT_ALLOWED: %s for %s: %w", decision.Route.Profile, stage, err)
		}
	}
	return nil
}

// ValidateProfileCapabilities denies live routes unless persisted probe
// evidence authorizes the requested model, effort, and execution mode.
func ValidateProfileCapabilities(cfg config.Config, profiles []model.Profile) error {
	byName := make(map[string]model.Profile, len(profiles))
	for _, profile := range profiles {
		byName[profile.Name] = profile
	}
	for _, stage := range config.StageKeys {
		decision, err := cfg.Resolve(stage, "")
		if err != nil {
			return err
		}
		if decision.Route.Provider == "fake" {
			continue
		}
		profile, ok := byName[decision.Route.Profile]
		if !ok {
			return fmt.Errorf("PROFILE_NOT_FOUND: %s for %s", decision.Route.Profile, stage)
		}
		capability := profile.Capability
		if capability == nil {
			return fmt.Errorf("PROFILE_CAPABILITY_NOT_PROBED: %s", profile.Name)
		}
		if capability.Provider != decision.Route.Provider || capability.Provider != profile.Provider {
			return fmt.Errorf("CAPABILITY_PROVIDER_MISMATCH: %s", profile.Name)
		}
		if !capability.ExecutableFound || capability.Version == "" {
			return fmt.Errorf("CAPABILITY_NOT_VERIFIED: %s", profile.Name)
		}
		if capability.Authenticated != "AUTHENTICATED" {
			if capability.Authenticated == "UNAUTHENTICATED" {
				return fmt.Errorf("PROFILE_UNAUTHENTICATED: %s", profile.Name)
			}
			return fmt.Errorf("PROFILE_AUTHENTICATION_NOT_VERIFIED: %s", profile.Name)
		}
		if decision.Route.Model != "" && !capability.ModelOverride {
			return fmt.Errorf("MODEL_OVERRIDE_UNSUPPORTED: %s", profile.Name)
		}
		if decision.Route.Effort != "" && capability.EffortOverride != "verified" {
			return fmt.Errorf("EFFORT_OVERRIDE_UNSUPPORTED: %s", profile.Name)
		}
		if stage == "analyze" && !capability.ReadOnlyExecution {
			return fmt.Errorf("READ_ONLY_EXECUTION_UNVERIFIED: %s", profile.Name)
		}
		if stage == "implement" && !capability.WorkspaceWrite {
			return fmt.Errorf("WORKSPACE_WRITE_UNVERIFIED: %s", profile.Name)
		}
	}
	return nil
}
