package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingOwnerArtifactPreservesConstitutionAndPartialProductSpec(t *testing.T) {
	root := t.TempDir()
	feature := filepath.Join(root, "specs", "001-product")
	if err := os.MkdirAll(feature, 0o700); err != nil {
		t.Fatal(err)
	}
	constitution := filepath.Join(root, ".specify", "memory", "constitution.md")
	if err := os.MkdirAll(filepath.Dir(constitution), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(constitution, []byte("existing constitution"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(feature, "spec.md"), []byte("existing product specification"), 0o600); err != nil {
		t.Fatal(err)
	}
	for stage, expectedPath := range map[string]string{
		"constitution": ".specify/memory/constitution.md",
		"specify":      "specs/001-product/spec.md",
	} {
		path, present, err := existingOwnerArtifact(root, feature, stage)
		if err != nil || !present || path != expectedPath {
			t.Fatalf("stage %s existing owner artifact=(%q,%v,%v), expected %q", stage, path, present, err, expectedPath)
		}
		if required, checkErr := stageCommandRequired(root, feature, stage); checkErr != nil || required {
			t.Fatalf("preserved stage %s command required=%v err=%v", stage, required, checkErr)
		}
	}
	for _, stage := range []string{"plan", "tasks"} {
		if path, present, err := existingOwnerArtifact(root, feature, stage); err != nil || present {
			t.Fatalf("absent %s owner artifact=(%q,%v,%v)", stage, path, present, err)
		}
		if required, checkErr := stageCommandRequired(root, feature, stage); checkErr != nil || !required {
			t.Fatalf("missing stage %s command required=%v err=%v", stage, required, checkErr)
		}
	}
}

func TestSpecifyAmendmentIntentRunsDespiteExistingSpecification(t *testing.T) {
	root := t.TempDir()
	feature := filepath.Join(root, "specs", "001-product")
	if err := os.MkdirAll(feature, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(feature, "spec.md"), []byte("# Existing specification\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if required, err := stageCommandRequired(root, feature, "specify", specificationIntentContinue); err != nil || required {
		t.Fatalf("continuation required=%v err=%v, want preserved", required, err)
	}
	if required, err := stageCommandRequired(root, feature, "specify", specificationIntentAmend); err != nil || !required {
		t.Fatalf("amendment required=%v err=%v, want invocation", required, err)
	}
}

func TestSpecificationIntentSeparatesNextFeatureFromAmendment(t *testing.T) {
	for _, prompt := range []string{"Implement spec based on this plan for next phase", "Start a new phase for the selected feature"} {
		if got := specificationIntentForPrompt(prompt); got != specificationIntentNext {
			t.Errorf("specificationIntentForPrompt(%q)=%q, want %q", prompt, got, specificationIntentNext)
		}
	}
	if got := specificationIntentForPrompt("Amend specification for the selected feature"); got != specificationIntentAmend {
		t.Fatalf("explicit amendment intent=%q, want %q", got, specificationIntentAmend)
	}
	if got := specificationIntentForPrompt("Continue implementation against the accepted specification"); got != specificationIntentContinue {
		t.Fatalf("continuation intent=%q, want %q", got, specificationIntentContinue)
	}
}

func TestExistingOwnerArtifactRejectsConstitutionSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	feature := filepath.Join(root, "specs", "001-product")
	if err := os.MkdirAll(feature, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".specify", "memory"), 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "constitution.md")
	if err := os.WriteFile(secret, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, ".specify", "memory", "constitution.md")); err != nil {
		t.Skip(err)
	}
	if _, _, err := existingOwnerArtifact(root, feature, "constitution"); err == nil {
		t.Fatal("constitution symlink escape was accepted")
	}
}
