package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/spectide/spectide/internal/runner"
	"os/exec"
	"strings"
	"time"
)

const maxEventLine = 1 << 20

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
	Provider          ProviderName `json:"provider"`
	ExecutableFound   bool         `json:"executable_found"`
	Version           string       `json:"version"`
	Authenticated     string       `json:"authenticated"`
	StreamingEvents   bool         `json:"streaming_events"`
	ModelOverride     bool         `json:"model_override"`
	EffortOverride    string       `json:"effort_override"`
	ReadOnlyExecution bool         `json:"read_only_execution"`
	WorkspaceWrite    bool         `json:"workspace_write"`
	Warnings          []string     `json:"warnings"`
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
	c := Capability{Provider: p.Provider, Authenticated: "NOT_PROBED", Warnings: []string{"Authentication is never read from credential files."}}
	path, e := exec.LookPath(p.Executable)
	if e != nil {
		return c, fmt.Errorf("EXECUTABLE_NOT_FOUND: %s", p.Executable)
	}
	c.ExecutableFound = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, runner.Invocation{Binary: path, Args: []string{"--version"}})
	if err != nil {
		return c, fmt.Errorf("PROVIDER_VERSION_PROBE_FAILED: %w", err)
	}
	c.Version = strings.TrimSpace(result.Stdout)
	if c.Version == "" {
		c.Version = strings.TrimSpace(result.Stderr)
	}
	if c.Version == "" {
		c.Version = "unknown"
	}
	return c, nil
}

// NormalizeJSONL accepts newline-delimited provider objects while discarding
// payload bodies that may contain user code, credentials, or private reasoning.
func NormalizeJSONL(provider ProviderName, raw []byte) ([]Event, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 4096), maxEventLine)
	var events []Event
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("MALFORMED_PROVIDER_EVENT: %w", err)
		}
		typ, _ := record["type"].(string)
		if typ == "" {
			typ = "unknown"
		}
		events = append(events, Event{Type: string(provider) + "." + typ, Summary: "structured provider event"})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("PROVIDER_EVENT_LIMIT: %w", err)
	}
	return events, nil
}
