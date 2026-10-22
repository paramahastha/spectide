package model

import "testing"

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
