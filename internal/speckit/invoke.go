package speckit

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
)

type StageResult struct {
	Process runner.Result
	Events  []adapters.Event
	Output  string
}

// FeatureScopeArguments binds every installed stage prompt to the canonical
// product specification directory selected by the workflow. Feature-owned
// artifact stages must not derive a second branch or feature directory.
func FeatureScopeArguments(projectRoot, featureDirectory, arguments string) (string, error) {
	if featureDirectory == "" {
		return arguments, nil
	}
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	feature, err := filepath.EvalSymlinks(featureDirectory)
	if err != nil {
		return "", err
	}
	feature, err = filepath.Abs(feature)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, feature)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("PERMISSION_DENIED: selected feature directory escapes project root")
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) != 2 || parts[0] != "specs" || parts[1] == "" || parts[1] == "." || parts[1] == ".." {
		return "", fmt.Errorf("SPEC_KIT_FEATURE_SCOPE_INVALID")
	}
	scope := filepath.ToSlash(filepath.Join(parts[0], parts[1]))
	directive := "Selected canonical feature directory: " + scope + ". Treat this as the only product specification scope for this run. For specify, create or update only " + scope + "/spec.md; do not create another feature directory, branch, or alternate specification. Keep all Spec-Kit feature artifacts and every retry/remediation in this selected directory."
	if strings.TrimSpace(arguments) == "" {
		return directive, nil
	}
	return strings.TrimSpace(arguments) + "\n\n" + directive, nil
}

// ExecuteStage expands a verified installed Spec-Kit command as a provider
// prompt, builds the provider's explicit invocation, and normalizes its output.
// Callers must independently validate profile authorization and capabilities.
func ExecuteStage(ctx context.Context, installation Installation, adapter adapters.Adapter, request adapters.Request, stage, arguments string) (StageResult, error) {
	return ExecuteStageWithTemplate(ctx, installation, adapter, request, stage, stage, arguments)
}

// ExecuteStageWithTemplate keeps the execution capability stage separate from
// the verified Spec-Kit prompt template. It is used for bounded remediation,
// where a write-capable implementation route must receive an owner-specific
// command template instead of general implementation instructions.
func ExecuteStageWithTemplate(ctx context.Context, installation Installation, adapter adapters.Adapter, request adapters.Request, stage, templateStage, arguments string) (StageResult, error) {
	if !installation.Verified {
		return StageResult{}, fmt.Errorf("SPEC_KIT_INTEGRATION_UNVERIFIED")
	}
	if adapter == nil {
		return StageResult{}, fmt.Errorf("PROVIDER_ADAPTER_REQUIRED")
	}
	if request.Profile.Provider != adapter.Name() {
		return StageResult{}, fmt.Errorf("PROFILE_PROVIDER_MISMATCH: adapter %s profile %s", adapter.Name(), request.Profile.Provider)
	}
	if templateStage == "" {
		return StageResult{}, fmt.Errorf("SPEC_KIT_STAGE_REQUIRED")
	}
	request.ProjectRoot = installation.ProjectRoot
	arguments = stageOutcomeInstructions(templateStage, arguments)
	arguments, err := FeatureScopeArguments(request.ProjectRoot, request.FeatureDirectory, arguments)
	if err != nil {
		return StageResult{}, err
	}
	prompt, err := StagePrompt(installation, templateStage, arguments)
	if err != nil {
		return StageResult{}, err
	}
	request.Stage = stage
	request.Prompt = prompt
	request.ReadOnly = stage == "analyze"
	invocation, err := adapter.Build(request)
	if err != nil {
		return StageResult{}, err
	}
	if invocation.Dir != installation.ProjectRoot {
		return StageResult{}, fmt.Errorf("PERMISSION_DENIED: provider invocation working directory differs from canonical Spec-Kit root")
	}
	invocation.ExtraFiles = append(invocation.ExtraFiles, request.InheritedFiles...)
	invocation.ExpectedExecutablePath = request.Profile.ApprovedExecutablePath
	invocation.ExpectedExecutableSHA256 = request.Profile.ApprovedExecutableSHA256
	invocation.ExpectedWorkingDirectory = installation.ProjectRoot
	invocation.ExpectedConfigDirectory = request.Profile.ApprovedConfigDirectory
	invocation.EnforceConfigDirectory = request.Profile.ApprovedExecutablePath != ""
	var stream *adapters.OutputStream
	if streaming, ok := adapter.(adapters.StreamingAdapter); ok {
		stream = streaming.NewOutputStream()
		invocation.StreamStdout = stream.Feed
	}
	process, runErr := runner.Run(ctx, invocation)
	result := StageResult{Process: process, Output: process.Stdout}
	if stream != nil {
		streamErr := stream.Close()
		if process.StdoutTruncated && streamErr == nil {
			streamErr = adapters.NewProviderError("PROVIDER_STREAM_LIMIT", "provider output exceeded the retained stream limit")
		}
		if runErr == nil && streamErr != nil {
			runErr = streamErr
		}
		result.Events = stream.Events()
		result.Output = stream.TerminalText()
	}
	if runErr != nil {
		return result, runErr
	}
	if stream == nil {
		result.Events, err = adapter.Normalize([]byte(process.Stdout))
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func stageOutcomeInstructions(stage, arguments string) string {
	var instruction string
	switch stage {
	case "clarify":
		instruction = "If no clarification is needed, make no artifact changes and emit exactly one standalone line: SPECTIDE_NO_CLARIFICATION: <specific reason>. Do not claim this outcome if the selected spec needs clarification."
	case "implement":
		instruction = "If the selected task is already satisfied, make no workspace changes and emit exactly one standalone line: SPECTIDE_NO_WORK: <specific evidence>. Do not use this marker when implementation changes are needed."
	default:
		return arguments
	}
	if strings.TrimSpace(arguments) == "" {
		return instruction
	}
	return strings.TrimSpace(arguments) + "\n\n" + instruction
}
