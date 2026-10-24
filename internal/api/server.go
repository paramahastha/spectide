package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spectide/spectide/internal/buildinfo"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/events"
	"github.com/spectide/spectide/internal/model"
	projectsvc "github.com/spectide/spectide/internal/projects"
	"github.com/spectide/spectide/internal/routing"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/webassets"
	"github.com/spectide/spectide/internal/workflow"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Store        *store.Store
	Engine       *workflow.Engine
	CSRF         string
	DashboardURL string
}

func New(s *store.Store, e *workflow.Engine) *Server {
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return &Server{Store: s, Engine: e, CSRF: hex.EncodeToString(token), DashboardURL: "http://127.0.0.1:4747"}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/api/v1/healthz", s.health)
	mux.HandleFunc("/api/v1/readyz", s.health)
	mux.HandleFunc("/api/v1/version", s.version)
	mux.HandleFunc("/api/v1/workspace/status", s.workspaceStatus)
	mux.HandleFunc("/api/v1/capabilities", s.capabilities)
	mux.HandleFunc("/api/v1/providers/capabilities", s.capabilities)
	mux.HandleFunc("/api/v1/config/validate", s.validateConfig)
	mux.HandleFunc("/api/v1/config/resolved", s.resolvedConfig)
	mux.HandleFunc("/api/v1/approvals", s.pendingApprovals)
	mux.HandleFunc("/api/v1/runs", s.runs)
	mux.HandleFunc("/api/v1/runs/", s.run)
	mux.HandleFunc("/api/v1/profiles", s.profiles)
	mux.HandleFunc("/api/v1/profiles/", s.profile)
	mux.HandleFunc("/api/v1/projects", s.projects)
	mux.HandleFunc("/api/v1/projects/", s.project)
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, _ *http.Request) {
		apiError(w, http.StatusNotFound, "NOT_FOUND", "API endpoint not found")
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, _ *http.Request) {
		apiError(w, http.StatusNotFound, "NOT_FOUND", "API endpoint not found")
	})
	assets, _ := fs.Sub(webassets.Static, "static")
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return s.secure(mux)
}

func (s *Server) workspaceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	id, err := s.Store.WorkspaceID(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, "STORE_ERROR", "workspace status unavailable")
		return
	}
	url := s.DashboardURL
	if url == "" {
		url = "http://127.0.0.1:4747"
	}
	jsonOut(w, http.StatusOK, model.WorkspaceStatus{WorkspaceID: id, Version: buildinfo.Version, DashboardURL: url, LocalOnly: true})
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/"), "/")
	if id == "" || strings.Contains(id, "/") {
		apiError(w, http.StatusNotFound, "NOT_FOUND", "profile endpoint not found")
		return
	}
	if r.Method != http.MethodPatch {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var profile model.Profile
	if err := decodeRequest(w, r, 16<<10, &profile); err != nil {
		apiError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	profile.ID = id
	profile.Capability = nil // capability observations can only come from a verified probe
	updated, err := s.Store.UpdateProfile(r.Context(), id, profile)
	if err != nil {
		if strings.Contains(err.Error(), "PROFILE_NOT_FOUND") {
			apiError(w, http.StatusNotFound, "PROFILE_NOT_FOUND", "profile not found")
			return
		}
		apiError(w, http.StatusUnprocessableEntity, "PROFILE_INVALID", err.Error())
		return
	}
	jsonOut(w, http.StatusOK, updated)
}
func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profiles, err := s.Store.Profiles(r.Context())
		if err != nil {
			apiError(w, 500, "STORE_ERROR", err.Error())
			return
		}
		jsonOut(w, 200, profiles)
	case http.MethodPost:
		var profile model.Profile
		if err := decodeRequest(w, r, 16<<10, &profile); err != nil {
			apiError(w, 400, "INVALID_REQUEST", err.Error())
			return
		}
		created, err := s.Store.SaveProfile(r.Context(), profile)
		if err != nil {
			apiError(w, 422, "PROFILE_INVALID", err.Error())
			return
		}
		jsonOut(w, http.StatusCreated, created)
	default:
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	profiles, err := s.Store.Profiles(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, "STORE_ERROR", err.Error())
		return
	}
	type capability struct {
		Profile             string    `json:"profile"`
		Provider            string    `json:"provider"`
		ExecutableFound     bool      `json:"executable_found"`
		Authenticated       string    `json:"authenticated"`
		ReadOnlyExecution   bool      `json:"read_only_execution"`
		WorkspaceWrite      bool      `json:"workspace_write"`
		ModelCatalogStatus  string    `json:"model_catalog_status,omitempty"`
		Models              []string  `json:"models,omitempty"`
		EffortValues        []string  `json:"effort_values,omitempty"`
		SafetyModes         []string  `json:"safety_modes,omitempty"`
		CommandCapabilities []string  `json:"command_capabilities,omitempty"`
		ObservedAt          time.Time `json:"observed_at,omitempty"`
	}
	items := make([]capability, 0, len(profiles))
	providerStatus := map[string]string{"fake": "SIMULATED_READY", "claude": "PROFILE_NOT_CONFIGURED", "codex": "PROFILE_NOT_CONFIGURED", "opencode": "PROFILE_NOT_CONFIGURED"}
	for _, profile := range profiles {
		if profile.Capability == nil {
			providerStatus[string(profile.Provider)] = "PROFILE_REGISTERED_NOT_PROBED"
			continue
		}
		providerStatus[string(profile.Provider)] = "PROFILE_READINESS_RECORDED"
		c := profile.Capability
		items = append(items, capability{Profile: profile.Name, Provider: profile.Provider, ExecutableFound: c.ExecutableFound, Authenticated: c.Authenticated, ReadOnlyExecution: c.ReadOnlyExecution, WorkspaceWrite: c.WorkspaceWrite, ModelCatalogStatus: c.ModelCatalogStatus, Models: append([]string(nil), c.ModelValues...), EffortValues: append([]string(nil), c.EffortValues...), SafetyModes: append([]string(nil), c.SafetyModes...), CommandCapabilities: append([]string(nil), c.CommandCapabilities...), ObservedAt: c.ObservedAt})
	}
	jsonOut(w, http.StatusOK, map[string]any{"providers": providerStatus, "profiles": items})
}

func (s *Server) pendingApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	approvals, err := s.Store.PendingApprovals(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, "STORE_ERROR", err.Error())
		return
	}
	if approvals == nil {
		approvals = []model.Approval{}
	}
	jsonOut(w, http.StatusOK, approvals)
}

// resolvedConfig returns only the sanitized global effective configuration.
// Project settings remain accessible through the registered canonical-project route.
func (s *Server) resolvedConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if len(r.URL.Query()) != 0 {
		apiError(w, http.StatusBadRequest, "INVALID_REQUEST", "config/resolved does not accept query parameters")
		return
	}
	value, err := config.LoadEffective(config.GlobalPath(), "")
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, "CONFIGURATION_INVALID", err.Error())
		return
	}
	jsonOut(w, http.StatusOK, value)
}

func (s *Server) validateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var input struct {
		Project string `json:"project"`
		Prompt  string `json:"prompt"`
	}
	if err := decodeRequest(w, r, 32<<10, &input); err != nil || strings.TrimSpace(input.Project) == "" || strings.TrimSpace(input.Prompt) == "" {
		apiError(w, http.StatusBadRequest, "INVALID_REQUEST", "project and prompt are required")
		return
	}
	preview, err := s.previewProject(r.Context(), input.Project, input.Prompt)
	if err != nil {
		apiPreviewBlocked(w, "CONFIGURATION_INVALID", err.Error())
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"valid": true, "preview": preview})
}

func (s *Server) previewProject(ctx context.Context, project, prompt string) (map[string]any, error) {
	root, err := canonicalProject(project)
	if err != nil {
		return nil, err
	}
	if _, err = s.Store.ProjectByPath(ctx, root); err != nil {
		return nil, fmt.Errorf("PROJECT_NOT_REGISTERED")
	}
	configPath, err := safeProjectConfigPath(root)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadEffective(config.GlobalPath(), configPath)
	if err != nil {
		return nil, err
	}
	profiles, err := s.Store.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	decisions, err := routing.ValidateLaunch(cfg, root, prompt, profiles)
	if err != nil {
		return nil, err
	}
	return map[string]any{"dry_run": true, "valid": true, "execution": "NOT_RUN", "mode": cfg.RoutingMode, "decisions": decisions}, nil
}

func queryInteger(r *http.Request, name string, fallback int) (int, error) {
	values, exists := r.URL.Query()[name]
	if !exists {
		return fallback, nil
	}
	if len(values) != 1 || values[0] == "" {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return strconv.Atoi(values[0])
}

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9:_-]{8,128}$`)

const sseReplayPageSize = 512

func apiError(w http.ResponseWriter, status int, code, message string) {
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = newRequestID()
		w.Header().Set("X-Request-ID", requestID)
	}
	jsonOut(w, status, contractError(code, message, requestID))
}

func apiPreviewBlocked(w http.ResponseWriter, code, message string) {
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = newRequestID()
		w.Header().Set("X-Request-ID", requestID)
	}
	response := contractError(code, message, requestID)
	response["execution"] = "NOT_RUN"
	response["dry_run"] = true
	response["blocked_reasons"] = []string{security.Redact(message)}
	jsonOut(w, http.StatusUnprocessableEntity, response)
}

func contractError(code, message, requestID string) map[string]any {
	return map[string]any{"error": map[string]any{"code": code, "message": security.Redact(message), "details": map[string]any{}, "request_id": requestID}, "request_id": requestID}
}

func newRequestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return hex.EncodeToString(value)
}

func decodeRequest(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("request must contain one JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := s.Store.Projects(r.Context())
		if err != nil {
			apiError(w, 500, "STORE_ERROR", err.Error())
			return
		}
		service := projectsvc.Service{Store: s.Store}
		for i := range projects {
			projects[i].Readiness = service.Readiness(r.Context(), projects[i])
		}
		jsonOut(w, 200, projects)
	case http.MethodPost:
		var input struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := decodeRequest(w, r, 16<<10, &input); err != nil || strings.TrimSpace(input.Path) == "" || !filepath.IsAbs(input.Path) {
			apiError(w, 400, "INVALID_REQUEST", "an absolute project path is required")
			return
		}
		project, created, err := (projectsvc.Service{Store: s.Store}).Register(r.Context(), input.Path, strings.TrimSpace(input.Name))
		if err != nil {
			apiError(w, 422, "PROJECT_INVALID", err.Error())
			return
		}
		project.Readiness = (projectsvc.Service{Store: s.Store}).Readiness(r.Context(), project)
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		jsonOut(w, status, project)
	default:
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "config" {
		apiError(w, 404, "NOT_FOUND", "project endpoint not found")
		return
	}
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	project, err := s.Store.Project(r.Context(), parts[0])
	if err != nil {
		apiError(w, 404, "PROJECT_NOT_FOUND", "registered project not found")
		return
	}
	resolved, err := filepath.EvalSymlinks(project.CanonicalPath)
	if err != nil || resolved != project.CanonicalPath {
		apiError(w, http.StatusForbidden, "PROJECT_PATH_CHANGED", "registered project path no longer resolves to its canonical location")
		return
	}
	configPath, err := safeProjectConfigPath(project.CanonicalPath)
	if err != nil {
		apiError(w, http.StatusForbidden, "PROJECT_CONFIG_PATH_INVALID", err.Error())
		return
	}
	value, err := config.LoadEffective(config.GlobalPath(), configPath)
	if err != nil {
		apiError(w, 422, "PROJECT_CONFIG_INVALID", err.Error())
		return
	}
	jsonOut(w, 200, value)
}
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", newRequestID())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		host, _, _ := net.SplitHostPort(r.Host)
		if host == "" {
			host = r.Host
		}
		if host != "localhost" && host != "127.0.0.1" && host != "[::1]" && host != "::1" {
			apiError(w, http.StatusForbidden, "INVALID_HOST", "invalid host")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.URL.Path != "/api/v1/healthz" && r.URL.Path != "/healthz" {
			if r.Header.Get("X-SpecTide-CSRF") != s.CSRF {
				apiError(w, http.StatusForbidden, "CSRF_REQUIRED", "missing or invalid CSRF token")
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				u, err := http.NewRequest(http.MethodGet, o, nil)
				if err != nil || u.URL.Scheme != "http" || u.URL.Host != r.Host || u.URL.User != nil || (u.URL.Path != "" && u.URL.Path != "/") || u.URL.RawQuery != "" || u.URL.Fragment != "" || (u.URL.Hostname() != "localhost" && u.URL.Hostname() != "127.0.0.1" && u.URL.Hostname() != "::1") {
					apiError(w, http.StatusForbidden, "INVALID_ORIGIN", "invalid origin")
					return
				}
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	encoded, err := json.Marshal(events.SanitizePayload(v))
	if err != nil {
		status = http.StatusInternalServerError
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
			w.Header().Set("X-Request-ID", requestID)
		}
		encoded, _ = json.Marshal(contractError("SERIALIZATION_FAILED", "response serialization failed", requestID))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(security.Redact(string(encoded) + "\n")))
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	jsonOut(w, 200, map[string]any{"status": "ok", "local_only": true})
}
func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	jsonOut(w, 200, map[string]any{"version": buildinfo.Version, "csrf_token": s.CSRF, "providers": map[string]string{"fake": "SIMULATED", "claude": "NOT_PROBED", "codex": "NOT_PROBED", "opencode": "NOT_PROBED"}})
}
func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		for key := range r.URL.Query() {
			if key != "limit" && key != "offset" && key != "project_id" {
				apiError(w, http.StatusBadRequest, "INVALID_PAGE", "only limit, offset, and project_id are supported")
				return
			}
		}
		limit, err := queryInteger(r, "limit", 100)
		if err != nil || limit < 1 || limit > 100 {
			apiError(w, http.StatusBadRequest, "INVALID_PAGE", "limit must be between 1 and 100")
			return
		}
		offset, err := queryInteger(r, "offset", 0)
		if err != nil || offset < 0 || offset > 1_000_000 {
			apiError(w, http.StatusBadRequest, "INVALID_PAGE", "offset must be between 0 and 1000000")
			return
		}
		projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
		var rs []model.Run
		var e error
		if projectID != "" {
			if _, err := s.Store.Project(r.Context(), projectID); err != nil {
				apiError(w, http.StatusBadRequest, "INVALID_PROJECT_ID", "project_id must identify a registered project")
				return
			}
			rs, e = s.Store.RunsPageForProject(r.Context(), projectID, limit, offset)
		} else {
			rs, e = s.Store.RunsPage(r.Context(), limit, offset)
		}
		if e != nil {
			apiError(w, http.StatusInternalServerError, "STORE_ERROR", e.Error())
			return
		}
		if rs == nil {
			rs = []model.Run{}
		}
		var total int
		if projectID != "" {
			total, e = s.Store.RunCountForProject(r.Context(), projectID)
		} else {
			total, e = s.Store.RunCount(r.Context())
		}
		if e != nil {
			apiError(w, http.StatusInternalServerError, "STORE_ERROR", e.Error())
			return
		}
		w.Header().Set("X-Total-Count", strconv.Itoa(total))
		w.Header().Set("X-Page-Limit", strconv.Itoa(limit))
		w.Header().Set("X-Page-Offset", strconv.Itoa(offset))
		w.Header().Set("X-Has-More", strconv.FormatBool(offset+len(rs) < total))
		jsonOut(w, 200, rs)
	case "POST":
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if !idempotencyPattern.MatchString(idempotencyKey) {
			apiError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "provide an Idempotency-Key of 8–128 safe characters")
			return
		}
		var in struct {
			ProjectID string `json:"project_id"`
			Project   string `json:"project"`
			Prompt    string `json:"prompt"`
			DryRun    bool   `json:"dry_run"`
		}
		if e := decodeRequest(w, r, 32<<10, &in); e != nil || (strings.TrimSpace(in.ProjectID) == "") == (strings.TrimSpace(in.Project) == "") || strings.TrimSpace(in.Prompt) == "" {
			apiError(w, http.StatusBadRequest, "INVALID_REQUEST", "exactly one project_id or project and a prompt are required")
			return
		}
		projectPath := in.Project
		if in.ProjectID != "" {
			resolved, resolveErr := (projectsvc.Service{Store: s.Store}).ResolveID(r.Context(), strings.TrimSpace(in.ProjectID))
			if resolveErr != nil {
				apiError(w, http.StatusUnprocessableEntity, "PROJECT_NOT_REGISTERED", "project_id does not identify an available registered project")
				return
			}
			projectPath = resolved.CanonicalPath
		}
		if in.DryRun {
			preview, err := s.previewProject(r.Context(), projectPath, in.Prompt)
			if err != nil {
				apiPreviewBlocked(w, "ROUTE_PREVIEW_BLOCKED", err.Error())
				return
			}
			jsonOut(w, http.StatusOK, preview)
			return
		}
		run, reused, e := s.Engine.StartIdempotent(r.Context(), idempotencyKey, projectPath, in.Prompt)
		if e != nil {
			code := "RUN_REJECTED"
			if strings.HasPrefix(e.Error(), "PROJECT_NOT_REGISTERED:") {
				code = "PROJECT_NOT_REGISTERED"
			}
			apiError(w, http.StatusUnprocessableEntity, code, e.Error())
			return
		}
		status := http.StatusAccepted
		if reused {
			status = http.StatusOK
		}
		jsonOut(w, status, run)
	default:
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func canonicalProject(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("PROJECT_PATH_UNAVAILABLE: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("PROJECT_NOT_DIRECTORY")
	}
	return filepath.Clean(root), nil
}

func safeProjectConfigPath(root string) (string, error) {
	path, err := config.SafeProjectPath(root)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/runs/"), "/")
	id := parts[0]
	if id == "" {
		apiError(w, http.StatusNotFound, "RUN_ID_REQUIRED", "run id is required")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		report, e := s.Store.Report(r.Context(), id)
		if e != nil {
			apiError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
			return
		}
		jsonOut(w, 200, report)
		return
	}
	if parts[1] == "approvals" {
		s.approvals(w, r, id, parts[2:])
		return
	}
	if len(parts) != 2 {
		apiError(w, http.StatusNotFound, "NOT_FOUND", "run endpoint not found")
		return
	}
	switch parts[1] {
	case "stages":
		if r.Method != http.MethodGet {
			apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		if _, err := s.Store.GetRun(r.Context(), id); err != nil {
			apiError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
			return
		}
		stages, err := s.Store.Stages(r.Context(), id)
		if err != nil {
			apiError(w, http.StatusInternalServerError, "STORE_ERROR", err.Error())
			return
		}
		if stages == nil {
			stages = []model.Stage{}
		}
		jsonOut(w, http.StatusOK, stages)
	case "artifacts":
		if r.Method != http.MethodGet {
			apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		if _, err := s.Store.GetRun(r.Context(), id); err != nil {
			apiError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
			return
		}
		artifacts, err := s.Store.Artifacts(r.Context(), id)
		if err != nil {
			apiError(w, http.StatusInternalServerError, "STORE_ERROR", err.Error())
			return
		}
		if artifacts == nil {
			artifacts = []model.Artifact{}
		}
		jsonOut(w, http.StatusOK, artifacts)
	case "events":
		if r.Method != http.MethodGet {
			apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.events(w, r, id)
	case "cancel":
		if r.Method != "POST" {
			apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		if err := s.Engine.Cancel(r.Context(), id); err != nil {
			apiError(w, http.StatusConflict, "RUN_CONTROL_REJECTED", err.Error())
			return
		}
		jsonOut(w, 202, map[string]string{"status": "cancellation requested"})
	case "pause", "resume":
		if r.Method != http.MethodPost {
			apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var err error
		if parts[1] == "pause" {
			err = s.Engine.Pause(id)
		} else {
			err = s.Engine.Resume(id)
		}
		if err != nil {
			apiError(w, http.StatusConflict, "RUN_CONTROL_REJECTED", err.Error())
			return
		}
		jsonOut(w, http.StatusAccepted, map[string]string{"status": parts[1] + " requested"})
	case "report":
		if r.Method != http.MethodGet {
			apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		rep, e := s.Store.Report(r.Context(), id)
		if e != nil {
			apiError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
			return
		}
		jsonOut(w, 200, rep)
	default:
		apiError(w, http.StatusNotFound, "NOT_FOUND", "run endpoint not found")
	}
}

func (s *Server) approvals(w http.ResponseWriter, r *http.Request, runID string, tail []string) {
	if _, err := s.Store.GetRun(r.Context(), runID); err != nil {
		apiError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
		return
	}
	if len(tail) == 0 && r.Method == http.MethodPost {
		var input struct {
			Action string `json:"action"`
		}
		if err := decodeRequest(w, r, 8<<10, &input); err != nil || strings.TrimSpace(input.Action) == "" {
			apiError(w, http.StatusBadRequest, "INVALID_REQUEST", "approval action is required")
			return
		}
		approval, err := s.Store.CreateApproval(r.Context(), model.Approval{RunID: runID, Action: strings.TrimSpace(input.Action)})
		if err != nil {
			apiError(w, http.StatusUnprocessableEntity, "APPROVAL_INVALID", err.Error())
			return
		}
		jsonOut(w, http.StatusCreated, approval)
		return
	}
	if len(tail) == 1 && r.Method == http.MethodPost {
		var input struct {
			Decision string `json:"decision"`
		}
		if err := decodeRequest(w, r, 8<<10, &input); err != nil {
			apiError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		decision := strings.ToUpper(input.Decision)
		if err := s.Store.ResolveApprovalForRun(r.Context(), runID, tail[0], decision); err != nil {
			apiError(w, http.StatusConflict, "APPROVAL_NOT_RESOLVED", err.Error())
			return
		}
		if run, err := s.Store.GetRun(r.Context(), runID); err == nil && run.State == model.Paused {
			if resumeErr := s.Engine.Resume(runID); resumeErr != nil {
				latest, getErr := s.Store.GetRun(r.Context(), runID)
				if getErr != nil || latest.State == model.Paused {
					apiError(w, http.StatusConflict, "APPROVAL_CONTINUATION_FAILED", resumeErr.Error())
					return
				}
			}
		}
		jsonOut(w, http.StatusOK, map[string]string{"status": decision})
		return
	}
	apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func (s *Server) events(w http.ResponseWriter, r *http.Request, id string) {
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if r.URL.Query().Get("after") == "" {
		after = 0
	} else if err != nil || after < 0 {
		apiError(w, http.StatusBadRequest, "INVALID_EVENT_CURSOR", "event cursor must be a non-negative integer")
		return
	}
	if h := r.Header.Get("Last-Event-ID"); h != "" {
		after, err = strconv.ParseInt(h, 10, 64)
		if err != nil || after < 0 {
			apiError(w, http.StatusBadRequest, "INVALID_EVENT_CURSOR", "event cursor must be a non-negative integer")
			return
		}
	}
	if _, err = s.Store.GetRun(r.Context(), id); err != nil {
		apiError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, ok := w.(http.Flusher)
	if !ok {
		apiError(w, http.StatusInternalServerError, "STREAM_UNAVAILABLE", "stream unavailable")
		return
	}
	controller := http.NewResponseController(w)
	highWater, err := s.Store.EventHighWater(r.Context(), id)
	if err != nil {
		return
	}
	replaying := after < highWater
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		through := highWater
		if !replaying {
			through, err = s.Store.EventHighWater(r.Context(), id)
		}
		if err != nil {
			return
		}
		events, e := s.Store.EventsPage(r.Context(), id, after, through, sseReplayPageSize)
		if e != nil {
			return
		}
		for _, event := range events {
			b, _ := json.Marshal(event)
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Seq, event.Type, b); e != nil {
				return
			}
			after = event.Seq
		}
		if err = controller.Flush(); err != nil {
			return
		}
		if replaying {
			if after >= highWater || len(events) == 0 {
				replaying = false
			} else {
				continue
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err = controller.Flush(); err != nil {
				return
			}
		}
	}
}
