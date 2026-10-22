package speckit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectsUninitializedProject(t *testing.T) {
	if _, e := Discover(t.TempDir()); e == nil {
		t.Fatal("expected error")
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
	if e := os.MkdirAll(filepath.Join(d, ".specify"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(d, ".opencode", "commands"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(d, ".opencode", "commands", "speckit.analyze.md"), []byte("x"), 0644); e != nil {
		t.Fatal(e)
	}
	_, _ = Discover(d)
}
