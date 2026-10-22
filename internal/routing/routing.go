// Package routing resolves stage routes deterministically and rejects unsafe fallbacks.
package routing

import (
	"fmt"
	"github.com/spectide/spectide/internal/adapters"
	"strings"
)

type Mode string

const (
	Manual    Mode = "manual"
	Hybrid    Mode = "hybrid"
	Automatic Mode = "automatic"
)

type Route struct {
	Profile, Model, Effort string
	Fallback               []Route
}
type Decision struct {
	Stage       string
	Route       Route
	Mode        Mode
	Explanation string
}
type Policy struct {
	Mode         Mode
	Default      Route
	Stages       map[string]Route
	Profiles     map[string]adapters.Profile
	ProjectScope string
}

func (p Policy) Resolve(stage, prompt string) (Decision, error) {
	r, explicit := p.Stages[stage]
	if p.Mode == Manual && !explicit {
		return Decision{}, fmt.Errorf("ROUTE_REQUIRED: manual route missing for %s", stage)
	}
	if !explicit {
		r = p.Default
	}
	if r.Profile == "" {
		return Decision{}, fmt.Errorf("PROFILE_REQUIRED: %s", stage)
	}
	if e := p.allowed(r); e != nil {
		return Decision{}, e
	}
	why := "configured route"
	if !explicit {
		why = "default route"
	}
	if p.Mode == Automatic {
		if risky(prompt) {
			why = "automatic high-risk route; default route retained"
		} else {
			why = "automatic deterministic default route"
		}
	}
	for _, f := range r.Fallback {
		if e := p.allowed(f); e != nil {
			return Decision{}, fmt.Errorf("unsafe fallback: %w", e)
		}
	}
	return Decision{Stage: stage, Route: r, Mode: p.Mode, Explanation: why}, nil
}
func (p Policy) allowed(r Route) error {
	profile := p.Profiles[r.Profile]
	if profile.Name == "" {
		return fmt.Errorf("PROFILE_NOT_FOUND: %s", r.Profile)
	}
	for _, scope := range profile.AllowedProjects {
		if scope == p.ProjectScope {
			return nil
		}
	}
	return fmt.Errorf("PROFILE_NOT_ALLOWED: %s for %s", r.Profile, p.ProjectScope)
}
func risky(prompt string) bool {
	p := strings.ToLower(prompt)
	for _, w := range []string{"payment", "refund", "auth", "concurrency", "migration", "security"} {
		if strings.Contains(p, w) {
			return true
		}
	}
	return false
}
