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
