package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSharedWorkspaceProjectionsSerializeStableSafeFields(t *testing.T) {
	stamp := time.Date(2026, 10, 24, 1, 2, 3, 0, time.UTC)
	values := []any{
		WorkspaceStatus{WorkspaceID: "ws-1", Version: "test", DashboardURL: "http://127.0.0.1:4747", LocalOnly: true},
		Project{ID: "project-1", CanonicalPath: "/tmp/project", DisplayName: "project", Readiness: ProjectReadiness{Status: "MISSING_ROUTE", Reason: "configure a project route"}},
		Run{ID: "run-1", ProjectID: "project-1"},
		WorkspaceImport{ID: "import-1", SourceWorkspaceID: "legacy-1", DestinationWorkspaceID: "ws-1", Status: "COMPLETED", BackupReference: "/private/backup.db", CreatedAt: stamp, UpdatedAt: stamp},
		WorkspaceImportEvent{ID: "event-1", ImportID: "import-1", Sequence: 1, Status: "DETECTED", OccurredAt: stamp},
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "OPENAI_API_KEY") {
			t.Fatalf("unexpected credential in projection: %s", encoded)
		}
		var decoded map[string]any
		if err = json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded) == 0 {
			t.Fatalf("empty projection for %T", value)
		}
	}
}

func TestReleaseGateTaxonomyIsStableAndMissingEvidenceIsNotRun(t *testing.T) {
	gates := []Gate{{Key: "analyze_clean", Verdict: "PASS", Evidence: "analysis report"}, {Key: "required_tests", Verdict: "FAIL", Evidence: "test command exited 1"}}
	got := ReleaseGateTaxonomy(gates)
	if len(got) != 13 || got[0].ID != "QG-01" || got[0].Verdict != "NOT_RUN" || got[1].ID != "QG-02" || got[1].Verdict != "PASS" || got[6].ID != "QG-07" || got[6].Verdict != "FAIL" || got[12].ID != "QG-13" || got[12].Verdict != "NOT_RUN" {
		t.Fatalf("release gate projection=%+v", got)
	}
}

func TestSummarizeAcceptanceSeparatesLiveAndSimulationEvidence(t *testing.T) {
	liveAndFake := []RoutingDecision{{Stage: "analyze", Provider: "codex"}, {Stage: "implement", Provider: "fake"}}
	gates := []Gate{{Key: "analyze_clean", Verdict: "PASS"}, {Key: "required_tests", Verdict: "PASS"}, {Key: "converged", Verdict: "PASS"}, {Key: "final_review", Verdict: "PASS"}, {Key: "workflow_execution", Verdict: "PASS"}}
	summary := SummarizeAcceptance(liveAndFake, gates)
	if summary.Verdict != "PASS" || summary.Execution != "MIXED" || len(summary.RequiredGates) != 5 || len(summary.SimulationGates) != 1 || summary.SimulationGates[0].Verdict != "NOT_RUN" {
		t.Fatalf("acceptance=%+v", summary)
	}
	gates[2].Verdict = "SIMULATED"
	summary = SummarizeAcceptance(liveAndFake, gates)
	if summary.Verdict != "BLOCKED" {
		t.Fatalf("simulated evidence satisfied a live gate: %+v", summary)
	}
	gates[2].Verdict = "FAIL"
	summary = SummarizeAcceptance(liveAndFake, gates)
	if summary.Verdict != "FAIL" {
		t.Fatalf("failed gate verdict=%+v", summary)
	}
}

func TestSimulatedAcceptanceIsExplicitlyLabeled(t *testing.T) {
	decisions := []RoutingDecision{{Stage: "analyze", Provider: "fake"}}
	gates := []Gate{{Key: "analyze_clean", Verdict: "PASS"}, {Key: "required_tests", Verdict: "PASS"}, {Key: "converged", Verdict: "PASS"}, {Key: "final_review", Verdict: "PASS"}, {Key: "simulated_e2e", Verdict: "PASS"}, {Key: "workflow_execution", Verdict: "PASS"}}
	if got := SummarizeAcceptance(decisions, gates); got.Verdict != "SIMULATED" || got.Execution != "SIMULATED" {
		t.Fatalf("acceptance=%+v", got)
	}
	gates = append(gates[:4], gates[5:]...)
	if got := SummarizeAcceptance(decisions, gates); got.Verdict != "BLOCKED" {
		t.Fatalf("missing simulation gate=%+v", got)
	}
}

func TestSummarizeAcceptanceRequiresSuccessfulTerminalOutcomeAndAllRequiredGates(t *testing.T) {
	decisions := []RoutingDecision{{Stage: "analyze", Provider: "codex"}}
	base := []Gate{{Key: "analyze_clean", Verdict: "PASS"}, {Key: "required_tests", Verdict: "PASS"}, {Key: "converged", Verdict: "PASS"}, {Key: "final_review", Verdict: "PASS"}, {Key: "workflow_execution", Verdict: "PASS", Required: true, Applicable: true, PolicySet: true}}
	if got := SummarizeAcceptance(decisions, append(append([]Gate(nil), base...), Gate{Key: "required_security", Verdict: "BLOCKED", Required: true, Applicable: true, PolicySet: true})); got.Verdict != "BLOCKED" {
		t.Fatalf("additional required gate did not block acceptance: %+v", got)
	}
	if got := SummarizeAcceptance(decisions, append(append([]Gate(nil), base...), Gate{Key: "optional_report", Verdict: "NOT_RUN", Required: false, Applicable: true, PolicySet: true})); got.Verdict != "PASS" {
		t.Fatalf("non-required applicable gate blocked acceptance: %+v", got)
	}
	for _, terminal := range []struct{ verdict, want string }{{"BLOCKED", "BLOCKED"}, {"FAIL", "FAIL"}, {"NOT_RUN", "BLOCKED"}} {
		gates := append([]Gate(nil), base...)
		gates[4].Verdict = terminal.verdict
		if got := SummarizeAcceptance(decisions, gates); got.Verdict != terminal.want {
			t.Errorf("workflow terminal %s yielded %s, want %s", terminal.verdict, got.Verdict, terminal.want)
		}
	}
}
