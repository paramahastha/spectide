package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spectide/spectide/internal/model"
)

func TestRequireExplicitNoopRequiresOneStandaloneReason(t *testing.T) {
	if reason, err := requireExplicitNoop("SPECTIDE_NO_WORK: existing implementation already satisfies the selected task", "SPECTIDE_NO_WORK"); err != nil || reason == "" {
		t.Fatalf("valid explicit no-op reason=%q err=%v", reason, err)
	}
	for _, output := range []string{
		"",
		"SPECTIDE_NO_WORK:",
		"`SPECTIDE_NO_WORK: already done`",
		"```text\nSPECTIDE_NO_WORK: already done\n```",
		"SPECTIDE_NO_WORK: existing implementation already satisfies the selected task\nSPECTIDE_NO_WORK: duplicate claim",
	} {
		if _, err := requireExplicitNoop(output, "SPECTIDE_NO_WORK"); err == nil {
			t.Errorf("accepted ambiguous no-op output %q", output)
		}
	}
}

func TestValidateOwnerArtifactSemantics(t *testing.T) {
	feature := t.TempDir()
	checklistPath := filepath.Join(feature, "checklists", "requirements.md")
	if err := os.MkdirAll(filepath.Dir(checklistPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checklistPath, []byte("arbitrary changed bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateChangedOwnerArtifact("checklist", feature, []model.Artifact{{Path: "specs/001/checklists/requirements.md"}}); err == nil {
		t.Fatal("checklist without requirement rows passed semantic validation")
	}
	if err := os.WriteFile(checklistPath, []byte("# Requirements Checklist\n\n- [ ] Authentication boundary is validated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateChangedOwnerArtifact("checklist", feature, []model.Artifact{{Path: "specs/001/checklists/requirements.md"}}); err != nil {
		t.Fatalf("valid checklist rejected: %v", err)
	}
	ledger := filepath.Join(feature, "tasks.md")
	if err := os.WriteFile(ledger, []byte("# Tasks\n\n- [ ] finish all work\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateChangedOwnerArtifact("tasks", feature, []model.Artifact{{Path: "specs/001/tasks.md"}}); err == nil {
		t.Fatal("task ledger without stable task IDs passed semantic validation")
	}
	if err := os.WriteFile(ledger, []byte("# Tasks\n\n- [ ] T091 Bind the selected feature scope\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateChangedOwnerArtifact("tasks", feature, []model.Artifact{{Path: "specs/001/tasks.md"}}); err != nil {
		t.Fatalf("valid task ledger rejected: %v", err)
	}
}

func TestExplicitNoopRequiresInspectableExistingEvidence(t *testing.T) {
	root := t.TempDir()
	feature := filepath.Join(root, "specs", "001-selected")
	if err := os.MkdirAll(feature, 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateExistingSpecification(feature); err == nil {
		t.Fatal("no-op clarification accepted with no existing specification")
	}
	if err := os.WriteFile(filepath.Join(feature, "spec.md"), []byte("# Product\n\n## User scenarios\n\nA developer can safely resume the selected workflow after an interrupted process.\n\n## Requirements\n\n- Users can resume a run.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateExistingSpecification(feature); err != nil {
		t.Fatalf("inspectable existing specification rejected: %v", err)
	}
	if err := validateExistingImplementation(root); err == nil {
		t.Fatal("no-op implementation accepted without existing source")
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateExistingImplementation(root); err != nil {
		t.Fatalf("existing source implementation rejected: %v", err)
	}
}
