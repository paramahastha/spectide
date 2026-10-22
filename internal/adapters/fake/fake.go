package fake

import (
	"context"
	"encoding/json"
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
)

type Adapter struct{}

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
