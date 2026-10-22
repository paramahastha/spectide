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
