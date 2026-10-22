package claude

import (
	"context"
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
)

type Adapter struct{}

func (Adapter) Name() adapters.ProviderName { return adapters.Claude }
func (Adapter) Probe(ctx context.Context, p adapters.Profile) (adapters.Capability, error) {
	return adapters.ProbeProvider(ctx, p)
}
func (Adapter) Build(r adapters.Request) (runner.Invocation, error) {
	a := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if r.ReadOnly {
		a = append(a, "--permission-mode", "plan")
	}
	if r.Model != "" {
		a = append(a, "--model", r.Model)
	}
	if r.Effort != "" {
		a = append(a, "--effort", r.Effort)
	}
	a = append(a, r.Prompt)
	env := []string{}
	if r.Profile.ConfigDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+r.Profile.ConfigDir)
	}
	return runner.Invocation{Binary: r.Profile.Executable, Args: a, Env: env, Dir: r.ProjectRoot, Timeout: r.Timeout}, nil
}
func (Adapter) Normalize(raw []byte) ([]adapters.Event, error) {
	return adapters.NormalizeJSONL(adapters.Claude, raw)
}
