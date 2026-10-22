package routing

import (
	"fmt"
	"strings"

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
		if err = validateFallbackProfiles(project, stage, decision.Route, byName); err != nil {
			return err
		}
		if decision.Route.Provider == "fake" {
			continue
		}
		profile, ok := byName[decision.Route.Profile]
		if !ok {
			return fmt.Errorf("PROFILE_NOT_FOUND: %s for %s", decision.Route.Profile, stage)
		}
		if err = validateProfileRoute(project, stage, decision.Route, profile); err != nil {
			return err
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
		for _, fallback := range decision.Route.Fallback {
			fallbackProfile, exists := byName[fallback.Profile]
			if !exists {
				return fmt.Errorf("FALLBACK_PROFILE_NOT_FOUND: %s for %s", fallback.Profile, stage)
			}
			if err = validateCapability(stage, fallback, fallbackProfile); err != nil {
				return fmt.Errorf("UNSAFE_FALLBACK: %w", err)
			}
		}
		if decision.Route.Provider == "fake" {
			continue
		}
		profile, ok := byName[decision.Route.Profile]
		if !ok {
			return fmt.Errorf("PROFILE_NOT_FOUND: %s for %s", decision.Route.Profile, stage)
		}
		if err = validateCapability(stage, decision.Route, profile); err != nil {
			return err
		}
	}
	return nil
}

func validateProfileRoute(project, stage string, route config.Route, profile model.Profile) error {
	if profile.Provider != route.Provider {
		return fmt.Errorf("PROFILE_PROVIDER_MISMATCH: %s for %s", route.Profile, stage)
	}
	if _, err := workspace.Canonical(project, profile.AllowedScopes); err != nil {
		return fmt.Errorf("PROFILE_NOT_ALLOWED: %s for %s: %w", route.Profile, stage, err)
	}
	return nil
}

func validateFallbackProfiles(project, stage string, route config.Route, profiles map[string]model.Profile) error {
	for _, fallback := range route.Fallback {
		fallbackProfile, ok := profiles[fallback.Profile]
		if !ok {
			return fmt.Errorf("FALLBACK_PROFILE_NOT_FOUND: %s for %s", fallback.Profile, stage)
		}
		if fallbackProfile.Provider != fallback.Provider {
			return fmt.Errorf("FALLBACK_PROVIDER_MISMATCH: %s for %s", fallback.Profile, stage)
		}
		if _, err := workspace.Canonical(project, fallbackProfile.AllowedScopes); err != nil {
			return fmt.Errorf("FALLBACK_PROFILE_NOT_ALLOWED: %s for %s: %w", fallback.Profile, stage, err)
		}
	}
	return nil
}

// ValidateRouteCapability checks a specific persisted profile snapshot against
// the policy required for one stage invocation.
func ValidateRouteCapability(stage string, route config.Route, profile model.Profile) error {
	capability := profile.Capability
	if capability == nil {
		return fmt.Errorf("PROFILE_CAPABILITY_NOT_PROBED: %s", profile.Name)
	}
	if capability.Provider != route.Provider || capability.Provider != profile.Provider {
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
	if route.Model != "" {
		if !capability.ModelOverride {
			return fmt.Errorf("MODEL_OVERRIDE_UNSUPPORTED: %s", profile.Name)
		}
		if capability.ModelCatalogStatus != "VERIFIED" || !containsCapability(capability.ModelValues, route.Model) {
			return fmt.Errorf("MODEL_NOT_VERIFIED: %s", profile.Name)
		}
		if capability.Provider == "opencode" {
			provider, _, ok := strings.Cut(route.Model, "/")
			if !ok || !containsCapability(capability.AuthenticatedProviders, provider) {
				return fmt.Errorf("MODEL_PROVIDER_UNAUTHENTICATED: %s", profile.Name)
			}
		}
	}
	if route.Effort != "" {
		if capability.EffortOverride != "verified" {
			return fmt.Errorf("EFFORT_OVERRIDE_UNSUPPORTED: %s", profile.Name)
		}
		if !containsCapability(capability.EffortValues, route.Effort) {
			return fmt.Errorf("EFFORT_NOT_VERIFIED: %s", profile.Name)
		}
	}
	if (stage == "analyze" || stage == "final_review") && (!capability.ReadOnlyExecution || !containsCapability(capability.SafetyModes, "read-only")) {
		return fmt.Errorf("READ_ONLY_EXECUTION_UNVERIFIED: %s", profile.Name)
	}
	if stage == "implement" && (!capability.WorkspaceWrite || !containsCapability(capability.SafetyModes, "workspace-write")) {
		return fmt.Errorf("WORKSPACE_WRITE_UNVERIFIED: %s", profile.Name)
	}
	return nil
}

func containsCapability(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validateCapability(stage string, route config.Route, profile model.Profile) error {
	return ValidateRouteCapability(stage, route, profile)
}
