package app

import (
	"context"
	"fmt"
	"github.com/spectide/spectide/internal/api"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/workflow"
	"os"
	"path/filepath"
)

type App struct {
	Store  *store.Store
	Engine *workflow.Engine
	API    *api.Server
}

func DefaultDBPath() string {
	d, e := os.UserConfigDir()
	if e != nil {
		d = "."
	}
	return filepath.Join(d, "spectide", "spectide.db")
}
func Open(dbPath string) (*App, error) {
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}
	s, e := store.Open(dbPath)
	if e != nil {
		return nil, fmt.Errorf("open state store: %w", e)
	}
	if e = s.RecoverInterrupted(context.Background()); e != nil {
		s.Close()
		return nil, fmt.Errorf("recover interrupted runs: %w", e)
	}
	eng := workflow.New(s)
	return &App{Store: s, Engine: eng, API: api.New(s, eng)}, nil
}
func (a *App) Close() error { return a.Store.Close() }
