package routing

import (
	"github.com/spectide/spectide/internal/adapters"
	"testing"
)

func policy(mode Mode) Policy {
	return Policy{Mode: mode, ProjectScope: "personal", Default: Route{Profile: "safe", Model: "x", Effort: "medium"}, Profiles: map[string]adapters.Profile{"safe": {Name: "safe", Provider: adapters.Codex, AllowedProjects: []string{"personal"}}, "work": {Name: "work", Provider: adapters.Claude, AllowedProjects: []string{"work"}}}, Capabilities: map[string]adapters.Capability{"safe": {ExecutableFound: true, Version: "1.0", ModelOverride: true, EffortOverride: "verified", ReadOnlyExecution: true, WorkspaceWrite: true}, "work": {ExecutableFound: true, Version: "1.0", ModelOverride: true, EffortOverride: "verified", ReadOnlyExecution: true, WorkspaceWrite: true}}}
}
func TestManualRequiresExplicitStage(t *testing.T) {
	_, e := policy(Manual).Resolve("plan", "")
	if e == nil {
		t.Fatal("expected missing route")
	}
}
func TestRejectsForbiddenFallback(t *testing.T) {
	p := policy(Hybrid)
	p.Default.Fallback = []Route{{Profile: "work"}}
	if _, e := p.Resolve("plan", ""); e == nil {
		t.Fatal("expected unauthorized fallback rejection")
	}
	p.FallbackAuthorized = true
	if _, e := p.Resolve("plan", ""); e == nil {
		t.Fatal("expected cross-scope fallback rejection")
	}
}
func TestAutomaticExplainsRisk(t *testing.T) {
	d, e := policy(Automatic).Resolve("implement", "fix payment race")
	if e != nil {
		t.Fatal(e)
	}
	if d.Explanation == "" {
		t.Fatal("missing explanation")
	}
}

func TestRejectsUnverifiedCapabilitiesAndProviderBoundaries(t *testing.T) {
	p := policy(Hybrid)
	p.Capabilities["safe"] = adapters.Capability{ExecutableFound: true, Authenticated: "UNAUTHENTICATED", Version: "1.0", ModelOverride: true, EffortOverride: "verified", ReadOnlyExecution: true, WorkspaceWrite: true}
	if _, err := p.Resolve("plan", ""); err == nil {
		t.Fatal("unauthenticated profile accepted")
	}
	p.Capabilities["safe"] = adapters.Capability{ExecutableFound: true, Authenticated: "AUTHENTICATED", Version: "1.0", ModelOverride: true, EffortOverride: "verified", WorkspaceWrite: true}
	if _, err := p.Resolve("analyze", ""); err == nil {
		t.Fatal("unverified read-only stage accepted")
	}
	if _, err := p.Resolve("unknown", ""); err == nil {
		t.Fatal("unknown stage accepted")
	}
}
