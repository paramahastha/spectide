package app

import (
	"fmt"
	"github.com/spectide/spectide/internal/api"
	"github.com/spectide/spectide/internal/projects"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/workflow"
	"os"
	"path/filepath"
	"runtime"
)

type App struct {
	Store    *store.Store
	Engine   *workflow.Engine
	API      *api.Server
	Projects projects.Service
}

func DefaultDBPath() string {
	home, _ := os.UserHomeDir()
	configDir, _ := os.UserConfigDir()
	return defaultDBPathFor(runtime.GOOS, home, configDir, os.Getenv("XDG_DATA_HOME"))
}

func defaultDBPathFor(goos, home, configDir, xdgDataHome string) string {
	if goos == "darwin" && home != "" {
		return filepath.Join(home, "Library", "Application Support", "spectide", "spectide.db")
	}
	if goos == "linux" {
		if xdgDataHome != "" && filepath.IsAbs(xdgDataHome) {
			return filepath.Join(xdgDataHome, "spectide", "spectide.db")
		}
		if home != "" {
			return filepath.Join(home, ".local", "share", "spectide", "spectide.db")
		}
	}
	if configDir == "" {
		configDir = "."
	}
	return filepath.Join(configDir, "spectide", "spectide.db")
}

// LegacyDBPath returns the former config-directory database only when it is
// distinct from the current platform default. Callers must require explicit
// confirmation before importing it.
func LegacyDBPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	legacy := filepath.Join(d, "spectide", "spectide.db")
	current, err := filepath.Abs(DefaultDBPath())
	if err != nil {
		return ""
	}
	legacyAbs, err := filepath.Abs(legacy)
	if err != nil || legacyAbs == current {
		return ""
	}
	return legacyAbs
}
func Open(dbPath string) (*App, error) {
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}
	s, e := store.Open(dbPath)
	if e != nil {
		return nil, fmt.Errorf("open state store: %w", e)
	}
	eng := workflow.New(s)
	return &App{Store: s, Engine: eng, API: api.New(s, eng), Projects: projects.Service{Store: s}}, nil
}
func (a *App) Close() error { return a.Store.Close() }
