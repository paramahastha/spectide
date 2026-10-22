package speckit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectsUninitializedProject(t *testing.T) {
	if _, e := Discover(t.TempDir()); e == nil {
		t.Fatal("expected error")
	}
}

func TestStagePromptExpandsTrustedTemplateAndArguments(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".opencode", "commands")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(".opencode", "commands", "speckit.analyze.md")
	if err := os.WriteFile(filepath.Join(root, rel), []byte("---\ndescription: analyze\n---\nReview read-only. Input: $ARGUMENTS"), 0600); err != nil {
		t.Fatal(err)
	}
	in := Installation{ProjectRoot: root, Commands: map[string]string{"analyze": "/speckit.analyze"}, CommandFiles: map[string]string{"analyze": rel}}
	prompt, err := StagePrompt(in, "analyze", "task details")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Review read-only.") || !strings.Contains(prompt, "Input: task details") || strings.Contains(prompt, "description: analyze") {
		t.Fatalf("prompt=%q", prompt)
	}
}

func TestFeatureScopeArgumentsPinsCanonicalFeatureDirectory(t *testing.T) {
	root := t.TempDir()
	feature := filepath.Join(root, "specs", "001-product")
	if err := os.MkdirAll(feature, 0700); err != nil {
		t.Fatal(err)
	}
	arguments, err := FeatureScopeArguments(root, feature, "implement the selected task")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"implement the selected task", "Selected canonical feature directory: specs/001-product", "specs/001-product/spec.md", "do not create another feature directory"} {
		if !strings.Contains(arguments, expected) {
			t.Fatalf("scope arguments missing %q: %s", expected, arguments)
		}
	}
}

func TestFeatureScopeArgumentsRejectsOtherAndNestedDirectories(t *testing.T) {
	root := t.TempDir()
	for _, feature := range []string{filepath.Join(root, "outside"), filepath.Join(root, "specs", "001-product", "nested")} {
		if err := os.MkdirAll(feature, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := FeatureScopeArguments(root, feature, "task"); err == nil {
			t.Fatalf("accepted non-canonical feature scope %q", feature)
		}
	}
}

func TestValidateUniqueFeatureDirectoryRejectsAlternativeSpecification(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "specs", "001-product")
	if err := os.MkdirAll(selected, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, "spec.md"), []byte("selected\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUniqueFeatureDirectory(root, selected); err != nil {
		t.Fatalf("selected scope rejected: %v", err)
	}
	alternate := filepath.Join(root, "specs", "002-other")
	if err := os.MkdirAll(alternate, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alternate, "spec.md"), []byte("alternate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUniqueFeatureDirectory(root, selected); err == nil {
		t.Fatal("alternate product specification was accepted")
	}
}

func TestStagePromptRejectsTemplateSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "template.md")
	if err := os.WriteFile(outside, []byte("unsafe"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".opencode", "commands")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(".opencode", "commands", "speckit.tasks.md")
	if err := os.Symlink(outside, filepath.Join(root, rel)); err != nil {
		t.Skip(err)
	}
	in := Installation{ProjectRoot: root, Commands: map[string]string{"tasks": "/speckit.tasks"}, CommandFiles: map[string]string{"tasks": rel}}
	if _, err := StagePrompt(in, "tasks", ""); err == nil {
		t.Fatal("template symlink outside project was accepted")
	}
}
func TestRequireStage(t *testing.T) {
	if _, e := RequireStage(Installation{Commands: map[string]string{"analyze": "/speckit.analyze"}}, "analyze"); e != nil {
		t.Fatal(e)
	}
	if _, e := RequireStage(Installation{}, "converge"); e == nil {
		t.Fatal("expected missing stage")
	}
}
func TestDiscoveryUsesInstalledCommandFiles(t *testing.T) {
	d := t.TempDir()
	if e := os.MkdirAll(filepath.Join(d, ".specify", "integrations"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(d, ".opencode", "commands"), 0755); e != nil {
		t.Fatal(e)
	}
	content := []byte("signed fixture command")
	rel := filepath.ToSlash(filepath.Join(".opencode", "commands", "speckit.analyze.md"))
	if e := os.WriteFile(filepath.Join(d, filepath.FromSlash(rel)), content, 0644); e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(content)
	manifest, err := json.Marshal(map[string]any{"integration": "opencode", "files": map[string]string{rel: hex.EncodeToString(digest[:])}})
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(d, ".specify", "integrations", "opencode.manifest.json")
	if err := os.WriteFile(manifestPath, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	integration, commands, files, hashes, err := discoverCommandFiles(d)
	if err != nil || integration != "opencode" || commands["analyze"] != "/speckit.analyze" || files["analyze"] != rel || hashes["analyze"] != hex.EncodeToString(digest[:]) {
		t.Fatalf("discovery=%s %v %v %v err=%v", integration, commands, files, hashes, err)
	}
	if err := os.WriteFile(filepath.Join(d, filepath.FromSlash(rel)), []byte("modified command"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err = discoverCommandFiles(d); err == nil || !strings.Contains(err.Error(), "SPEC_KIT_COMMAND_INTEGRITY_MISMATCH") {
		t.Fatalf("modified command integrity error=%v", err)
	}
}
