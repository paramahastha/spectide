package model

import "testing"

func TestSummarizeAcceptanceSeparatesLiveAndSimulationEvidence(t *testing.T) {
	liveAndFake := []RoutingDecision{{Stage: "analyze", Provider: "codex"}, {Stage: "implement", Provider: "fake"}}
	gates := []Gate{{Key: "analyze_clean", Verdict: "PASS"}, {Key: "required_tests", Verdict: "PASS"}, {Key: "converged", Verdict: "PASS"}, {Key: "final_review", Verdict: "PASS"}}
	summary := SummarizeAcceptance(liveAndFake, gates)
	if summary.Verdict != "PASS" || summary.Execution != "MIXED" || len(summary.RequiredGates) != 4 || len(summary.SimulationGates) != 1 || summary.SimulationGates[0].Verdict != "NOT_RUN" {
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
	gates := []Gate{{Key: "analyze_clean", Verdict: "PASS"}, {Key: "required_tests", Verdict: "PASS"}, {Key: "converged", Verdict: "PASS"}, {Key: "final_review", Verdict: "PASS"}, {Key: "simulated_e2e", Verdict: "PASS"}}
	if got := SummarizeAcceptance(decisions, gates); got.Verdict != "SIMULATED" || got.Execution != "SIMULATED" {
		t.Fatalf("acceptance=%+v", got)
	}
	gates = gates[:len(gates)-1]
	if got := SummarizeAcceptance(decisions, gates); got.Verdict != "BLOCKED" {
		t.Fatalf("missing simulation gate=%+v", got)
	}
}
