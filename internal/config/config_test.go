package config

import (
	"os"
	"path/filepath"
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
