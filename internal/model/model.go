package model

import (
	"sort"
	"time"
)

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
	StageRun  StageState = "RUNNING"
	Succeeded StageState = "SUCCEEDED"
	StageFail StageState = "FAILED"
	Skipped   StageState = "SKIPPED_WITH_REASON"
)

type Run struct {
	ID        string     `json:"id"`
	Project   string     `json:"project"`
	Prompt    string     `json:"prompt"`
	State     RunState   `json:"state"`
	Execution string     `json:"execution,omitempty"`
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
	Run               Run                `json:"run"`
	Acceptance        AcceptanceSummary  `json:"acceptance"`
	ReleaseGates      []ReleaseGate      `json:"release_gates"`
	Configuration     *RunConfiguration  `json:"configuration,omitempty"`
	Stages            []Stage            `json:"stages"`
	Attempts          []ExecutionAttempt `json:"execution_attempts"`
	QualityExecutions []QualityExecution `json:"quality_executions"`
	Decisions         []RoutingDecision  `json:"routing_decisions"`
	Gates             []Gate             `json:"gates"`
	Artifacts         []Artifact         `json:"artifacts"`
	Checkpoints       []Checkpoint       `json:"checkpoints"`
	Approvals         []Approval         `json:"approvals"`
}

// ReleaseGate is the stable release-evidence projection. Detailed runtime
// gates remain in Report.Gates; this taxonomy makes release reports and the
// acceptance ledger comparable without renaming or discarding those details.
type ReleaseGate struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Verdict  string `json:"verdict"`
	Evidence string `json:"evidence"`
}

var releaseGateDefinitions = []struct {
	ID, Name, DetailKey string
}{
	{"QG-01", "requirement traceability", "requirement_traceability"},
	{"QG-02", "spec/plan/tasks consistency", "analyze_clean"},
	{"QG-03", "checklist acceptance", "checklist_acceptance"},
	{"QG-04", "task completeness", "task_completeness"},
	{"QG-05", "build", "build"},
	{"QG-06", "static checks", "static_checks"},
	{"QG-07", "automated tests", "required_tests"},
	{"QG-08", "convergence", "converged"},
	{"QG-09", "provider compatibility", "provider_compatibility"},
	{"QG-10", "recovery", "recovery"},
	{"QG-11", "security", "security"},
	{"QG-12", "final review", "final_review"},
	{"QG-13", "documentation", "documentation"},
}

// ReleaseGateTaxonomy projects stable release IDs from detailed run gates.
// Missing source evidence is explicit NOT_RUN, never an inferred pass.
func ReleaseGateTaxonomy(gates []Gate) []ReleaseGate {
	byKey := make(map[string]Gate, len(gates))
	for _, gate := range gates {
		byKey[gate.Key] = gate
	}
	projected := make([]ReleaseGate, 0, len(releaseGateDefinitions))
	for _, definition := range releaseGateDefinitions {
		gate, ok := byKey[definition.DetailKey]
		if !ok {
			projected = append(projected, ReleaseGate{ID: definition.ID, Name: definition.Name, Verdict: "NOT_RUN", Evidence: "no detailed evidence was recorded for this release gate"})
			continue
		}
		projected = append(projected, ReleaseGate{ID: definition.ID, Name: definition.Name, Verdict: gate.Verdict, Evidence: gate.Evidence})
	}
	return projected
}

type AcceptanceSummary struct {
	Verdict         string `json:"verdict"`
	Execution       string `json:"execution"`
	RequiredGates   []Gate `json:"required_gates"`
	SimulationGates []Gate `json:"simulation_gates,omitempty"`
}

// SummarizeAcceptance distinguishes evidence that satisfies live/local gates
// from deterministic simulation evidence. Missing gates remain NOT_RUN.
func SummarizeAcceptance(decisions []RoutingDecision, gates []Gate) AcceptanceSummary {
	hasLive, hasSimulated := false, false
	for _, decision := range decisions {
		if decision.Stage == "tests" || decision.Stage == "report" || decision.Provider == "local" {
			continue
		}
		if decision.Provider == "fake" {
			hasSimulated = true
		} else {
			hasLive = true
		}
	}
	execution := "SIMULATED"
	if hasLive && hasSimulated {
		execution = "MIXED"
	} else if hasLive {
		execution = "LIVE"
	}
	byKey := make(map[string]Gate, len(gates))
	for _, gate := range gates {
		byKey[gate.Key] = gate
	}
	keys := []string{"analyze_clean", "required_tests", "converged", "final_review", "workflow_execution"}
	if !hasLive {
		keys = append(keys, "simulated_e2e")
	}
	summary := AcceptanceSummary{Execution: execution}
	allPass := true
	hasFail := false
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		gate, ok := byKey[key]
		if !ok {
			gate = Gate{Key: key, Verdict: "NOT_RUN", Evidence: "required evidence was not recorded", Required: true, Applicable: true, PolicySet: true}
		}
		if key == "simulated_e2e" && hasLive {
			gate.Required, gate.Applicable, gate.PolicySet = false, false, true
		}
		if key == "workflow_execution" {
			gate.Required, gate.Applicable, gate.PolicySet = true, true, true
		}
		if !gate.PolicySet && !gate.Required && !gate.Applicable {
			gate.Required, gate.Applicable = true, true
		}
		if !gate.Required || !gate.Applicable {
			continue
		}
		summary.RequiredGates = append(summary.RequiredGates, gate)
		seen[key] = true
		if gate.Verdict != "PASS" {
			allPass = false
		}
		if gate.Verdict == "FAIL" {
			hasFail = true
		}
	}
	additional := make([]Gate, 0)
	for _, gate := range gates {
		if seen[gate.Key] || (gate.Key == "simulated_e2e" && hasLive) {
			continue
		}
		if !gate.PolicySet && !gate.Required && !gate.Applicable {
			gate.Required, gate.Applicable = true, true
		}
		if gate.Required && gate.Applicable {
			additional = append(additional, gate)
		}
	}
	sort.Slice(additional, func(i, j int) bool { return additional[i].Key < additional[j].Key })
	for _, gate := range additional {
		summary.RequiredGates = append(summary.RequiredGates, gate)
		if gate.Verdict != "PASS" {
			allPass = false
		}
		if gate.Verdict == "FAIL" {
			hasFail = true
		}
	}
	if hasLive {
		gate, ok := byKey["simulated_e2e"]
		if !ok {
			gate = Gate{Key: "simulated_e2e", Verdict: "NOT_RUN", Evidence: "simulation-only check does not apply to live execution", Required: false, Applicable: false, PolicySet: true}
		}
		summary.SimulationGates = append(summary.SimulationGates, gate)
	}
	if allPass {
		if hasLive {
			summary.Verdict = "PASS"
		} else {
			summary.Verdict = "SIMULATED"
		}
	} else if hasFail {
		summary.Verdict = "FAIL"
	} else {
		summary.Verdict = "BLOCKED"
	}
	return summary
}

// ExecutionAttempt records one actual or attempted provider invocation. Its
// identity/route are fixed at creation; only the RUNNING result is finalized.
type ExecutionAttempt struct {
	ID                 string     `json:"id"`
	RunID              string     `json:"run_id"`
	Stage              string     `json:"stage"`
	Phase              string     `json:"phase"`
	Iteration          int        `json:"iteration"`
	Provider           string     `json:"provider"`
	Profile            string     `json:"profile"`
	Model              string     `json:"model"`
	Effort             string     `json:"effort,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	StartEventSeq      int64      `json:"start_event_seq"`
	CompletionEventSeq int64      `json:"completion_event_seq,omitempty"`
	ProcessID          int        `json:"process_id,omitempty"`
	DurationMillis     int64      `json:"duration_ms"`
	ExitCode           int        `json:"exit_code"`
	Result             string     `json:"result"`
	Error              string     `json:"error,omitempty"`
	StdoutSHA256       string     `json:"stdout_sha256,omitempty"`
	StderrSHA256       string     `json:"stderr_sha256,omitempty"`
	EvidenceJSON       string     `json:"evidence_json"`
	ArtifactIDs        []string   `json:"artifact_ids,omitempty"`
}

type QualityExecution struct {
	ID                 string          `json:"id"`
	RunID              string          `json:"run_id"`
	Phase              string          `json:"phase"`
	Iteration          int             `json:"iteration"`
	ProcessID          int             `json:"process_id"`
	StartedAt          time.Time       `json:"started_at"`
	FinishedAt         time.Time       `json:"finished_at"`
	CompletionEventSeq int64           `json:"completion_event_seq"`
	Verdict            string          `json:"verdict"`
	Results            []QualityResult `json:"results"`
}

type QualityResult struct {
	Key       string        `json:"key"`
	Verdict   string        `json:"verdict"`
	Command   []string      `json:"command"`
	ProcessID int           `json:"process_id"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration_ns"`
	Summary   string        `json:"summary"`
	StartedAt time.Time     `json:"started_at"`
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
	Key        string `json:"key"`
	Verdict    string `json:"verdict"`
	Evidence   string `json:"evidence"`
	Required   bool   `json:"required"`
	Applicable bool   `json:"applicable"`
	PolicySet  bool   `json:"-"`
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

const (
	ApprovalPending  = "PENDING"
	ApprovalApproved = "APPROVED"
	ApprovalDenied   = "DENIED"
)

type Profile struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Provider        string             `json:"provider"`
	Executable      string             `json:"executable"`
	ConfigDirectory string             `json:"config_directory,omitempty"`
	AllowedScopes   []string           `json:"allowed_scopes"`
	Capability      *ProfileCapability `json:"capability,omitempty"`
}

// ProfileCapability contains non-secret, observed provider readiness. It is
// deliberately separate from profile configuration and never stores tokens.
type ProfileCapability struct {
	Provider                string    `json:"provider"`
	ExecutableFound         bool      `json:"executable_found"`
	ApprovedExecutablePath  string    `json:"approved_executable_path,omitempty"`
	ExecutableSHA256        string    `json:"executable_sha256,omitempty"`
	ApprovedConfigDirectory string    `json:"approved_config_directory,omitempty"`
	Version                 string    `json:"version"`
	Authenticated           string    `json:"authenticated"`
	StreamingEvents         bool      `json:"streaming_events"`
	ModelOverride           bool      `json:"model_override"`
	ModelCatalogStatus      string    `json:"model_catalog_status"`
	ModelValues             []string  `json:"model_values,omitempty"`
	AuthenticatedProviders  []string  `json:"authenticated_providers,omitempty"`
	EffortOverride          string    `json:"effort_override"`
	EffortValues            []string  `json:"effort_values,omitempty"`
	ReadOnlyExecution       bool      `json:"read_only_execution"`
	WorkspaceWrite          bool      `json:"workspace_write"`
	SafetyModes             []string  `json:"safety_modes,omitempty"`
	CommandCapabilities     []string  `json:"command_capabilities,omitempty"`
	Warnings                []string  `json:"warnings,omitempty"`
	ObservedAt              time.Time `json:"observed_at"`
}

type Project struct {
	ID            string   `json:"id"`
	CanonicalPath string   `json:"canonical_path"`
	DisplayName   string   `json:"display_name"`
	AllowedScopes []string `json:"allowed_scopes"`
}
