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
	Mode               Mode
	Default            Route
	Stages             map[string]Route
	Profiles           map[string]adapters.Profile
	Capabilities       map[string]adapters.Capability
	ProjectScope       string
	FallbackAuthorized bool
}

func (p Policy) Resolve(stage, prompt string) (Decision, error) {
	if !knownStage(stage) {
		return Decision{}, fmt.Errorf("UNKNOWN_STAGE: %s", stage)
	}
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
	if e := p.allowed(stage, r); e != nil {
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
		if !p.FallbackAuthorized {
			return Decision{}, fmt.Errorf("FALLBACK_NOT_AUTHORIZED")
		}
		if e := p.allowed(stage, f); e != nil {
			return Decision{}, fmt.Errorf("unsafe fallback: %w", e)
		}
	}
	return Decision{Stage: stage, Route: r, Mode: p.Mode, Explanation: why}, nil
}
func (p Policy) allowed(stage string, r Route) error {
	profile := p.Profiles[r.Profile]
	if profile.Name == "" {
		return fmt.Errorf("PROFILE_NOT_FOUND: %s", r.Profile)
	}
	if profile.Provider == "" {
		return fmt.Errorf("PROFILE_PROVIDER_REQUIRED: %s", r.Profile)
	}
	for _, scope := range profile.AllowedProjects {
		if scope == p.ProjectScope {
			capability, ok := p.Capabilities[r.Profile]
			if !ok || !capability.ExecutableFound || capability.Version == "" {
				return fmt.Errorf("CAPABILITY_NOT_VERIFIED: %s", r.Profile)
			}
			if capability.Authenticated != "AUTHENTICATED" {
				if capability.Authenticated == "UNAUTHENTICATED" {
					return fmt.Errorf("PROFILE_UNAUTHENTICATED: %s", r.Profile)
				}
				return fmt.Errorf("PROFILE_AUTHENTICATION_NOT_VERIFIED: %s", r.Profile)
			}
			if r.Model != "" && !capability.ModelOverride {
				return fmt.Errorf("MODEL_OVERRIDE_UNSUPPORTED: %s", r.Profile)
			}
			if r.Effort != "" && capability.EffortOverride != "verified" {
				return fmt.Errorf("EFFORT_OVERRIDE_UNSUPPORTED: %s", r.Profile)
			}
			if stage == "analyze" && !capability.ReadOnlyExecution {
				return fmt.Errorf("READ_ONLY_EXECUTION_UNVERIFIED: %s", r.Profile)
			}
			if stage == "implement" && !capability.WorkspaceWrite {
				return fmt.Errorf("WORKSPACE_WRITE_UNVERIFIED: %s", r.Profile)
			}
			return nil
		}
	}
	return fmt.Errorf("PROFILE_NOT_ALLOWED: %s for %s", r.Profile, p.ProjectScope)
}

func knownStage(stage string) bool {
	switch stage {
	case "constitution", "specify", "clarify", "plan", "checklist", "tasks", "analyze", "implement", "tests", "converge", "final_review", "report":
		return true
	default:
		return false
	}
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
