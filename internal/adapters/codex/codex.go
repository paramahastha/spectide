package codex

import (
	"context"
	"encoding/json"
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
)

type Adapter struct{}

func (Adapter) Name() adapters.ProviderName { return adapters.Codex }
func (Adapter) Probe(ctx context.Context, p adapters.Profile) (adapters.Capability, error) {
	return adapters.ProbeBinary(p)
}
func (Adapter) Build(r adapters.Request) (runner.Invocation, error) {
	a := []string{"exec", "--json", "--cd", r.ProjectRoot, "--sandbox"}
	if r.ReadOnly {
		a = append(a, "read-only")
	} else {
		a = append(a, "workspace-write")
	}
	if r.Model != "" {
		a = append(a, "--model", r.Model)
	}
	if r.Effort != "" {
		a = append(a, "--config", "model_reasoning_effort=\""+r.Effort+"\"")
	}
	a = append(a, r.Prompt)
	return runner.Invocation{Binary: r.Profile.Executable, Args: a, Dir: r.ProjectRoot}, nil
}
func (Adapter) Normalize(raw []byte) ([]adapters.Event, error) {
	var v map[string]any
	if e := json.Unmarshal(raw, &v); e != nil {
		return nil, e
	}
	return []adapters.Event{{Type: "codex.event", Summary: "Codex structured event", Raw: raw}}, nil
}
