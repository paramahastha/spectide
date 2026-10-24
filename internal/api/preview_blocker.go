package api

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spectide/spectide/internal/config"
)

var previewSafeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type PreviewRemediation struct {
	Target string `json:"target"`
	Label  string `json:"label"`
}

type PreviewBlocker struct {
	Category    string             `json:"category"`
	Message     string             `json:"message"`
	Remediation PreviewRemediation `json:"remediation"`
	Stage       string             `json:"stage,omitempty"`
	Profile     string             `json:"profile,omitempty"`
	Provider    string             `json:"provider,omitempty"`
}

func classifyPreviewBlocker(err error, cfg *config.Config) PreviewBlocker {
	message := "The route preview could not be completed. Review project readiness before trying again."
	if err == nil {
		return PreviewBlocker{Category: "UNKNOWN", Message: message, Remediation: PreviewRemediation{Target: "readiness", Label: "Review project readiness"}}
	}
	detail := err.Error()
	upper := strings.ToUpper(detail)
	category, target, label := "UNKNOWN", "readiness", "Review project readiness"
	switch {
	case strings.Contains(upper, "PROJECT_NOT_REGISTERED"), strings.Contains(upper, "PROJECT_PATH_UNAVAILABLE"), strings.Contains(upper, "PROJECT_NOT_DIRECTORY"), strings.Contains(upper, "PATH_CHANGED"):
		category, target, label = "PROJECT_UNAVAILABLE", "project-selection", "Select an available project"
		message = "The registered project is unavailable. Select an available project before previewing routes."
	case strings.Contains(upper, "ROUTE_REQUIRED"), strings.Contains(upper, "PROFILE_REQUIRED"), strings.Contains(upper, "MISSING_ROUTE"):
		category, target, label = "MISSING_ROUTE", "routing", "Review route settings"
		message = "A route is not configured for this project. Review route settings before previewing."
	case strings.Contains(upper, "PROFILE_NOT_FOUND"), strings.Contains(upper, "FALLBACK_PROFILE_NOT_FOUND"):
		category, target, label = "MISSING_PROFILE", "profiles", "Review profile setup"
		message = "A configured route needs a registered provider profile. Review profile setup before previewing."
	case strings.Contains(upper, "PROFILE_UNAUTHENTICATED"), strings.Contains(upper, "PROFILE_AUTHENTICATION_NOT_VERIFIED"):
		category, target, label = "CAPABILITY_NOT_VERIFIED", "readiness", "Review profile readiness"
		message = "Provider authentication evidence needs verification before routes can be previewed."
	case strings.Contains(upper, "PROFILE_NOT_ALLOWED"), strings.Contains(upper, "PROFILE_NOT_AUTHORIZED"):
		category, target, label = "PROFILE_NOT_AUTHORIZED", "profiles", "Review profile access"
		message = "A configured profile is not authorized for this project. Review profile access before previewing."
	case strings.Contains(upper, "CAPABILITY"), strings.Contains(upper, "MODEL_NOT_VERIFIED"), strings.Contains(upper, "EFFORT_NOT_VERIFIED"), strings.Contains(upper, "READ_ONLY_EXECUTION_UNVERIFIED"), strings.Contains(upper, "WORKSPACE_WRITE_UNVERIFIED"):
		category, target, label = "CAPABILITY_NOT_VERIFIED", "readiness", "Review profile readiness"
		message = "Provider capability evidence needs verification before routes can be previewed."
	case strings.Contains(upper, "CONFIG"), strings.Contains(upper, "INVALID"), strings.Contains(upper, "SCOPE"), strings.Contains(upper, "PERMISSION_DENIED"):
		category, target, label = "CONFIGURATION_INVALID", "routing", "Review route settings"
		message = "Project route configuration needs attention before it can be previewed."
	}
	blocker := PreviewBlocker{Category: category, Message: message, Remediation: PreviewRemediation{Target: target, Label: label}}
	if cfg == nil {
		return blocker
	}
	routes := []struct {
		stage string
		route config.Route
	}{{stage: "", route: cfg.DefaultRoute}}
	for stage, route := range cfg.StageRoutes {
		routes = append(routes, struct {
			stage string
			route config.Route
		}{stage: stage, route: route})
	}
	for _, stage := range config.StageKeys {
		if strings.Contains(detail, "for "+stage) || strings.Contains(detail, "for "+stage+":") || strings.Contains(detail, "route missing for "+stage) {
			blocker.Stage = stage
			break
		}
	}
	for _, item := range routes {
		matched := config.Route{}
		for _, candidate := range append([]config.Route{item.route}, item.route.Fallback...) {
			if candidate.Profile != "" && strings.Contains(detail, candidate.Profile) {
				matched = candidate
				break
			}
		}
		if matched.Profile == "" {
			continue
		}
		if !strings.Contains(upper, "PROFILE") && !strings.Contains(upper, "CAPABILITY") && !strings.Contains(upper, "AUTH") {
			continue
		}
		blocker.Profile = matched.Profile
		blocker.Provider = matched.Provider
		if blocker.Stage == "" && item.stage != "" {
			blocker.Stage = item.stage
		}
		break
	}
	if blocker.Stage != "" {
		valid := false
		for _, stage := range config.StageKeys {
			if blocker.Stage == stage {
				valid = true
				break
			}
		}
		if !valid {
			blocker.Stage = ""
		}
	}
	if !previewSafeIdentifier.MatchString(blocker.Profile) {
		blocker.Profile = ""
	}
	if blocker.Provider != "fake" && blocker.Provider != "codex" && blocker.Provider != "claude" && blocker.Provider != "opencode" {
		blocker.Provider = ""
	}
	if blocker.Category == "UNKNOWN" && (blocker.Stage != "" || blocker.Profile != "") {
		return blocker
	}
	if blocker.Category == "UNKNOWN" {
		blocker.Message = fmt.Sprintf("%s", message)
	}
	return blocker
}
