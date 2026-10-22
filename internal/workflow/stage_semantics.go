package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/speckit"
)

var checklistItem = regexp.MustCompile(`(?m)^\s*[-*+]\s+\[[ xX]\]\s+\S.+$`)
var taskItem = regexp.MustCompile(`(?m)^\s*[-*+]\s+\[[ xX]\]\s+T[0-9]{3,}[a-z]?\s+\S.+$`)

func requireExplicitNoop(output, marker string) (string, error) {
	prefix := marker + ":"
	reason := ""
	count := 0
	fenced := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || line != trimmed {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) {
			count++
			reason = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	if count != 1 || len([]rune(reason)) < 12 || strings.ContainsAny(reason, "\r\n") {
		return "", fmt.Errorf("STAGE_EXPLICIT_NO_OP_REQUIRED: %s", marker)
	}
	return reason, nil
}

func validateExistingSpecification(featureDir string) error {
	artifacts, err := speckit.InspectArtifacts(featureDir)
	if err != nil {
		return err
	}
	if !artifacts.Present["spec"] {
		return fmt.Errorf("STAGE_NO_OP_ARTIFACT_MISSING: spec.md")
	}
	path := filepath.Join(featureDir, "spec.md")
	if err = validateMarkdownOwner(path, "spec.md", false); err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	normalized := strings.ToLower(string(content))
	if len(strings.TrimSpace(normalized)) < 100 || !strings.Contains(normalized, "## ") || !(strings.Contains(normalized, "requirement") || strings.Contains(normalized, "acceptance") || strings.Contains(normalized, "scenario")) {
		return fmt.Errorf("STAGE_NO_OP_SPECIFICATION_UNVERIFIED")
	}
	return nil
}

func validateExistingImplementation(projectRoot string) error {
	files, err := workspaceFiles(projectRoot)
	if err != nil {
		return err
	}
	for path, file := range files {
		ext := strings.ToLower(filepath.Ext(path))
		relative := filepath.ToSlash(path)
		if strings.HasPrefix(relative, "specs/") || strings.HasPrefix(relative, "vendor/") || strings.HasPrefix(relative, "node_modules/") || strings.HasPrefix(relative, "dist/") || strings.HasPrefix(relative, "build/") {
			continue
		}
		switch ext {
		case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rs", ".java", ".cs", ".rb", ".php", ".swift", ".kt":
			if file.SHA256 != "" {
				return nil
			}
		}
	}
	return fmt.Errorf("STAGE_NO_OP_IMPLEMENTATION_UNVERIFIED")
}

func validateChangedOwnerArtifact(stage, featureDir string, artifacts []model.Artifact) error {
	if len(artifacts) != 1 {
		return nil
	}
	var path string
	switch stage {
	case "specify", "clarify":
		path = filepath.Join(featureDir, "spec.md")
		return validateMarkdownOwner(path, "spec.md", false)
	case "plan":
		path = filepath.Join(featureDir, "plan.md")
		return validateMarkdownOwner(path, "plan.md", false)
	case "checklist":
		path = filepath.Join(featureDir, "checklists", "requirements.md")
		return validateMarkdownOwner(path, "checklist", true)
	case "tasks":
		path = filepath.Join(featureDir, "tasks.md")
		return validateMarkdownOwner(path, "tasks", true)
	default:
		return nil
	}
}

func validateMarkdownOwner(path, kind string, structured bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(string(data))
	if content == "" || !strings.Contains(content, "# ") {
		return fmt.Errorf("STAGE_OWNER_ARTIFACT_INVALID: %s", kind)
	}
	switch kind {
	case "checklist":
		if !checklistItem.MatchString(content) {
			return fmt.Errorf("STAGE_OWNER_ARTIFACT_INVALID: checklist has no requirement items")
		}
	case "tasks":
		if !taskItem.MatchString(content) {
			return fmt.Errorf("STAGE_OWNER_ARTIFACT_INVALID: task ledger has no stable task IDs")
		}
	default:
		if structured && !strings.Contains(content, "## ") {
			return fmt.Errorf("STAGE_OWNER_ARTIFACT_INVALID: %s has no sections", kind)
		}
	}
	return nil
}
