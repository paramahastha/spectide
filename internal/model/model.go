package model

import "time"

type RunState string

const (
	Created   RunState = "CREATED"
	Running   RunState = "RUNNING"
	Completed RunState = "COMPLETED"
	Blocked   RunState = "BLOCKED"
	Failed    RunState = "FAILED"
	Cancelled RunState = "CANCELLED"
	Paused    RunState = "PAUSED"
)

type StageState string

const (
	Pending   StageState = "PENDING"
	Succeeded StageState = "SUCCEEDED"
	StageFail StageState = "FAILED"
	Skipped   StageState = "SKIPPED_WITH_REASON"
)

type Run struct {
	ID        string     `json:"id"`
	Project   string     `json:"project"`
	Prompt    string     `json:"prompt"`
	State     RunState   `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type RunConfiguration struct {
	RunID      string    `json:"run_id"`
	ConfigJSON string    `json:"config_json"`
	SHA256     string    `json:"sha256"`
	CreatedAt  time.Time `json:"created_at"`
}

type Stage struct {
	ID       string     `json:"id"`
	RunID    string     `json:"run_id"`
	Key      string     `json:"key"`
	Position int        `json:"position"`
	State    StageState `json:"state"`
	Provider string     `json:"provider"`
	Profile  string     `json:"profile"`
	Model    string     `json:"model"`
	Effort   string     `json:"effort"`
	Attempt  int        `json:"attempt"`
}

type Event struct {
	RunID     string         `json:"run_id"`
	Seq       int64          `json:"seq"`
	Timestamp time.Time      `json:"timestamp"`
	Stage     string         `json:"stage,omitempty"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Payload   map[string]any `json:"payload"`
}

type Report struct {
	Run           Run               `json:"run"`
	Configuration *RunConfiguration `json:"configuration,omitempty"`
	Stages        []Stage           `json:"stages"`
	Decisions     []RoutingDecision `json:"routing_decisions"`
	Gates         []Gate            `json:"gates"`
	Artifacts     []Artifact        `json:"artifacts"`
	Checkpoints   []Checkpoint      `json:"checkpoints"`
	Approvals     []Approval        `json:"approvals"`
}

type RoutingDecision struct {
	Stage       string `json:"stage"`
	Provider    string `json:"provider"`
	Profile     string `json:"profile"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Policy      string `json:"policy"`
	Explanation string `json:"explanation"`
}

type Gate struct {
	Key      string `json:"key"`
	Verdict  string `json:"verdict"`
	Evidence string `json:"evidence"`
}

type Artifact struct {
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	Stage      string    `json:"stage"`
	Path       string    `json:"path"`
	Kind       string    `json:"kind"`
	SHA256     string    `json:"sha256"`
	ObservedAt time.Time `json:"observed_at"`
}

type Checkpoint struct {
	ID                   string    `json:"id"`
	RunID                string    `json:"run_id"`
	Stage                string    `json:"stage"`
	StateJSON            string    `json:"state_json"`
	WorkspaceFingerprint string    `json:"workspace_fingerprint"`
	CreatedAt            time.Time `json:"created_at"`
}

type Approval struct {
	ID         string     `json:"id"`
	RunID      string     `json:"run_id"`
	Action     string     `json:"action"`
	Status     string     `json:"status"`
	Decision   string     `json:"decision"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

type Profile struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Provider        string   `json:"provider"`
	Executable      string   `json:"executable"`
	ConfigDirectory string   `json:"config_directory,omitempty"`
	AllowedScopes   []string `json:"allowed_scopes"`
}

type Project struct {
	ID            string   `json:"id"`
	CanonicalPath string   `json:"canonical_path"`
	DisplayName   string   `json:"display_name"`
	AllowedScopes []string `json:"allowed_scopes"`
}
