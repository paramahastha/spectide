package fake

import (
	"context"
	"encoding/json"
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
	"github.com/spectide/spectide/internal/security"
	"strings"
	"time"
)

type ScriptStep struct {
	Delay          time.Duration
	Type           string
	Summary        string
	FailureCode    string
	FailureMessage string
	RetryAfter     time.Duration
}

type Adapter struct{ Script []ScriptStep }

func (Adapter) Name() adapters.ProviderName { return adapters.Fake }
func (Adapter) Probe(ctx context.Context, p adapters.Profile) (adapters.Capability, error) {
	return adapters.Capability{Provider: adapters.Fake, ExecutableFound: true, Version: "deterministic", Authenticated: "not-required", StreamingEvents: true, ModelOverride: true, EffortOverride: "supported", Warnings: []string{"SIMULATED: not a live provider"}}, nil
}
func (Adapter) Build(r adapters.Request) (runner.Invocation, error) { return runner.Invocation{}, nil }
func (Adapter) Normalize(raw []byte) ([]adapters.Event, error) {
	var v map[string]any
	if e := json.Unmarshal(raw, &v); e != nil {
		return nil, e
	}
	return []adapters.Event{{Type: "fake.event", Summary: "SIMULATED event", Raw: raw}}, nil
}

// Execute replays a deterministic script. A retry delay is returned as typed
// metadata; the fake itself never sleeps for the provider's retry interval.
func (a Adapter) Execute(ctx context.Context) ([]adapters.Event, error) {
	events := make([]adapters.Event, 0, len(a.Script))
	for _, step := range a.Script {
		if step.Delay > 0 {
			timer := time.NewTimer(step.Delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return events, adapters.WrapProviderError("RUN_CANCELLED", "simulated provider execution was cancelled", ctx.Err())
			case <-timer.C:
			}
		}
		if step.RetryAfter > 0 {
			return events, adapters.NewRateLimitError(step.RetryAfter)
		}
		if step.FailureCode != "" {
			message := security.Redact(step.FailureMessage)
			return events, adapters.NewProviderError(step.FailureCode, message)
		}
		typ := step.Type
		if !safeEventType(typ) {
			typ = "event"
		}
		summary := security.Redact(step.Summary)
		if summary == "" {
			summary = "SIMULATED event"
		}
		events = append(events, adapters.Event{Type: "fake." + typ, Summary: "SIMULATED: " + summary})
	}
	return events, nil
}

func safeEventType(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("_.-", r) {
			return false
		}
	}
	return true
}
