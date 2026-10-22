package routing

import (
	"github.com/spectide/spectide/internal/adapters"
	"testing"
)

func policy(mode Mode) Policy {
	return Policy{Mode: mode, ProjectScope: "personal", Default: Route{Profile: "safe", Model: "x", Effort: "medium"}, Profiles: map[string]adapters.Profile{"safe": {Name: "safe", AllowedProjects: []string{"personal"}}, "work": {Name: "work", AllowedProjects: []string{"work"}}}}
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
	_, e := p.Resolve("plan", "")
	if e == nil {
		t.Fatal("expected denied fallback")
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
