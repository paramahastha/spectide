// Package config reads and validates non-secret SpecTide settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FileName = "config.json"

type Route struct {
	Provider           string  `json:"provider"`
	Profile            string  `json:"profile"`
	Model              string  `json:"model,omitempty"`
	Effort             string  `json:"effort,omitempty"`
	FallbackAuthorized bool    `json:"fallback_authorized,omitempty"`
	Fallback           []Route `json:"fallback,omitempty"`
}
type Config struct {
	Version       int              `json:"version"`
	Project       string           `json:"project,omitempty"`
	RoutingMode   string           `json:"routing_mode"`
	DefaultRoute  Route            `json:"default_route"`
	StageRoutes   map[string]Route `json:"stage_routes,omitempty"`
	AllowedScopes []string         `json:"allowed_scopes,omitempty"`
}

var StageKeys = []string{"constitution", "specify", "clarify", "plan", "checklist", "tasks", "analyze", "implement", "tests", "converge", "final_review", "report"}

type Decision struct {
	Stage       string `json:"stage"`
	Route       Route  `json:"route"`
	Mode        string `json:"mode"`
	Explanation string `json:"explanation"`
}

func ProjectPath(root string) string { return filepath.Join(root, ".spectide", FileName) }

func GlobalPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "spectide", FileName)
}

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

// LoadEffective applies the default, user-global, then project configuration
// layers. Project values override global values; stage routes merge by stage.
func LoadEffective(globalPath, projectPath string) (Config, error) {
	effective := Default()
	for _, path := range []string{globalPath, projectPath} {
		if path == "" {
			continue
		}
		override, exists, err := loadOptional(path)
		if err != nil {
			return Config{}, err
		}
		if exists {
			effective = merge(effective, override)
		}
	}
	if err := Validate(effective); err != nil {
		return Config{}, err
	}
	return effective, nil
}

func loadOptional(path string) (Config, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	var c Config
	if err = json.Unmarshal(b, &c); err != nil {
		return Config{}, false, fmt.Errorf("invalid config %s: %w", path, err)
	}
	if err = Validate(c); err != nil {
		return Config{}, false, err
	}
	return c, true, nil
}

func merge(base, override Config) Config {
	if override.Version != 0 {
		base.Version = override.Version
	}
	if override.Project != "" {
		base.Project = override.Project
	}
	if override.RoutingMode != "" {
		base.RoutingMode = override.RoutingMode
	}
	if !routeEmpty(override.DefaultRoute) {
		base.DefaultRoute = mergeRoute(base.DefaultRoute, override.DefaultRoute)
	} else if override.RoutingMode == "manual" {
		base.DefaultRoute = Route{}
	}
	if override.StageRoutes != nil {
		if base.StageRoutes == nil {
			base.StageRoutes = make(map[string]Route)
		}
		for stage, route := range override.StageRoutes {
			base.StageRoutes[stage] = mergeRoute(base.StageRoutes[stage], route)
		}
	}
	if override.AllowedScopes != nil {
		base.AllowedScopes = append([]string(nil), override.AllowedScopes...)
	}
	return base
}

func routeEmpty(route Route) bool {
	return route.Provider == "" && route.Profile == "" && route.Model == "" && route.Effort == "" && !route.FallbackAuthorized && route.Fallback == nil
}

func mergeRoute(base, override Route) Route {
	if override.Provider != "" {
		base.Provider = override.Provider
	}
	if override.Profile != "" {
		base.Profile = override.Profile
	}
	if override.Model != "" {
		base.Model = override.Model
	}
	if override.Effort != "" {
		base.Effort = override.Effort
	}
	if override.Fallback != nil {
		base.Fallback = append([]Route(nil), override.Fallback...)
		base.FallbackAuthorized = override.FallbackAuthorized
	} else if override.FallbackAuthorized {
		base.FallbackAuthorized = true
	}
	return base
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
		if !knownStage(stage) {
			return fmt.Errorf("UNKNOWN_STAGE: %s", stage)
		}
		if !knownProvider(r.Provider) {
			return fmt.Errorf("UNKNOWN_PROVIDER: %s", r.Provider)
		}
		if err := validateFallbacks(r); err != nil {
			return fmt.Errorf("INVALID_STAGE_ROUTE: %s: %w", stage, err)
		}
	}
	if c.DefaultRoute.Provider != "" && !knownProvider(c.DefaultRoute.Provider) {
		return fmt.Errorf("UNKNOWN_PROVIDER: %s", c.DefaultRoute.Provider)
	}
	if c.DefaultRoute.Provider != "" {
		if err := validateFallbacks(c.DefaultRoute); err != nil {
			return fmt.Errorf("INVALID_DEFAULT_ROUTE: %w", err)
		}
	}
	if c.RoutingMode != "manual" && (c.DefaultRoute.Provider == "" || c.DefaultRoute.Profile == "") {
		return fmt.Errorf("DEFAULT_ROUTE_REQUIRED")
	}
	return nil
}

func validateFallbacks(route Route) error {
	if len(route.Fallback) > 0 && !route.FallbackAuthorized {
		return fmt.Errorf("FALLBACK_NOT_AUTHORIZED")
	}
	for i, fallback := range route.Fallback {
		if fallback.Provider == "" || fallback.Profile == "" {
			return fmt.Errorf("FALLBACK_ROUTE_INCOMPLETE: %d", i)
		}
		if !knownProvider(fallback.Provider) {
			return fmt.Errorf("UNKNOWN_FALLBACK_PROVIDER: %s", fallback.Provider)
		}
		if fallback.FallbackAuthorized || len(fallback.Fallback) > 0 {
			return fmt.Errorf("NESTED_FALLBACK_UNSUPPORTED: %d", i)
		}
	}
	return nil
}

func knownProvider(provider string) bool {
	return provider == "claude" || provider == "codex" || provider == "opencode" || provider == "fake"
}

func (c Config) Resolve(stage, prompt string) (Decision, error) {
	explicit, has := c.StageRoutes[stage]
	if !knownStage(stage) {
		return Decision{}, fmt.Errorf("UNKNOWN_STAGE: %s", stage)
	}
	var route Route
	var explanation string
	switch c.RoutingMode {
	case "manual":
		if !has {
			return Decision{}, fmt.Errorf("MANUAL_ROUTE_REQUIRED: %s", stage)
		}
		route = explicit
		explanation = "explicit manual stage route"
	case "hybrid":
		if has {
			route = explicit
			explanation = "explicit route override"
		} else {
			route = c.DefaultRoute
			explanation = "hybrid default route"
		}
	case "automatic":
		route = c.DefaultRoute
		if risky(prompt) {
			explanation = "automatic deterministic policy: high-risk task routed to configured default"
		} else {
			explanation = "automatic deterministic default route"
		}
	default:
		return Decision{}, fmt.Errorf("INVALID_ROUTING_MODE: %s", c.RoutingMode)
	}
	if route.Provider == "" || route.Profile == "" {
		return Decision{}, fmt.Errorf("ROUTE_INCOMPLETE: %s", stage)
	}
	return Decision{Stage: stage, Route: route, Mode: c.RoutingMode, Explanation: explanation}, nil
}

func knownStage(stage string) bool {
	for _, key := range StageKeys {
		if key == stage {
			return true
		}
	}
	return false
}
func risky(prompt string) bool {
	p := strings.ToLower(prompt)
	for _, term := range []string{"payment", "refund", "auth", "concurrency", "migration", "security"} {
		if strings.Contains(p, term) {
			return true
		}
	}
	return false
}
