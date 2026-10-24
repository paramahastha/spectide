package projects

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
)

func TestRegistrationIsCanonicalIdempotentAndReadinessIsComputed(t *testing.T) {
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	service := Service{Store: state}
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "project-link")
	if err = os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	registered, created, err := service.Register(context.Background(), link, "demo")
	if err != nil || !created {
		t.Fatalf("register: project=%+v created=%v err=%v", registered, created, err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if registered.CanonicalPath != canonicalRoot {
		t.Fatalf("path=%q want canonical %q", registered.CanonicalPath, canonicalRoot)
	}
	repeated, created, err := service.Register(context.Background(), root, "renamed")
	if err != nil || created || repeated.ID != registered.ID || repeated.DisplayName != "demo" {
		t.Fatalf("repeat registration overwrote identity: %+v created=%v err=%v", repeated, created, err)
	}
	if got := service.Readiness(context.Background(), registered); got.Status != "MISSING_CONFIGURATION" {
		t.Fatalf("readiness=%+v", got)
	}
	if err = config.Save(config.ProjectPath(canonicalRoot), config.Default()); err != nil {
		t.Fatal(err)
	}
	if got := service.Readiness(context.Background(), registered); got.Status != "READY" {
		t.Fatalf("configured fake route readiness=%+v", got)
	}
	if _, err = service.ResolvePath(context.Background(), filepath.Join(t.TempDir(), "unregistered")); err == nil {
		t.Fatal("unregistered path resolved")
	}
}

func TestReadinessReportsDistinctConfigurationAndAuthorizationStates(t *testing.T) {
	for _, expected := range []string{"INVALID_CONFIGURATION", "MISSING_ROUTE", "MISSING_PROFILE", "PROFILE_NOT_AUTHORIZED", "CAPABILITY_NOT_VERIFIED", "PATH_CHANGED"} {
		t.Run(expected, func(t *testing.T) {
			ctx := context.Background()
			state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer state.Close()
			service := Service{Store: state}
			root := t.TempDir()
			canonical, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			project, _, err := service.Register(ctx, canonical, "matrix")
			if err != nil {
				t.Fatal(err)
			}
			configPath := config.ProjectPath(canonical)
			if err = os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
				t.Fatal(err)
			}
			cfg := config.Default()
			cfg.Project, cfg.AllowedScopes = canonical, []string{canonical}
			profileScopes := []string{canonical}
			switch expected {
			case "INVALID_CONFIGURATION":
				if err = os.WriteFile(configPath, []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "MISSING_ROUTE":
				cfg.DefaultRoute = config.Route{}
				raw, marshalErr := json.Marshal(cfg)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if err = os.WriteFile(configPath, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			case "MISSING_PROFILE", "PROFILE_NOT_AUTHORIZED", "CAPABILITY_NOT_VERIFIED":
				cfg.DefaultRoute = config.Route{Provider: "codex", Profile: "matrix-codex"}
				if err = config.Save(configPath, cfg); err != nil {
					t.Fatal(err)
				}
				if expected == "MISSING_PROFILE" {
					break
				}
				if expected == "PROFILE_NOT_AUTHORIZED" {
					profileScopes = []string{t.TempDir()}
				}
				_, err = state.SaveProfile(ctx, model.Profile{Name: "matrix-codex", Provider: "codex", Executable: "codex", AllowedScopes: profileScopes})
				if err != nil {
					t.Fatal(err)
				}
			case "PATH_CHANGED":
				if err = config.Save(configPath, cfg); err != nil {
					t.Fatal(err)
				}
				if err = os.RemoveAll(canonical); err != nil {
					t.Fatal(err)
				}
			}
			got := service.Readiness(ctx, project)
			if got.Status != expected {
				t.Fatalf("readiness=%+v want status %s", got, expected)
			}
		})
	}
}
