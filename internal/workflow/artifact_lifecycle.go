package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spectide/spectide/internal/speckit"
)

// existingOwnerArtifact prevents create-once Spec-Kit stages from rerunning
// over an artifact that already defines the selected product scope.
func existingOwnerArtifact(projectRoot, featureDir, stage string) (string, bool, error) {
	if featureDir == "" {
		return "", false, nil
	}
	var artifactPath string
	switch stage {
	case "constitution":
		artifactPath = filepath.Join(projectRoot, ".specify", "memory", "constitution.md")
	case "specify":
		state, err := speckit.InspectArtifacts(featureDir)
		if err != nil {
			return "", false, err
		}
		if state.Present["spec"] {
			artifactPath = state.SpecPath
		}
	case "plan":
		state, err := speckit.InspectArtifacts(featureDir)
		if err != nil {
			return "", false, err
		}
		if state.Present["plan"] {
			artifactPath = state.PlanPath
		}
	case "tasks":
		state, err := speckit.InspectArtifacts(featureDir)
		if err != nil {
			return "", false, err
		}
		if state.Present["tasks"] {
			artifactPath = state.TasksPath
		}
	default:
		return "", false, nil
	}
	if artifactPath == "" {
		return "", false, nil
	}
	info, err := os.Stat(artifactPath)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("SPEC_KIT_ARTIFACT_NOT_REGULAR")
	}
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", false, err
	}
	resolved, err := filepath.EvalSymlinks(artifactPath)
	if err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("SPEC_KIT_ARTIFACT_PATH_ESCAPE")
	}
	return filepath.ToSlash(rel), true, nil
}

func stageCommandRequired(projectRoot, featureDir, stage string, specificationIntent ...string) (bool, error) {
	// An explicitly requested amendment is the one exception to Specify's
	// create-once policy: it updates the selected feature's existing spec.
	if stage == "specify" && len(specificationIntent) > 0 && specificationIntent[0] == specificationIntentAmend {
		return true, nil
	}
	_, present, err := existingOwnerArtifact(projectRoot, featureDir, stage)
	if err != nil {
		return false, err
	}
	return !present, nil
}
