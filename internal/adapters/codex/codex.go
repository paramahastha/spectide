package codex

import (
	"context"
	"fmt"
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
		a = append(a, "--config", fmt.Sprintf("model_reasoning_effort=%q", r.Effort))
	}
	a = append(a, r.Prompt)
	env := []string{}
	if r.Profile.ConfigDir != "" {
		env = append(env, "CODEX_HOME="+r.Profile.ConfigDir)
	}
	return runner.Invocation{Binary: r.Profile.Executable, Args: a, Dir: r.ProjectRoot, Env: env}, nil
}
func (Adapter) Normalize(raw []byte) ([]adapters.Event, error) {
	return adapters.NormalizeJSONL(adapters.Codex, raw)
}
