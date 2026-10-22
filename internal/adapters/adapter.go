package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/runner"
	"github.com/spectide/spectide/internal/security"
)

const maxEventLine = 1 << 20
const maxStreamEvents = 10000

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
	Name                     string
	Provider                 ProviderName
	Executable, ConfigDir    string
	AllowedProjects          []string
	ApprovedExecutablePath   string
	ApprovedExecutableSHA256 string
	ApprovedConfigDirectory  string
}
type Request struct {
	ProjectRoot, Stage, Prompt, Model, Effort string
	FeatureDirectory                          string
	EventCallback                             func(Event)
	ReadOnly                                  bool
	Timeout                                   time.Duration
	InheritedFiles                            []*os.File
	Profile                                   Profile
}
type Capability struct {
	Provider                ProviderName `json:"provider"`
	ExecutableFound         bool         `json:"executable_found"`
	ApprovedExecutablePath  string       `json:"approved_executable_path,omitempty"`
	ExecutableSHA256        string       `json:"executable_sha256,omitempty"`
	ApprovedConfigDirectory string       `json:"approved_config_directory,omitempty"`
	Version                 string       `json:"version"`
	Authenticated           string       `json:"authenticated"`
	StreamingEvents         bool         `json:"streaming_events"`
	ModelOverride           bool         `json:"model_override"`
	ModelCatalogStatus      string       `json:"model_catalog_status"`
	ModelValues             []string     `json:"model_values,omitempty"`
	AuthenticatedProviders  []string     `json:"authenticated_providers,omitempty"`
	EffortOverride          string       `json:"effort_override"`
	EffortValues            []string     `json:"effort_values,omitempty"`
	ReadOnlyExecution       bool         `json:"read_only_execution"`
	WorkspaceWrite          bool         `json:"workspace_write"`
	SafetyModes             []string     `json:"safety_modes,omitempty"`
	CommandCapabilities     []string     `json:"command_capabilities,omitempty"`
	Warnings                []string     `json:"warnings"`
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

type StreamingAdapter interface{ NewOutputStream() *OutputStream }

// OutputStream incrementally parses bounded provider JSONL and retains only
// normalized event metadata plus the final user-visible answer text.
type OutputStream struct {
	provider     ProviderName
	onEvent      func(Event)
	pending      []byte
	events       []Event
	terminal     strings.Builder
	total        int
	failed       error
	closed       bool
	terminalSeen bool
}

func NewOutputStream(provider ProviderName) *OutputStream     { return &OutputStream{provider: provider} }
func (s *OutputStream) SetEventCallback(callback func(Event)) { s.onEvent = callback }
func (s *OutputStream) Feed(chunk []byte) error {
	if s.closed {
		return NewProviderError("PROVIDER_STREAM_CLOSED", "provider output arrived after stream close")
	}
	if s.failed != nil {
		return s.failed
	}
	s.total += len(chunk)
	if s.total > maxEventLine {
		s.failed = NewProviderError("PROVIDER_STREAM_LIMIT", "provider output exceeds the stream limit")
		return s.failed
	}
	s.pending = append(s.pending, chunk...)
	for {
		i := bytes.IndexByte(s.pending, '\n')
		if i < 0 {
			break
		}
		line := bytes.TrimSpace(append([]byte(nil), s.pending[:i]...))
		s.pending = append(s.pending[:0], s.pending[i+1:]...)
		if len(line) > 0 {
			if err := s.consume(line); err != nil {
				s.failed = err
				return err
			}
		}
	}
	if len(s.pending) > maxEventLine {
		s.failed = NewProviderError("PROVIDER_FRAME_LIMIT", "provider frame exceeds the line limit")
		return s.failed
	}
	return nil
}
func (s *OutputStream) Close() error {
	if s.closed {
		return s.failed
	}
	s.closed = true
	if s.failed != nil {
		return s.failed
	}
	line := bytes.TrimSpace(s.pending)
	s.pending = nil
	if len(line) > 0 {
		if err := s.consume(line); err != nil {
			s.failed = err
			return err
		}
	}
	if !s.terminalSeen {
		s.failed = NewProviderError("PROVIDER_TERMINAL_EVENT_MISSING", "provider stream ended without a terminal result")
		return s.failed
	}
	if strings.TrimSpace(s.terminal.String()) == "" {
		s.failed = NewProviderError("PROVIDER_TERMINAL_OUTPUT_MISSING", "provider terminal result contained no user-visible output")
		return s.failed
	}
	return nil
}
func (s *OutputStream) Events() []Event { return append([]Event(nil), s.events...) }
func (s *OutputStream) TerminalText() string {
	return security.Redact(strings.TrimSpace(s.terminal.String()))
}
func (s *OutputStream) consume(line []byte) error {
	var record map[string]any
	if err := json.Unmarshal(line, &record); err != nil {
		return NewProviderError("MALFORMED_PROVIDER_EVENT", "provider event is not valid JSON")
	}
	typ, _ := record["type"].(string)
	if strings.TrimSpace(typ) == "" {
		return NewProviderError("UNSUPPORTED_PROVIDER_EVENT", "provider event has no supported type")
	}
	if versionValue, exists := record["version"]; exists {
		version, ok := versionValue.(string)
		if !ok || version != "1" {
			return NewProviderError("UNSUPPORTED_PROVIDER_EVENT_VERSION", "provider event version is not supported")
		}
	}
	if !supportedEventType(s.provider, typ) {
		return NewProviderError("UNSUPPORTED_PROVIDER_EVENT", "provider event type is not supported")
	}
	if typ == "error" || strings.HasSuffix(typ, ".failed") || typ == "turn.failed" || typ == "step_error" || typ == "rate_limits.error" || record["is_error"] == true {
		code := "PROVIDER_REPORTED_ERROR"
		if rateLimited, _ := record["rate_limited"].(bool); rateLimited || typ == "rate_limits.error" {
			code = "RATE_LIMITED"
		}
		return NewProviderError(code, "provider reported an execution error")
	}
	if typ == "result" {
		if subtype, ok := record["subtype"].(string); ok && subtype != "success" {
			return NewProviderError("PROVIDER_REPORTED_ERROR", "provider reported an unsuccessful result")
		}
	}
	if err := validateEventShape(s.provider, typ, record); err != nil {
		return err
	}
	if len(s.events) >= maxStreamEvents {
		return NewProviderError("PROVIDER_EVENT_LIMIT", "provider event count exceeds the stream limit")
	}
	event := Event{Type: string(s.provider) + "." + typ, Summary: "structured provider event"}
	s.events = append(s.events, event)
	if s.onEvent != nil {
		s.onEvent(event)
	}
	if terminalEvent(s.provider, typ) {
		s.terminalSeen = true
	}
	s.extract(typ, record)
	return nil
}

func supportedEventType(provider ProviderName, typ string) bool {
	var supported []string
	switch provider {
	case Claude:
		supported = []string{"system", "assistant", "content_block_start", "content_block_delta", "content_block_stop", "message_start", "message_delta", "message_stop", "result", "ping", "error"}
	case Codex:
		supported = []string{"thread.started", "turn.started", "turn.completed", "turn.failed", "item.started", "item.updated", "item.completed", "rate_limits", "rate_limits.error", "error"}
	case OpenCode:
		supported = []string{"step_start", "text", "reasoning", "tool_use", "tool_result", "step_finish", "session.idle", "step_error", "error"}
	default:
		return false
	}
	for _, candidate := range supported {
		if typ == candidate {
			return true
		}
	}
	return false
}

func terminalEvent(provider ProviderName, typ string) bool {
	switch provider {
	case Claude:
		return typ == "result"
	case Codex:
		return typ == "turn.completed"
	case OpenCode:
		return typ == "step_finish" || typ == "session.idle"
	default:
		return false
	}
}

func validateEventShape(provider ProviderName, typ string, record map[string]any) error {
	if provider == Claude && typ == "result" {
		if _, ok := record["result"].(string); !ok {
			return NewProviderError("PROVIDER_EVENT_SHAPE_INVALID", "provider terminal result has an invalid payload")
		}
	}
	if provider == Codex && typ == "turn.completed" {
		if _, ok := record["last_agent_message"].(string); !ok {
			return NewProviderError("PROVIDER_EVENT_SHAPE_INVALID", "provider terminal result has an invalid payload")
		}
	}
	if provider == OpenCode && typ == "text" {
		if _, ok := record["text"].(string); !ok {
			return NewProviderError("PROVIDER_EVENT_SHAPE_INVALID", "provider text event has an invalid payload")
		}
	}
	return nil
}
func (s *OutputStream) extract(typ string, record map[string]any) {
	appendText := func(value string) {
		if value != "" && s.terminal.Len()+len(value) <= maxEventLine {
			s.terminal.WriteString(value)
		}
	}
	resetText := func(value string) {
		if value != "" && len(value) <= maxEventLine {
			s.terminal.Reset()
			s.terminal.WriteString(value)
		}
	}
	textField, _ := record["text"].(string)
	switch s.provider {
	case Claude:
		if typ == "result" {
			if value, ok := record["result"].(string); ok {
				resetText(value)
				return
			}
		}
		if typ == "content_block_delta" {
			if delta, ok := record["delta"].(map[string]any); ok {
				value, _ := delta["text"].(string)
				appendText(value)
				return
			}
		}
		if typ == "assistant" {
			if message, ok := record["message"].(map[string]any); ok {
				if parts, ok := message["content"].([]any); ok {
					for _, part := range parts {
						if item, ok := part.(map[string]any); ok {
							value, _ := item["text"].(string)
							appendText(value)
						}
					}
				}
			}
		}
	case Codex:
		if typ == "turn.completed" {
			if value, ok := record["last_agent_message"].(string); ok {
				resetText(value)
				return
			}
		}
		if typ == "item.completed" {
			if item, ok := record["item"].(map[string]any); ok && item["type"] == "agent_message" {
				value, _ := item["text"].(string)
				appendText(value)
				return
			}
		}
		if typ == "message" {
			appendText(textField)
		}
	case OpenCode:
		if part, ok := record["part"].(map[string]any); ok && part["type"] == "text" {
			value, _ := part["text"].(string)
			appendText(value)
			return
		}
		if typ == "text" || textField != "" {
			appendText(textField)
		}
	}
}

func ProbeBinary(p Profile) (Capability, error) {
	c := Capability{Provider: p.Provider, Authenticated: "NOT_PROBED", Warnings: []string{"SpecTide does not open or persist provider credential files."}}
	reference, e := security.ResolveExecutableReference(p.Executable)
	if e != nil {
		return c, WrapProviderError("EXECUTABLE_NOT_FOUND", p.Executable, e)
	}
	c.ExecutableFound = true
	c.ApprovedExecutablePath = reference.Path
	c.ExecutableSHA256 = reference.SHA256
	configDirectory, e := security.ResolveConfigDirectory(p.ConfigDir)
	if e != nil {
		return c, WrapProviderError("CONFIG_DIRECTORY_INVALID", "provider configuration reference is invalid", e)
	}
	c.ApprovedConfigDirectory = configDirectory
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, runner.Invocation{Binary: reference.Path, Args: []string{"--version"}, ExpectedExecutablePath: reference.Path, ExpectedExecutableSHA256: reference.SHA256})
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

// ProbeProvider gathers only provider-supported readiness and CLI capability
// facts. Provider output is parsed in memory and is never returned or stored.
func ProbeProvider(ctx context.Context, p Profile) (Capability, error) {
	c, err := ProbeBinary(p)
	if err != nil {
		return c, err
	}
	env := []string{}
	if p.ConfigDir != "" {
		key := map[ProviderName]string{Claude: "CLAUDE_CONFIG_DIR", Codex: "CODEX_HOME", OpenCode: "OPENCODE_CONFIG_DIR"}[p.Provider]
		if key != "" {
			env = append(env, key+"="+c.ApprovedConfigDirectory)
		}
	}
	ref := runner.Invocation{Binary: c.ApprovedExecutablePath, Env: env, Dir: os.TempDir(), Timeout: 5 * time.Second, ExpectedExecutablePath: c.ApprovedExecutablePath, ExpectedExecutableSHA256: c.ExecutableSHA256}
	probe := func(args ...string) (string, bool) {
		inv := ref
		inv.Args = args
		r, e := runner.Run(ctx, inv)
		return strings.TrimSpace(r.Stdout + "\n" + r.Stderr), e == nil
	}
	switch p.Provider {
	case Claude:
		status, ok := probe("auth", "status", "--json")
		c.Authenticated = claudeAuthStatus(status, ok)
		help, helpOK := probe("--help")
		if helpOK {
			c.ModelOverride = strings.Contains(help, "--model")
			c.CommandCapabilities = []string{"version", "auth-status", "help"}
			c.EffortValues = valuesAfterFlag(help, "--effort")
			c.EffortOverride = capabilityOverride(c.EffortValues)
			if strings.Contains(help, "plan") {
				c.ReadOnlyExecution = true
				c.SafetyModes = append(c.SafetyModes, "read-only")
			}
			if strings.Contains(help, "acceptEdits") {
				c.WorkspaceWrite = true
				c.SafetyModes = append(c.SafetyModes, "workspace-write")
			}
		}
		c.ModelCatalogStatus = "UNAVAILABLE"
		c.Warnings = append(c.Warnings, "Provider CLI does not expose a verified model catalog.")
	case Codex:
		status, ok := probe("login", "status")
		c.Authenticated = textAuthStatus(status, ok)
		help, helpOK := probe("exec", "--help")
		if helpOK {
			c.ModelOverride = strings.Contains(help, "--model")
			c.CommandCapabilities = []string{"version", "login-status", "exec-help"}
			if strings.Contains(help, "read-only") {
				c.ReadOnlyExecution = true
				c.SafetyModes = append(c.SafetyModes, "read-only")
			}
			if strings.Contains(help, "workspace-write") {
				c.WorkspaceWrite = true
				c.SafetyModes = append(c.SafetyModes, "workspace-write")
			}
		}
		c.ModelCatalogStatus = "UNAVAILABLE"
		c.Warnings = append(c.Warnings, "Provider CLI does not expose a verified model catalog or effort catalog.")
	case OpenCode:
		auth, ok := probe("auth", "list", "--format", "json")
		c.AuthenticatedProviders = parseOpenCodeAuthProviders(auth, ok)
		if len(c.AuthenticatedProviders) > 0 {
			c.Authenticated = "AUTHENTICATED"
		} else if ok {
			c.Authenticated = "UNAUTHENTICATED"
		} else {
			c.Authenticated = "UNKNOWN"
		}
		models, modelsOK := probe("models")
		c.ModelValues = parseModelIDs(models, modelsOK)
		if len(c.ModelValues) > 0 {
			c.ModelCatalogStatus = "VERIFIED"
		} else {
			c.ModelCatalogStatus = "UNAVAILABLE"
		}
		help, helpOK := probe("run", "--help")
		if helpOK {
			c.ModelOverride = strings.Contains(help, "--model")
			c.CommandCapabilities = []string{"version", "auth-list", "models", "run-help"}
		}
		c.Warnings = append(c.Warnings, "Read-only and workspace-write execution controls are not verified for OpenCode.")
	default:
		c.Authenticated = "NOT_PROBED"
		c.ModelCatalogStatus = "UNAVAILABLE"
	}
	return c, nil
}

func capabilityOverride(values []string) string {
	if len(values) > 0 {
		return "verified"
	}
	return "unsupported"
}
func valuesAfterFlag(help, flag string) []string {
	idx := strings.Index(help, flag)
	if idx < 0 {
		return nil
	}
	part := help[idx+len(flag):]
	if len(part) > 256 {
		part = part[:256]
	}
	// Provider help renders enum values as (a, b, c) or [a|b|c].
	start := strings.IndexAny(part, "([{")
	if start < 0 {
		return nil
	}
	end := strings.IndexAny(part[start+1:], ")]}")
	if end < 0 {
		return nil
	}
	chunk := part[start+1 : start+1+end]
	chunk = strings.ReplaceAll(chunk, "|", ",")
	var out []string
	for _, v := range strings.Split(chunk, ",") {
		v = strings.TrimSpace(strings.Trim(v, "`'\""))
		if validCapabilityValue(v) {
			out = append(out, v)
		}
	}
	return uniqueSorted(out)
}
func validCapabilityValue(v string) bool {
	if v == "" || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:/-", r)) {
			return false
		}
	}
	return true
}
func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func claudeAuthStatus(raw string, ok bool) string {
	var v struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if json.Unmarshal([]byte(raw), &v) == nil {
		if v.LoggedIn {
			return "AUTHENTICATED"
		}
		return "UNAUTHENTICATED"
	}
	return "UNKNOWN"
}
func textAuthStatus(raw string, ok bool) string {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "not logged in") || strings.Contains(lower, "not authenticated") {
		return "UNAUTHENTICATED"
	}
	if ok && (strings.Contains(lower, "logged in") || strings.Contains(lower, "authenticated")) {
		return "AUTHENTICATED"
	}
	return "UNKNOWN"
}
func parseOpenCodeAuthProviders(raw string, ok bool) []string {
	if !ok {
		return nil
	}
	var v map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &v) != nil {
		return nil
	}
	out := []string{}
	for name, data := range v {
		if !validCapabilityValue(name) {
			continue
		}
		var items []json.RawMessage
		if json.Unmarshal(data, &items) == nil && len(items) > 0 {
			out = append(out, name)
		}
	}
	return uniqueSorted(out)
}
func parseModelIDs(raw string, ok bool) []string {
	if !ok {
		return nil
	}
	out := []string{}
	for _, line := range strings.Fields(raw) {
		line = strings.Trim(line, "`'\"[](),")
		if strings.Contains(line, "/") && validCapabilityValue(line) {
			out = append(out, line)
		}
	}
	return uniqueSorted(out)
}

// NormalizeJSONL accepts newline-delimited provider objects while discarding
// payload bodies that may contain user code, credentials, or private reasoning.
func NormalizeJSONL(provider ProviderName, raw []byte) ([]Event, error) {
	stream := NewOutputStream(provider)
	if err := stream.Feed(raw); err != nil {
		return nil, err
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}
	return stream.Events(), nil
}
