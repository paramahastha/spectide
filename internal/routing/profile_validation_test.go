package routing

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
)

func TestValidateProfileRoutesChecksExistenceProviderAndScope(t *testing.T) {
	project := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "local-codex"}
	profile := model.Profile{Name: "local-codex", Provider: "codex", AllowedScopes: []string{project}}
	if err := ValidateProfileRoutes(cfg, project, []model.Profile{profile}); err != nil {
		t.Fatalf("authorized profile rejected: %v", err)
	}

	if err := ValidateProfileRoutes(cfg, project, nil); err == nil || !strings.Contains(err.Error(), "PROFILE_NOT_FOUND") {
		t.Fatalf("missing profile error=%v", err)
	}
	wrongProvider := profile
	wrongProvider.Provider = "claude"
	if err := ValidateProfileRoutes(cfg, project, []model.Profile{wrongProvider}); err == nil || !strings.Contains(err.Error(), "PROFILE_PROVIDER_MISMATCH") {
		t.Fatalf("provider mismatch error=%v", err)
	}
	outside := t.TempDir()
	profile.AllowedScopes = []string{filepath.Clean(outside)}
	if err := ValidateProfileRoutes(cfg, project, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "PROFILE_NOT_ALLOWED") {
		t.Fatalf("outside-scope profile error=%v", err)
	}
}

func TestValidateProfileCapabilitiesRequiresVerifiedAuthorization(t *testing.T) {
	project := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "local-codex"}
	profile := model.Profile{Name: "local-codex", Provider: "codex", AllowedScopes: []string{project}}
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "PROFILE_CAPABILITY_NOT_PROBED") {
		t.Fatalf("missing capability error=%v", err)
	}
	profile.Capability = &model.ProfileCapability{Provider: "codex", ExecutableFound: true, Version: "1.2.3", Authenticated: "NOT_PROBED", ReadOnlyExecution: true, WorkspaceWrite: true, SafetyModes: []string{"read-only", "workspace-write"}, ModelOverride: true, ModelCatalogStatus: "VERIFIED", ModelValues: []string{"x"}}
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "PROFILE_AUTHENTICATION_NOT_VERIFIED") {
		t.Fatalf("unprobed authentication error=%v", err)
	}
	profile.Capability.Authenticated = "AUTHENTICATED"
	profile.Capability.EffortValues = []string{"high"}
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err != nil {
		t.Fatalf("complete capability rejected: %v", err)
	}
	cfg.StageRoutes = map[string]config.Route{"plan": {Provider: "codex", Profile: "local-codex", Model: "unknown-model"}}
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "MODEL_NOT_VERIFIED") {
		t.Fatalf("unknown model error=%v", err)
	}
	cfg.StageRoutes = map[string]config.Route{"plan": {Provider: "codex", Profile: "local-codex", Model: "x"}}
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err != nil {
		t.Fatalf("verified model rejected: %v", err)
	}

	cfg.StageRoutes = map[string]config.Route{"analyze": {Provider: "codex", Profile: "local-codex", Effort: "high"}}
	profile.Capability.EffortOverride = "unsupported"
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "EFFORT_OVERRIDE_UNSUPPORTED") {
		t.Fatalf("unsupported effort error=%v", err)
	}
	profile.Capability.EffortOverride = "verified"
	profile.Capability.EffortValues = nil
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "EFFORT_NOT_VERIFIED") {
		t.Fatalf("unverified exact effort error=%v", err)
	}
	profile.Capability.EffortValues = []string{"high"}
	profile.Capability.ReadOnlyExecution = false
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "READ_ONLY_EXECUTION_UNVERIFIED") {
		t.Fatalf("unverified read-only error=%v", err)
	}
	cfg.StageRoutes = map[string]config.Route{"final_review": {Provider: "codex", Profile: "local-codex"}}
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "READ_ONLY_EXECUTION_UNVERIFIED") {
		t.Fatalf("unverified final-review read-only error=%v", err)
	}
}

func TestFallbackProfilesAreAuthorizedAndRemainWithinProjectBoundary(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute = config.Route{
		Provider: "codex", Profile: "primary",
		FallbackAuthorized: true,
		Fallback:           []config.Route{{Provider: "claude", Profile: "backup"}},
	}
	capability := func(provider string) *model.ProfileCapability {
		return &model.ProfileCapability{Provider: provider, ExecutableFound: true, Version: "1.0", Authenticated: "AUTHENTICATED", ReadOnlyExecution: true, WorkspaceWrite: true, SafetyModes: []string{"read-only", "workspace-write"}}
	}
	profiles := []model.Profile{
		{Name: "primary", Provider: "codex", AllowedScopes: []string{project}, Capability: capability("codex")},
		{Name: "backup", Provider: "claude", AllowedScopes: []string{project}, Capability: capability("claude")},
	}
	if err := ValidateProfileRoutes(cfg, project, profiles); err != nil {
		t.Fatalf("authorized same-project fallback rejected: %v", err)
	}
	if err := ValidateProfileCapabilities(cfg, profiles); err != nil {
		t.Fatalf("verified fallback capability rejected: %v", err)
	}
	profiles[1].AllowedScopes = []string{outside}
	if err := ValidateProfileRoutes(cfg, project, profiles); err == nil || !strings.Contains(err.Error(), "FALLBACK_PROFILE_NOT_ALLOWED") {
		t.Fatalf("cross-project fallback error=%v", err)
	}
	profiles[1].AllowedScopes = []string{project}
	profiles[1].Provider = "opencode"
	if err := ValidateProfileRoutes(cfg, project, profiles); err == nil || !strings.Contains(err.Error(), "FALLBACK_PROVIDER_MISMATCH") {
		t.Fatalf("fallback provider mismatch error=%v", err)
	}
}
