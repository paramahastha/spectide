// Package speckit discovers installed Spec-Kit integrations without pretending
// that agent instruction files are standalone shell commands.
package speckit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Installation struct {
	Binary      string
	ProjectRoot string
	Integration string
	Commands    map[string]string
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
	dir := filepath.Join(abs, ".opencode", "commands")
	for _, key := range []string{"specify", "clarify", "plan", "checklist", "tasks", "analyze", "implement", "converge"} {
		p := filepath.Join(dir, "speckit."+key+".md")
		if _, err := os.Stat(p); err == nil {
			commands[key] = "/speckit." + key
		}
	}
	return Installation{Binary: bin, ProjectRoot: abs, Integration: "opencode", Commands: commands}, nil
}
func RequireStage(in Installation, stage string) (string, error) {
	cmd := in.Commands[stage]
	if cmd == "" {
		return "", fmt.Errorf("SPEC_KIT_STAGE_UNAVAILABLE: %s", stage)
	}
	return cmd, nil
}
