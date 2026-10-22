// Package speckit discovers installed Spec-Kit integrations without pretending
// that agent instruction files are standalone shell commands.
package speckit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Installation struct {
	Binary       string
	ProjectRoot  string
	Integration  string
	Commands     map[string]string
	CommandFiles map[string]string
}

func Discover(root string) (Installation, error) {
	abs, e := filepath.Abs(root)
	if e != nil {
		return Installation{}, e
	}
	info, e := os.Stat(filepath.Join(abs, ".specify"))
	if e != nil || !info.IsDir() {
		return Installation{}, fmt.Errorf("SPEC_KIT_NOT_INITIALIZED: .specify is missing")
	}
	bin, e := exec.LookPath("specify")
	if e != nil {
		return Installation{ProjectRoot: abs}, fmt.Errorf("EXECUTABLE_NOT_FOUND: specify")
	}
	commands := map[string]string{}
	files := map[string]string{}
	integration := ""
	manifestDir := filepath.Join(abs, ".specify", "integrations")
	manifests, _ := filepath.Glob(filepath.Join(manifestDir, "*.manifest.json"))
	for _, manifestPath := range manifests {
		b, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest struct {
			Integration string            `json:"integration"`
			Files       map[string]string `json:"files"`
		}
		if json.Unmarshal(b, &manifest) != nil {
			continue
		}
		for rel := range manifest.Files {
			if stage, ok := stageFromPath(rel); ok {
				target := filepath.Join(abs, filepath.FromSlash(rel))
				if _, err = os.Stat(target); err == nil {
					commands[stage] = "/speckit." + stage
					files[stage] = rel
					if integration == "" {
						integration = manifest.Integration
					}
				}
			}
		}
	}
	if integration == "" {
		integration = "unknown"
	}
	return Installation{Binary: bin, ProjectRoot: abs, Integration: integration, Commands: commands, CommandFiles: files}, nil
}
func RequireStage(in Installation, stage string) (string, error) {
	cmd := in.Commands[stage]
	if cmd == "" {
		return "", fmt.Errorf("SPEC_KIT_STAGE_UNAVAILABLE: %s", stage)
	}
	return cmd, nil
}

func stageFromPath(rel string) (string, bool) {
	known := []string{"constitution", "specify", "clarify", "plan", "checklist", "tasks", "analyze", "implement", "converge"}
	s := filepath.ToSlash(rel)
	for _, stage := range known {
		if strings.HasSuffix(s, "/speckit."+stage+".md") || strings.HasSuffix(s, "/speckit-"+stage+"/SKILL.md") {
			return stage, true
		}
	}
	return "", false
}

// StagePrompt expands an installed official command template as a prompt. It
// never executes markdown or treats workspace command files as shell scripts.
func StagePrompt(in Installation, stage, arguments string) (string, error) {
	if _, err := RequireStage(in, stage); err != nil {
		return "", err
	}
	rel := in.CommandFiles[stage]
	if rel == "" {
		return "", fmt.Errorf("SPEC_KIT_COMMAND_TEMPLATE_UNAVAILABLE: %s", stage)
	}
	root, err := filepath.EvalSymlinks(in.ProjectRoot)
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	r, err := filepath.Rel(root, target)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("PERMISSION_DENIED: Spec-Kit command template escapes project root")
	}
	b, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	body := string(b)
	if strings.HasPrefix(body, "---") {
		if i := strings.Index(body[3:], "\n---"); i >= 0 {
			body = body[3+i+4:]
			body = strings.TrimLeft(body, "\r\n")
		}
	}
	if strings.Contains(body, "$ARGUMENTS") {
		body = strings.ReplaceAll(body, "$ARGUMENTS", arguments)
	} else if strings.TrimSpace(arguments) != "" {
		body += "\n\nUser task: " + arguments
	}
	return body, nil
}
