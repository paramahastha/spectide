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
	}
	for _, stage := range []string{"plan", "tasks"} {
		if path, present, err := existingOwnerArtifact(root, feature, stage); err != nil || present {
			t.Fatalf("absent %s owner artifact=(%q,%v,%v)", stage, path, present, err)
		}
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
