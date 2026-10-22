package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/spectide/spectide/internal/runner"
	"os"
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

// ProviderError gives callers a stable code while retaining a safe, readable
// message and the underlying cause for local diagnostics.
type ProviderError struct {
	Code       string
	Message    string
	RetryAfter time.Duration
	Cause      error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func (e *ProviderError) Unwrap() error { return e.Cause }

func NewProviderError(code, message string) *ProviderError {
	return &ProviderError{Code: code, Message: message}
}

func WrapProviderError(code, message string, cause error) *ProviderError {
	return &ProviderError{Code: code, Message: message, Cause: cause}
}

func NewRateLimitError(retryAfter time.Duration) *ProviderError {
	return &ProviderError{Code: "RATE_LIMITED", Message: "simulated provider rate limit", RetryAfter: retryAfter}
}

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
	InheritedFiles                            []*os.File
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
		return c, WrapProviderError("EXECUTABLE_NOT_FOUND", p.Executable, e)
	}
	c.ExecutableFound = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, runner.Invocation{Binary: path, Args: []string{"--version"}})
	if err != nil {
		return c, WrapProviderError("PROVIDER_VERSION_PROBE_FAILED", "provider version probe failed", err)
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
			return nil, WrapProviderError("MALFORMED_PROVIDER_EVENT", "provider event is not valid JSON", err)
		}
		typ, _ := record["type"].(string)
		if typ == "" {
			typ = "unknown"
		}
		events = append(events, Event{Type: string(provider) + "." + typ, Summary: "structured provider event"})
	}
	if err := scanner.Err(); err != nil {
		return nil, WrapProviderError("PROVIDER_EVENT_LIMIT", "provider event exceeds the line-size limit", err)
	}
	return events, nil
}
