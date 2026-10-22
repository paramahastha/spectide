package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectConfigSaveLoadIsPrivateAndValidated(t *testing.T) {
	path := ProjectPath(t.TempDir())
	c := Default()
	c.Project = "example"
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "example" {
		t.Fatalf("config=%+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	bad := c
	bad.RoutingMode = "automatic-ish"
	if Validate(bad) == nil {
		t.Fatal("invalid routing mode accepted")
	}
}

func TestMissingConfigGetsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || c.RoutingMode != "hybrid" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestManualHybridAndAutomaticRouting(t *testing.T) {
	c := Default()
	c.RoutingMode = "manual"
	c.DefaultRoute = Route{}
	if _, err := c.Resolve("plan", "task"); err == nil {
		t.Fatal("manual mode silently used a default")
	}
	c.StageRoutes = map[string]Route{"plan": {Provider: "codex", Profile: "work"}}
	d, err := c.Resolve("plan", "task")
	if err != nil || d.Route.Profile != "work" || d.Explanation != "explicit manual stage route" {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
	c = Default()
	c.StageRoutes = map[string]Route{"plan": {Provider: "codex", Profile: "work"}}
	d, err = c.Resolve("plan", "task")
	if err != nil || d.Route.Provider != "codex" {
		t.Fatalf("hybrid decision=%+v err=%v", d, err)
	}
	c.RoutingMode = "automatic"
	d, err = c.Resolve("analyze", "security migration")
	if err != nil || !strings.Contains(d.Explanation, "high-risk") {
		t.Fatalf("automatic decision=%+v err=%v", d, err)
	}
}
