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
