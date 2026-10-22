package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/spectide/spectide/internal/runner"
	"os/exec"
	"strings"
	"time"
)

type ProviderName string

const (
	Claude   ProviderName = "claude"
	Codex    ProviderName = "codex"
	OpenCode ProviderName = "opencode"
	Fake     ProviderName = "fake"
)

type Profile struct {
	Name                  string
	Provider              ProviderName
	Executable, ConfigDir string
	AllowedProjects       []string
}
type Request struct {
	ProjectRoot, Stage, Prompt, Model, Effort string
	ReadOnly                                  bool
	Timeout                                   time.Duration
	Profile                                   Profile
}
type Capability struct {
	Provider        ProviderName `json:"provider"`
	ExecutableFound bool         `json:"executable_found"`
	Version         string       `json:"version"`
	Authenticated   string       `json:"authenticated"`
	StreamingEvents bool         `json:"streaming_events"`
	ModelOverride   bool         `json:"model_override"`
	EffortOverride  string       `json:"effort_override"`
	Warnings        []string     `json:"warnings"`
}
type Event struct {
	Type, Summary string
	Raw           json.RawMessage
}
type Adapter interface {
	Name() ProviderName
	Probe(context.Context, Profile) (Capability, error)
	Build(Request) (runner.Invocation, error)
	Normalize([]byte) ([]Event, error)
}

func ProbeBinary(p Profile) (Capability, error) {
	c := Capability{Provider: p.Provider, Authenticated: "unknown", Warnings: []string{"Authentication is never read from credential files."}}
	path, e := exec.LookPath(p.Executable)
	if e != nil {
		return c, fmt.Errorf("EXECUTABLE_NOT_FOUND: %s", p.Executable)
	}
	c.ExecutableFound = true
	c.Version = strings.TrimSpace(path)
	return c, nil
}
