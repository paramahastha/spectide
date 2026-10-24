// Package projects owns canonical project registration and launch resolution.
package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/routing"
	"github.com/spectide/spectide/internal/store"
)

type Service struct{ Store *store.Store }

func Canonical(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("PROJECT_PATH_REQUIRED")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("PROJECT_PATH_INVALID")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("PROJECT_PATH_UNAVAILABLE")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("PROJECT_NOT_DIRECTORY")
	}
	return filepath.Clean(resolved), nil
}

func (s Service) Register(ctx context.Context, path, name string) (model.Project, bool, error) {
	canonical, err := Canonical(path)
	if err != nil {
		return model.Project{}, false, err
	}
	if name == "" {
		name = filepath.Base(canonical)
	}
	existing, err := s.Store.ProjectByPath(ctx, canonical)
	if err == nil {
		return existing, false, nil
	}
	if err = s.Store.RegisterProject(ctx, canonical, name, []string{canonical}); err != nil {
		return model.Project{}, false, err
	}
	created, err := s.Store.ProjectByPath(ctx, canonical)
	return created, true, err
}

func (s Service) ResolvePath(ctx context.Context, path string) (model.Project, error) {
	canonical, err := Canonical(path)
	if err != nil {
		return model.Project{}, err
	}
	project, err := s.Store.ProjectByPath(ctx, canonical)
	if err != nil {
		return model.Project{}, fmt.Errorf("PROJECT_NOT_REGISTERED: run spectide add %s", canonical)
	}
	return project, nil
}

func (s Service) ResolveID(ctx context.Context, id string) (model.Project, error) {
	project, err := s.Store.Project(ctx, id)
	if err != nil {
		return model.Project{}, fmt.Errorf("PROJECT_NOT_REGISTERED")
	}
	canonical, err := Canonical(project.CanonicalPath)
	if err != nil || canonical != project.CanonicalPath {
		return model.Project{}, fmt.Errorf("PATH_CHANGED")
	}
	return project, nil
}

func (s Service) Readiness(ctx context.Context, project model.Project) model.ProjectReadiness {
	canonical, err := Canonical(project.CanonicalPath)
	if err != nil || canonical != project.CanonicalPath {
		return model.ProjectReadiness{Status: "PATH_CHANGED", Reason: "The registered directory is unavailable or no longer canonical.", Action: "Restore the directory or add its current path."}
	}
	configPath, err := config.SafeProjectPath(canonical)
	if err != nil {
		return model.ProjectReadiness{Status: "PATH_CHANGED", Reason: "Project configuration is outside the registered directory."}
	}
	if _, err = os.Stat(configPath); os.IsNotExist(err) {
		return model.ProjectReadiness{Status: "MISSING_CONFIGURATION", Reason: "No project configuration is present.", Action: "Configure a route for this project."}
	}
	if err != nil {
		return model.ProjectReadiness{Status: "INVALID_CONFIGURATION", Reason: "Project configuration could not be read."}
	}
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		return model.ProjectReadiness{Status: "INVALID_CONFIGURATION", Reason: "Project configuration could not be read."}
	}
	var declared config.Config
	if json.Unmarshal(raw, &declared) == nil && declared.RoutingMode != "" && declared.DefaultRoute.Provider == "" && declared.DefaultRoute.Profile == "" {
		return model.ProjectReadiness{Status: "MISSING_ROUTE", Reason: "No default route is configured.", Action: "Configure a route for this project."}
	}
	cfg, err := config.LoadEffective(config.GlobalPath(), configPath)
	if err != nil {
		return model.ProjectReadiness{Status: "INVALID_CONFIGURATION", Reason: "Project configuration is invalid."}
	}
	if cfg.DefaultRoute.Provider == "" || cfg.DefaultRoute.Profile == "" {
		return model.ProjectReadiness{Status: "MISSING_ROUTE", Reason: "No default route is configured."}
	}
	profiles, err := s.Store.Profiles(ctx)
	if err != nil {
		return model.ProjectReadiness{Status: "INVALID_CONFIGURATION", Reason: "Project readiness could not be evaluated."}
	}
	for _, route := range append([]config.Route{cfg.DefaultRoute}, routeValues(cfg.StageRoutes)...) {
		if route.Provider == "fake" {
			continue
		}
		found := false
		for _, profile := range profiles {
			if profile.Name == route.Profile && string(profile.Provider) == route.Provider {
				found = true
				break
			}
		}
		if !found {
			return model.ProjectReadiness{Status: "MISSING_PROFILE", Reason: "A configured provider profile is not registered.", Action: "Register the required local profile."}
		}
	}
	if err = routing.ValidateProfileRoutes(cfg, canonical, profiles); err != nil {
		return model.ProjectReadiness{Status: "PROFILE_NOT_AUTHORIZED", Reason: "A configured profile is not authorized for this project."}
	}
	if err = routing.ValidateProfileCapabilities(cfg, profiles); err != nil {
		return model.ProjectReadiness{Status: "CAPABILITY_NOT_VERIFIED", Reason: "Provider capabilities have not been verified."}
	}
	return model.ProjectReadiness{Status: "READY", Reason: "Project configuration and provider evidence are ready."}
}

func routeValues(routes map[string]config.Route) []config.Route {
	result := make([]config.Route, 0, len(routes))
	for _, route := range routes {
		result = append(result, route)
	}
	return result
}
