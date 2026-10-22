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
	profile.Capability = &model.ProfileCapability{Provider: "codex", ExecutableFound: true, Version: "1.2.3", Authenticated: "NOT_PROBED", ReadOnlyExecution: true, WorkspaceWrite: true}
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "PROFILE_AUTHENTICATION_NOT_VERIFIED") {
		t.Fatalf("unprobed authentication error=%v", err)
	}
	profile.Capability.Authenticated = "AUTHENTICATED"
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err != nil {
		t.Fatalf("complete capability rejected: %v", err)
	}

	cfg.StageRoutes = map[string]config.Route{"analyze": {Provider: "codex", Profile: "local-codex", Effort: "high"}}
	profile.Capability.EffortOverride = "unsupported"
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "EFFORT_OVERRIDE_UNSUPPORTED") {
		t.Fatalf("unsupported effort error=%v", err)
	}
	profile.Capability.EffortOverride = "verified"
	profile.Capability.ReadOnlyExecution = false
	if err := ValidateProfileCapabilities(cfg, []model.Profile{profile}); err == nil || !strings.Contains(err.Error(), "READ_ONLY_EXECUTION_UNVERIFIED") {
		t.Fatalf("unverified read-only error=%v", err)
	}
}
