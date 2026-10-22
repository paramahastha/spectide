package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/model"
)

func evidenceStore(t *testing.T) (*Store, model.Run) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(context.Background(), "demo", "test evidence")
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, run
}

func TestRunConfigurationAndStageAttemptsAreImmutable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	config := []byte(`[{"stage":"analyze","provider":"fake","profile":"offline","simulated":true}]`)
	run, reused, err := s.CreateRunWithConfiguration(ctx, "run-key-configuration", "/workspace", "inspect", config)
	if err != nil || reused {
		t.Fatalf("run=%+v reused=%v err=%v", run, reused, err)
	}
	stored, err := s.RunConfiguration(ctx, run.ID)
	if err != nil || stored.ConfigJSON != string(config) || len(stored.SHA256) != 64 {
		t.Fatalf("configuration=%+v err=%v", stored, err)
	}
	if _, err = s.DB.Exec(`UPDATE run_configurations SET config_json='{}' WHERE run_id=?`, run.ID); err == nil {
		t.Fatal("database allowed effective run configuration mutation")
	}
	if _, reused, err = s.CreateRunWithConfiguration(ctx, "run-key-configuration", "/workspace", "inspect", config); err != nil || !reused {
		t.Fatalf("same configuration was not idempotent: reused=%v err=%v", reused, err)
	}
	changedConfig := []byte(`[{"stage":"analyze","provider":"codex","profile":"live","simulated":false}]`)
	if _, _, err = s.CreateRunWithConfiguration(ctx, "run-key-configuration", "/workspace", "inspect", changedConfig); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST") {
		t.Fatalf("changed effective configuration reused the key: %v", err)
	}

	first := model.Stage{ID: "attempt-1", RunID: run.ID, Key: "analyze", Position: 1, State: model.Pending, Provider: "fake", Profile: "offline", Attempt: 1}
	if err = s.AddStage(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := model.Stage{ID: "attempt-2", RunID: run.ID, Key: "analyze", Position: 1, State: model.Pending, Provider: "fake", Profile: "offline", Attempt: 2}
	if err = s.AddStage(ctx, second); err == nil || !strings.Contains(err.Error(), "STAGE_ATTEMPT_STILL_ACTIVE") {
		t.Fatalf("parallel active attempt accepted: %v", err)
	}
	if err = s.SetStage(ctx, first.ID, model.StageFail); err != nil {
		t.Fatal(err)
	}
	changedRoute := second
	changedRoute.Provider = "codex"
	if err = s.AddStage(ctx, changedRoute); err == nil || !strings.Contains(err.Error(), "STAGE_ATTEMPT_CONFIGURATION_CHANGED") {
		t.Fatalf("retry changed the route: %v", err)
	}
	if err = s.AddStage(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStage(ctx, second.ID, model.StageRun); err != nil {
		t.Fatalf("pending -> running failed: %v", err)
	}
	if err = s.SetStage(ctx, second.ID, model.Succeeded); err != nil {
		t.Fatalf("running -> succeeded failed: %v", err)
	}
	if err = s.SetStage(ctx, second.ID, model.StageFail); err == nil {
		t.Fatal("terminal running attempt was mutable")
	}
	if _, err = s.DB.Exec(`UPDATE stages SET profile='mutated' WHERE id=?`, second.ID); err == nil {
		t.Fatal("database allowed stage attempt mutation")
	}
	if err = s.SetStage(ctx, first.ID, model.Succeeded); err == nil {
		t.Fatal("terminal attempt was mutable")
	}
	stages, err := s.Stages(ctx, run.ID)
	if err != nil || len(stages) != 2 || stages[0].Attempt != 1 || stages[0].State != model.StageFail || stages[1].Attempt != 2 || stages[1].State != model.Succeeded {
		t.Fatalf("attempt history=%+v err=%v", stages, err)
	}
}

func TestIdempotencyHashesOriginalPromptButPersistsRedactedPrompt(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	firstPrompt := "inspect api_key=first-synthetic-secret-123456"
	first, reused, err := s.CreateRunIdempotent(ctx, "secret-request-key", "/workspace", firstPrompt)
	if err != nil || reused {
		t.Fatalf("first run=%+v reused=%v err=%v", first, reused, err)
	}
	if strings.Contains(first.Prompt, "first-synthetic-secret-123456") || !strings.Contains(first.Prompt, "[REDACTED]") {
		t.Fatalf("persisted prompt was not redacted: %q", first.Prompt)
	}
	if _, reused, err = s.CreateRunIdempotent(ctx, "secret-request-key", "/workspace", firstPrompt); err != nil || !reused {
		t.Fatalf("same raw request not idempotent: reused=%v err=%v", reused, err)
	}
	_, _, err = s.CreateRunIdempotent(ctx, "secret-request-key", "/workspace", "inspect api_key=second-synthetic-secret-654321")
	if err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST") {
		t.Fatalf("different secret-bearing prompt reused the request key: %v", err)
	}
}

func TestEvidenceAndCapabilityStringsAreRedactedBeforePersistence(t *testing.T) {
	ctx := context.Background()
	s, run := evidenceStore(t)
	if _, err := s.SaveProfile(ctx, model.Profile{Name: "unsafe-reference", Provider: "codex", Executable: "codex api_key=profile-secret-123456", AllowedScopes: []string{"/tmp"}}); err == nil || !strings.Contains(err.Error(), "PROFILE_SECRET_REFERENCE_REJECTED") {
		t.Fatalf("secret-bearing execution reference was persisted: %v", err)
	}
	gateSecret := "gate-synthetic-secret-123456"
	if err := s.Gate(ctx, run.ID, "required_tests", "FAIL", "api_key="+gateSecret); err != nil {
		t.Fatal(err)
	}
	approvalSecret := "approval-synthetic-secret-654321"
	approval, err := s.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: "token=" + approvalSecret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveProfile(ctx, model.Profile{Name: "safe-fixture", Provider: "codex", Executable: "codex", AllowedScopes: []string{"/tmp"}}); err != nil {
		t.Fatal(err)
	}
	capabilitySecret := "capability-synthetic-secret-abcdef"
	capability, err := s.SaveProfileCapability(ctx, "safe-fixture", model.ProfileCapability{Provider: "codex", Version: "api_key=" + capabilitySecret, Authenticated: "AUTHENTICATED", Warnings: []string{"password=" + capabilitySecret}})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{capability.Version, capability.Warnings[0], approval.Action} {
		if strings.Contains(value, gateSecret) || strings.Contains(value, approvalSecret) || strings.Contains(value, capabilitySecret) {
			t.Fatalf("returned evidence leaked a secret: %q", value)
		}
	}
	for table, column := range map[string]string{"quality_gates": "evidence", "approvals": "action"} {
		var persisted string
		query := `SELECT ` + column + ` FROM ` + table + ` WHERE run_id=?`
		if err = s.DB.QueryRowContext(ctx, query, run.ID).Scan(&persisted); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{gateSecret, approvalSecret, capabilitySecret} {
			if strings.Contains(persisted, secret) {
				t.Fatalf("%s.%s leaked %q", table, column, secret)
			}
		}
	}
	var storedCapability string
	if err = s.DB.QueryRowContext(ctx, `SELECT capability_json FROM profile_capabilities WHERE profile_name=?`, "safe-fixture").Scan(&storedCapability); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedCapability, capabilitySecret) {
		t.Fatalf("capability metadata leaked: %s", storedCapability)
	}
	if _, _, err = s.CreateRunWithConfiguration(ctx, "", "/workspace", "safe prompt", []byte(`[{"model":"api_key=configuration-secret-123456"}]`)); err == nil || !strings.Contains(err.Error(), "RUN_CONFIGURATION_SECRET_REFERENCE_REJECTED") {
		t.Fatalf("secret-bearing execution identity was persisted: %v", err)
	}
}

func TestStateMutationsRollbackWhenTheirOrderedEventCannotBeWritten(t *testing.T) {
	ctx := context.Background()
	s, run := evidenceStore(t)
	rejectEvent := func(eventType string) func() {
		t.Helper()
		name := "reject_" + strings.NewReplacer(".", "_", "-", "_").Replace(eventType)
		if _, err := s.DB.Exec(`CREATE TRIGGER ` + name + ` BEFORE INSERT ON events WHEN NEW.type='` + eventType + `' BEGIN SELECT RAISE(ABORT,'injected event failure'); END`); err != nil {
			t.Fatal(err)
		}
		return func() {
			t.Helper()
			if _, err := s.DB.Exec(`DROP TRIGGER ` + name); err != nil {
				t.Fatal(err)
			}
		}
	}
	stage := model.Stage{ID: "stage-atomic", RunID: run.ID, Key: "analyze", Position: 0, State: model.Pending, Provider: "fake", Attempt: 1}
	remove := rejectEvent("stage.created")
	if err := s.AddStage(ctx, stage); err == nil {
		t.Fatal("stage creation committed without its event")
	}
	remove()
	stages, err := s.Stages(ctx, run.ID)
	if err != nil || len(stages) != 0 {
		t.Fatalf("stage creation was not rolled back: stages=%+v err=%v", stages, err)
	}
	if err = s.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}

	remove = rejectEvent("stage.transition")
	if err := s.SetStage(ctx, stage.ID, model.StageRun); err == nil {
		t.Fatal("stage transition committed without its event")
	}
	remove()
	stages, err = s.Stages(ctx, run.ID)
	if err != nil || len(stages) != 1 || stages[0].State != model.Pending {
		t.Fatalf("stage mutation was not rolled back: stages=%+v err=%v", stages, err)
	}

	remove = rejectEvent("quality.gate_recorded")
	if err = s.Gate(ctx, run.ID, "required_tests", "PASS", "all checks passed"); err == nil {
		t.Fatal("quality gate committed without its event")
	}
	remove()
	gates, err := s.Gates(ctx, run.ID)
	if err != nil || len(gates) != 0 {
		t.Fatalf("quality gate mutation was not rolled back: gates=%+v err=%v", gates, err)
	}

	remove = rejectEvent("routing.resolved")
	if err = s.SaveRoutingDecision(ctx, run.ID, "analyze", "fake", "offline", "fixture", "", "explicit", "fixture route"); err == nil {
		t.Fatal("routing decision committed without its event")
	}
	remove()
	decisions, err := s.RoutingDecisions(ctx, run.ID)
	if err != nil || len(decisions) != 0 {
		t.Fatalf("routing decision mutation was not rolled back: decisions=%+v err=%v", decisions, err)
	}

	remove = rejectEvent("approval.created")
	if _, err = s.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: "review"}); err == nil {
		t.Fatal("approval creation committed without its event")
	}
	remove()
	approvals, err := s.Approvals(ctx, run.ID)
	if err != nil || len(approvals) != 0 {
		t.Fatalf("approval creation was not rolled back: approvals=%+v err=%v", approvals, err)
	}
	approval, err := s.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: "review"})
	if err != nil {
		t.Fatal(err)
	}
	remove = rejectEvent("approval.resolved")
	if err = s.ResolveApproval(ctx, approval.ID, model.ApprovalApproved); err == nil {
		t.Fatal("approval resolution committed without its event")
	}
	remove()
	approvals, err = s.Approvals(ctx, run.ID)
	if err != nil || len(approvals) != 1 || approvals[0].Status != model.ApprovalPending {
		t.Fatalf("approval resolution was not rolled back: approvals=%+v err=%v", approvals, err)
	}

	if err = s.SetStage(ctx, stage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	if err = s.Gate(ctx, run.ID, "required_tests", "PASS", "all checks passed"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveRoutingDecision(ctx, run.ID, "analyze", "fake", "offline", "fixture", "", "explicit", "fixture route"); err != nil {
		t.Fatal(err)
	}
	if err = s.ResolveApproval(ctx, approval.ID, model.ApprovalDenied); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for index, event := range events {
		if event.Seq != int64(index+1) {
			t.Fatalf("event sequence is not contiguous: index=%d event=%+v", index, event)
		}
	}
}

func TestArtifactCheckpointAndApprovalRepositories(t *testing.T) {
	ctx := context.Background()
	s, run := evidenceStore(t)
	digest := sha256.Sum256([]byte("spec artifact"))
	artifact, err := s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "specify", Path: "specs/001/spec.md", Kind: "markdown", SHA256: hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := s.Artifacts(ctx, run.ID)
	if err != nil || len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("artifacts=%+v err=%v", artifacts, err)
	}
	if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "specify", Path: "../secret", Kind: "markdown", SHA256: hex.EncodeToString(digest[:])}); err == nil {
		t.Fatal("artifact path escape accepted")
	}

	checkpoint, err := s.SaveCheckpoint(ctx, model.Checkpoint{RunID: run.ID, Stage: "plan", WorkspaceFingerprint: "tree-sha256", StateJSON: `{"state":"safe","nested":{"token":"supersecret123456"}}`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(checkpoint.StateJSON, "supersecret") {
		t.Fatal("checkpoint retained nested secret")
	}
	checkpoints, err := s.Checkpoints(ctx, run.ID)
	if err != nil || len(checkpoints) != 1 || checkpoints[0].ID != checkpoint.ID {
		t.Fatalf("checkpoints=%+v err=%v", checkpoints, err)
	}
	if _, err = s.SaveCheckpoint(ctx, model.Checkpoint{RunID: run.ID, Stage: "plan", WorkspaceFingerprint: "x", StateJSON: "{"}); err == nil {
		t.Fatal("invalid checkpoint JSON accepted")
	}

	approval, err := s.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: "write workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResolveApproval(ctx, approval.ID, "APPROVED"); err != nil {
		t.Fatal(err)
	}
	if err = s.ResolveApproval(ctx, approval.ID, "DENIED"); err == nil {
		t.Fatal("resolved approval was mutable")
	}
	approvals, err := s.Approvals(ctx, run.ID)
	if err != nil || len(approvals) != 1 || approvals[0].Status != "APPROVED" || approvals[0].ResolvedAt == nil {
		t.Fatalf("approvals=%+v err=%v", approvals, err)
	}
	report, err := s.Report(ctx, run.ID)
	if err != nil || len(report.Artifacts) != 1 || len(report.Checkpoints) != 1 || len(report.Approvals) != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}
