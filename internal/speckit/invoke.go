package speckit

import (
	"context"
	"fmt"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
)

type StageResult struct {
	Process runner.Result
	Events  []adapters.Event
}

// ExecuteStage expands a verified installed Spec-Kit command as a provider
// prompt, builds the provider's explicit invocation, and normalizes its output.
// Callers must independently validate profile authorization and capabilities.
func ExecuteStage(ctx context.Context, installation Installation, adapter adapters.Adapter, request adapters.Request, stage, arguments string) (StageResult, error) {
	if !installation.Verified {
		return StageResult{}, fmt.Errorf("SPEC_KIT_INTEGRATION_UNVERIFIED")
	}
	if adapter == nil {
		return StageResult{}, fmt.Errorf("PROVIDER_ADAPTER_REQUIRED")
	}
	if request.Profile.Provider != adapter.Name() {
		return StageResult{}, fmt.Errorf("PROFILE_PROVIDER_MISMATCH: adapter %s profile %s", adapter.Name(), request.Profile.Provider)
	}
	prompt, err := StagePrompt(installation, stage, arguments)
	if err != nil {
		return StageResult{}, err
	}
	request.Stage = stage
	request.ProjectRoot = installation.ProjectRoot
	request.Prompt = prompt
	request.ReadOnly = stage == "analyze"
	invocation, err := adapter.Build(request)
	if err != nil {
		return StageResult{}, err
	}
	if invocation.Dir != installation.ProjectRoot {
		return StageResult{}, fmt.Errorf("PERMISSION_DENIED: provider invocation working directory differs from canonical Spec-Kit root")
	}
	process, runErr := runner.Run(ctx, invocation)
	result := StageResult{Process: process}
	if runErr != nil {
		return result, runErr
	}
	result.Events, err = adapter.Normalize([]byte(process.Stdout))
	if err != nil {
		return result, err
	}
	return result, nil
}
