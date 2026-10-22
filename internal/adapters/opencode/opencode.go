package opencode

import (
	"context"
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
)

type Adapter struct{}

func (Adapter) Name() adapters.ProviderName { return adapters.OpenCode }
func (Adapter) Probe(ctx context.Context, p adapters.Profile) (adapters.Capability, error) {
	return adapters.ProbeBinary(p)
}
func (Adapter) Build(r adapters.Request) (runner.Invocation, error) {
	a := []string{"run", "--format", "json", "--dir", r.ProjectRoot}
	if r.Model != "" {
		a = append(a, "--model", r.Model)
	}
	if r.Effort != "" {
		a = append(a, "--variant", r.Effort)
	}
	a = append(a, r.Prompt)
	env := []string{}
	if r.Profile.ConfigDir != "" {
		env = append(env, "OPENCODE_CONFIG_DIR="+r.Profile.ConfigDir)
	}
	return runner.Invocation{Binary: r.Profile.Executable, Args: a, Env: env, Dir: r.ProjectRoot}, nil
}
func (Adapter) Normalize(raw []byte) ([]adapters.Event, error) {
	return adapters.NormalizeJSONL(adapters.OpenCode, raw)
}
