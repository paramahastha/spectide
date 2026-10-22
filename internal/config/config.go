// Package config reads and validates non-secret SpecTide settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const FileName = "config.json"

type Route struct {
	Provider string `json:"provider"`
	Profile  string `json:"profile"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
}
type Config struct {
	Version       int              `json:"version"`
	Project       string           `json:"project,omitempty"`
	RoutingMode   string           `json:"routing_mode"`
	DefaultRoute  Route            `json:"default_route"`
	StageRoutes   map[string]Route `json:"stage_routes,omitempty"`
	AllowedScopes []string         `json:"allowed_scopes,omitempty"`
}

func ProjectPath(root string) string { return filepath.Join(root, ".spectide", FileName) }

func Default() Config {
	return Config{Version: 1, RoutingMode: "hybrid", DefaultRoute: Route{Provider: "fake", Profile: "fake-demo", Model: "deterministic", Effort: "none"}}
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	var c Config
	decErr := json.Unmarshal(b, &c)
	if decErr != nil {
		return Config{}, fmt.Errorf("invalid config %s: %w", path, decErr)
	}
	if err := Validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func Save(path string, c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func Validate(c Config) error {
	if c.Version != 1 {
		return fmt.Errorf("CONFIG_VERSION_UNSUPPORTED: %d", c.Version)
	}
	switch c.RoutingMode {
	case "manual", "hybrid", "automatic":
	default:
		return fmt.Errorf("INVALID_ROUTING_MODE: %q", c.RoutingMode)
	}
	for stage, r := range c.StageRoutes {
		if stage == "" || r.Profile == "" || r.Provider == "" {
			return fmt.Errorf("INVALID_STAGE_ROUTE: %q requires provider and profile", stage)
		}
	}
	if c.RoutingMode == "manual" && (c.DefaultRoute.Provider == "" || c.DefaultRoute.Profile == "") {
		return fmt.Errorf("MANUAL_DEFAULT_ROUTE_REQUIRED")
	}
	return nil
}
