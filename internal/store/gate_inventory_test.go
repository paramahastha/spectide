package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
)

func TestTerminalReportsPreserveRequiredGateMatrix(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	makeRun := func(name string, live bool) model.Run {
		t.Helper()
		run, err := s.CreateRun(ctx, t.TempDir(), name)
		if err != nil {
			t.Fatal(err)
		}
		if live {
			if err = s.SaveRoutingDecision(ctx, run.ID, "analyze", "codex", "fixture", "", "", "hybrid", "live fixture", false); err != nil {
				t.Fatal(err)
			}
		}
		return run
	}
	gateMap := func(runID string) map[string]string {
		t.Helper()
		gates, err := s.Gates(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		got := make(map[string]string, len(gates))
		for _, gate := range gates {
			got[gate.Key] = gate.Verdict
		}
		return got
	}
	assertInventory := func(runID string, verdicts map[string]string) {
		t.Helper()
		got := gateMap(runID)
		if len(got) != len(requiredGateInventory) {
			t.Fatalf("gate inventory has %d keys: %v", len(got), got)
		}
		for _, key := range requiredGateInventory {
			if verdict, ok := verdicts[key]; !ok || got[key] != verdict {
				t.Fatalf("gate %s=%q, want %q; all=%v", key, got[key], verdict, got)
			}
		}
	}

	missing := makeRun("missing integration", true)
	assertInventory(missing.ID, map[string]string{"analyze_clean": "NOT_RUN", "required_tests": "NOT_RUN", "converged": "NOT_RUN", "final_review": "NOT_RUN", "simulated_e2e": "NOT_RUN", "workflow_execution": "NOT_RUN"})
	if report, err := s.Report(ctx, missing.ID); err != nil || report.Acceptance.Execution != "LIVE" || report.Acceptance.Verdict != "BLOCKED" || len(report.Acceptance.RequiredGates) != 4 {
		t.Fatalf("missing integration report=%+v err=%v", report.Acceptance, err)
	}
	if err = s.SetRunState(ctx, missing.ID, model.Blocked); err != nil {
		t.Fatal(err)
	}
	assertInventory(missing.ID, map[string]string{"analyze_clean": "NOT_RUN", "required_tests": "NOT_RUN", "converged": "NOT_RUN", "final_review": "NOT_RUN", "simulated_e2e": "NOT_RUN", "workflow_execution": "BLOCKED"})

	failure := makeRun("early provider failure", true)
	if err = s.Gate(ctx, failure.ID, "analyze_clean", "FAIL", "provider returned invalid analysis"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, failure.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, failure.ID, model.Failed); err != nil {
		t.Fatal(err)
	}
	assertInventory(failure.ID, map[string]string{"analyze_clean": "FAIL", "required_tests": "NOT_RUN", "converged": "NOT_RUN", "final_review": "NOT_RUN", "simulated_e2e": "NOT_RUN", "workflow_execution": "FAIL"})
	if report, err := s.Report(ctx, failure.ID); err != nil || report.Acceptance.Execution != "LIVE" || report.Acceptance.Verdict != "FAIL" {
		t.Fatalf("failed provider report=%+v err=%v", report.Acceptance, err)
	}

	cancelled := makeRun("cancelled", true)
	if err = s.SetRunState(ctx, cancelled.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, cancelled.ID, model.Cancelled); err != nil {
		t.Fatal(err)
	}
	assertInventory(cancelled.ID, map[string]string{"analyze_clean": "NOT_RUN", "required_tests": "NOT_RUN", "converged": "NOT_RUN", "final_review": "NOT_RUN", "simulated_e2e": "NOT_RUN", "workflow_execution": "BLOCKED"})

	interrupted := makeRun("interrupted", true)
	if err = s.AcquireWorkspaceLockOwned(ctx, interrupted.Project, interrupted.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, interrupted.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, interrupted.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assertInventory(interrupted.ID, map[string]string{"analyze_clean": "NOT_RUN", "required_tests": "NOT_RUN", "converged": "NOT_RUN", "final_review": "NOT_RUN", "simulated_e2e": "NOT_RUN", "workflow_execution": "BLOCKED"})
	if report, err := s.Report(ctx, interrupted.ID); err != nil || report.Acceptance.Execution != "LIVE" || report.Acceptance.Verdict != "BLOCKED" {
		t.Fatalf("interrupted report=%+v err=%v", report.Acceptance, err)
	}

	simulated := makeRun("successful simulation", false)
	if err = s.SetRunState(ctx, simulated.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"analyze_clean", "required_tests", "converged", "final_review", "simulated_e2e"} {
		if err = s.Gate(ctx, simulated.ID, key, "PASS", "fixture evidence"); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SetRunState(ctx, simulated.ID, model.Completed); err != nil {
		t.Fatal(err)
	}
	assertInventory(simulated.ID, map[string]string{"analyze_clean": "PASS", "required_tests": "PASS", "converged": "PASS", "final_review": "PASS", "simulated_e2e": "PASS", "workflow_execution": "PASS"})
	if report, err := s.Report(ctx, simulated.ID); err != nil || report.Acceptance.Execution != "SIMULATED" || report.Acceptance.Verdict != "SIMULATED" {
		t.Fatalf("simulation report=%+v err=%v", report.Acceptance, err)
	}

	live := makeRun("successful live fixture", true)
	if err = s.SetRunState(ctx, live.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"analyze_clean", "required_tests", "converged", "final_review"} {
		if err = s.Gate(ctx, live.ID, key, "PASS", "fixture evidence"); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SetRunState(ctx, live.ID, model.Completed); err != nil {
		t.Fatal(err)
	}
	assertInventory(live.ID, map[string]string{"analyze_clean": "PASS", "required_tests": "PASS", "converged": "PASS", "final_review": "PASS", "simulated_e2e": "NOT_RUN", "workflow_execution": "PASS"})
	if report, err := s.Report(ctx, live.ID); err != nil || report.Acceptance.Execution != "LIVE" || report.Acceptance.Verdict != "PASS" || len(report.Acceptance.SimulationGates) != 1 || report.Acceptance.SimulationGates[0].Verdict != "NOT_RUN" {
		t.Fatalf("live report=%+v err=%v", report.Acceptance, err)
	}
}
