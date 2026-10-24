package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
)

func TestRunListExecutionLabelsComeFromImmutableRoutes(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	type route struct {
		Provider string `json:"provider"`
	}
	type stage struct {
		Decision struct {
			Route route `json:"route"`
		} `json:"decision"`
	}
	cases := []struct {
		name      string
		providers []string
		want      string
	}{
		{name: "simulated", providers: []string{"fake", "fake"}, want: "SIMULATED"},
		{name: "live", providers: []string{"codex", "claude"}, want: "LIVE"},
		{name: "mixed", providers: []string{"fake", "codex"}, want: "MIXED"},
	}
	wantByID := map[string]string{}
	for _, tc := range cases {
		var stages []stage
		for _, provider := range tc.providers {
			var item stage
			item.Decision.Route.Provider = provider
			stages = append(stages, item)
		}
		encoded, marshalErr := json.Marshal(struct {
			Stages []stage `json:"stages"`
		}{Stages: stages})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		run, _, createErr := s.CreateRunWithConfiguration(ctx, "", "/workspace/"+tc.name, tc.name, encoded)
		if createErr != nil {
			t.Fatal(createErr)
		}
		wantByID[run.ID] = tc.want
		got, getErr := s.GetRun(ctx, run.ID)
		if getErr != nil || got.Execution != tc.want {
			t.Fatalf("GetRun execution=%q want %q, err=%v", got.Execution, tc.want, getErr)
		}
	}
	legacy, err := s.CreateRun(ctx, "/workspace/legacy", "legacy route evidence")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveRoutingDecision(ctx, legacy.ID, "analyze", "codex", "legacy-live", "", "", "manual", "stored route", false); err != nil {
		t.Fatal(err)
	}
	if err = s.AddStage(ctx, model.Stage{ID: "legacy-fake-stage", RunID: legacy.ID, Key: "implement", Position: 2, State: model.Pending, Provider: "fake", Profile: "offline", Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	wantByID[legacy.ID] = "MIXED"
	runs, err := s.Runs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if want := wantByID[run.ID]; run.Execution != want {
			t.Errorf("run %s execution=%q want %q", run.ID, run.Execution, want)
		}
	}
}

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
			gates, gateErr := s.Gates(ctx, run.ID)
			if gateErr != nil {
				t.Fatal(gateErr)
			}
			for _, gate := range gates {
				if gate.Key == "simulated_e2e" && (gate.Required || gate.Applicable) {
					t.Fatalf("simulation-only gate policy remained required/applicable: %+v", gate)
				}
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
		if len(got) < len(requiredGateInventory) {
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
	if report, err := s.Report(ctx, missing.ID); err != nil || report.Acceptance.Execution != "LIVE" || report.Acceptance.Verdict != "BLOCKED" || len(report.Acceptance.RequiredGates) != 5 {
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
	lateCancelled := makeRun("late cancellation after gates", true)
	if err = s.SetRunState(ctx, lateCancelled.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"analyze_clean", "required_tests", "converged", "final_review"} {
		if err = s.Gate(ctx, lateCancelled.ID, key, "PASS", "gate passed before cancellation"); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SetRunState(ctx, lateCancelled.ID, model.Cancelled); err != nil {
		t.Fatal(err)
	}
	if report, reportErr := s.Report(ctx, lateCancelled.ID); reportErr != nil || report.Acceptance.Verdict != "BLOCKED" || gateByName(report.Acceptance.RequiredGates, "workflow_execution").Verdict != "BLOCKED" {
		t.Fatalf("late cancellation retained acceptance pass: %+v err=%v", report.Acceptance, reportErr)
	}
	lateFailed := makeRun("late failure after gates", true)
	if err = s.SetRunState(ctx, lateFailed.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"analyze_clean", "required_tests", "converged", "final_review"} {
		if err = s.Gate(ctx, lateFailed.ID, key, "PASS", "gate passed before failure"); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SetRunState(ctx, lateFailed.ID, model.Failed); err != nil {
		t.Fatal(err)
	}
	if report, reportErr := s.Report(ctx, lateFailed.ID); reportErr != nil || report.Acceptance.Verdict != "FAIL" {
		t.Fatalf("late failure retained acceptance pass: %+v err=%v", report.Acceptance, reportErr)
	}

	interrupted := makeRun("interrupted", true)
	if err = s.AcquireWorkspaceLockOwned(ctx, interrupted.Project, interrupted.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, interrupted.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	activeStage := model.Stage{ID: "abandoned-active-stage", RunID: interrupted.ID, Key: "implement", Position: 1, State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
	if err = s.AddStage(ctx, activeStage); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStage(ctx, activeStage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	_, err = s.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: interrupted.ID, Stage: "implement", Phase: "implement", Provider: "codex", Profile: "fixture"})
	if err != nil {
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
	stages, err := s.Stages(ctx, interrupted.ID)
	if err != nil || len(stages) != 1 || stages[0].State != model.StageFail {
		t.Fatalf("interrupted stages=%+v err=%v", stages, err)
	}
	attempts, err := s.ExecutionAttempts(ctx, interrupted.ID)
	if err != nil || len(attempts) != 1 || attempts[0].Result != "FAILED" || attempts[0].FinishedAt == nil {
		t.Fatalf("interrupted attempts=%+v err=%v", attempts, err)
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
	if err = s.Gate(ctx, live.ID, "required_security_review", "BLOCKED", "required supplementary gate is incomplete"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, live.ID, model.Completed); err == nil || !strings.Contains(err.Error(), "RUN_COMPLETION_REQUIRED_GATES_UNSATISFIED") {
		t.Fatalf("run completed with an additional required blocked gate: %v", err)
	}
	if report, reportErr := s.Report(ctx, live.ID); reportErr != nil || report.Run.State != model.Running || report.Acceptance.Verdict != "BLOCKED" {
		t.Fatalf("blocked required gate report=%+v err=%v", report.Acceptance, reportErr)
	}
	if err = s.Gate(ctx, live.ID, "required_security_review", "PASS", "required supplementary gate passed"); err != nil {
		t.Fatal(err)
	}
	if err = s.GateWithPolicy(ctx, live.ID, "optional_telemetry", "NOT_RUN", "optional gate was not selected", false, true); err != nil {
		t.Fatal(err)
	}
	if err = s.GateWithPolicy(ctx, live.ID, "platform_specific", "NOT_RUN", "platform gate does not apply to this project", true, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, live.ID, model.Completed); err != nil {
		t.Fatal(err)
	}
	assertInventory(live.ID, map[string]string{"analyze_clean": "PASS", "required_tests": "PASS", "converged": "PASS", "final_review": "PASS", "simulated_e2e": "NOT_RUN", "workflow_execution": "PASS"})
	if report, err := s.Report(ctx, live.ID); err != nil || report.Acceptance.Execution != "LIVE" || report.Acceptance.Verdict != "PASS" || len(report.Acceptance.SimulationGates) != 1 || report.Acceptance.SimulationGates[0].Verdict != "NOT_RUN" || gateByName(report.Acceptance.RequiredGates, "workflow_execution").Verdict != "PASS" || gateByName(report.Acceptance.RequiredGates, "optional_telemetry").Key != "" || gateByName(report.Acceptance.RequiredGates, "platform_specific").Key != "" {
		t.Fatalf("live report=%+v err=%v", report.Acceptance, err)
	}
}

func gateByName(gates []model.Gate, key string) model.Gate {
	for _, gate := range gates {
		if gate.Key == key {
			return gate
		}
	}
	return model.Gate{}
}
