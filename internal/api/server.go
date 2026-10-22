package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/events"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/routing"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/webassets"
	"github.com/spectide/spectide/internal/workflow"
	workspaceguard "github.com/spectide/spectide/internal/workspace"
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
	Store  *store.Store
	Engine *workflow.Engine
	CSRF   string
}

func New(s *store.Store, e *workflow.Engine) *Server {
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return &Server{Store: s, Engine: e, CSRF: hex.EncodeToString(token)}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/api/v1/healthz", s.health)
	mux.HandleFunc("/api/v1/readyz", s.health)
	mux.HandleFunc("/api/v1/version", s.version)
	mux.HandleFunc("/api/v1/runs", s.runs)
	mux.HandleFunc("/api/v1/runs/", s.run)
	mux.HandleFunc("/api/v1/profiles", s.profiles)
	mux.HandleFunc("/api/v1/projects", s.projects)
	mux.HandleFunc("/api/v1/projects/", s.project)
	assets, _ := fs.Sub(webassets.Static, "static")
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return s.secure(mux)
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

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9:_-]{8,128}$`)

const sseReplayPageSize = 512

func apiError(w http.ResponseWriter, status int, code, message string) {
	jsonOut(w, status, map[string]any{"error": map[string]string{"code": code, "message": security.Redact(message)}})
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
		jsonOut(w, 200, projects)
	case http.MethodPost:
		var input struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := decodeRequest(w, r, 16<<10, &input); err != nil || strings.TrimSpace(input.Path) == "" || strings.TrimSpace(input.Name) == "" {
			apiError(w, 400, "INVALID_REQUEST", "project path and name are required")
			return
		}
		abs, err := filepath.Abs(input.Path)
		if err != nil {
			apiError(w, 400, "INVALID_PROJECT_PATH", err.Error())
			return
		}
		canonical, err := filepath.EvalSymlinks(abs)
		if err != nil {
			apiError(w, 422, "PROJECT_PATH_UNAVAILABLE", err.Error())
			return
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.IsDir() {
			apiError(w, 422, "PROJECT_NOT_DIRECTORY", canonical)
			return
		}
		if err = s.Store.RegisterProject(r.Context(), canonical, strings.TrimSpace(input.Name), []string{canonical}); err != nil {
			apiError(w, 422, "PROJECT_INVALID", err.Error())
			return
		}
		projects, err := s.Store.Projects(r.Context())
		if err != nil {
			apiError(w, 500, "STORE_ERROR", err.Error())
			return
		}
		for _, project := range projects {
			if project.CanonicalPath == canonical {
				jsonOut(w, http.StatusCreated, project)
				return
			}
		}
		apiError(w, 500, "PROJECT_NOT_PERSISTED", "registered project could not be read")
	default:
		apiError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "config" || r.Method != http.MethodGet {
		apiError(w, 404, "NOT_FOUND", "project endpoint not found")
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
		encoded = []byte(`{"error":{"code":"SERIALIZATION_FAILED","message":"response serialization failed"}}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(security.Redact(string(encoded) + "\n")))
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, 200, map[string]any{"status": "ok", "local_only": true})
}
func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, 200, map[string]any{"version": "0.1.0-dev", "csrf_token": s.CSRF, "providers": map[string]string{"fake": "SIMULATED", "claude": "NOT_PROBED", "codex": "NOT_PROBED", "opencode": "NOT_PROBED"}})
}
func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		rs, e := s.Store.Runs(r.Context())
		if e != nil {
			apiError(w, http.StatusInternalServerError, "STORE_ERROR", e.Error())
			return
		}
		jsonOut(w, 200, rs)
	case "POST":
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if !idempotencyPattern.MatchString(idempotencyKey) {
			apiError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "provide an Idempotency-Key of 8–128 safe characters")
			return
		}
		var in struct {
			Project string `json:"project"`
			Prompt  string `json:"prompt"`
			DryRun  bool   `json:"dry_run"`
		}
		if e := decodeRequest(w, r, 32<<10, &in); e != nil || strings.TrimSpace(in.Project) == "" || strings.TrimSpace(in.Prompt) == "" {
			apiError(w, http.StatusBadRequest, "INVALID_REQUEST", "project and prompt are required")
			return
		}
		if in.DryRun {
			root, err := canonicalProject(in.Project)
			if err != nil {
				apiError(w, http.StatusUnprocessableEntity, "ROUTE_PREVIEW_BLOCKED", err.Error())
				return
			}
			configPath, err := safeProjectConfigPath(root)
			if err != nil {
				apiError(w, http.StatusUnprocessableEntity, "ROUTE_PREVIEW_BLOCKED", err.Error())
				return
			}
			cfg, err := config.LoadEffective(config.GlobalPath(), configPath)
			if err != nil {
				apiError(w, http.StatusUnprocessableEntity, "ROUTE_PREVIEW_BLOCKED", err.Error())
				return
			}
			if len(cfg.AllowedScopes) > 0 {
				if _, err = workspaceguard.Canonical(root, cfg.AllowedScopes); err != nil {
					apiError(w, http.StatusUnprocessableEntity, "ROUTE_PREVIEW_BLOCKED", err.Error())
					return
				}
			}
			profiles, profileErr := s.Store.Profiles(r.Context())
			if profileErr != nil {
				apiError(w, http.StatusInternalServerError, "STORE_ERROR", profileErr.Error())
				return
			}
			if err = routing.ValidateProfileRoutes(cfg, root, profiles); err != nil {
				apiError(w, http.StatusUnprocessableEntity, "ROUTE_PREVIEW_BLOCKED", err.Error())
				return
			}
			if err = routing.ValidateProfileCapabilities(cfg, profiles); err != nil {
				apiError(w, http.StatusUnprocessableEntity, "ROUTE_PREVIEW_BLOCKED", err.Error())
				return
			}
			decisions := make([]config.Decision, 0, len(config.StageKeys))
			for _, stage := range config.StageKeys {
				decision, resolveErr := cfg.Resolve(stage, in.Prompt)
				if resolveErr != nil {
					apiError(w, http.StatusUnprocessableEntity, "ROUTE_PREVIEW_BLOCKED", resolveErr.Error())
					return
				}
				decisions = append(decisions, decision)
			}
			jsonOut(w, http.StatusOK, map[string]any{"dry_run": true, "execution": "NOT_RUN", "mode": cfg.RoutingMode, "decisions": decisions})
			return
		}
		run, reused, e := s.Engine.StartIdempotent(r.Context(), idempotencyKey, in.Project, in.Prompt)
		if e != nil {
			apiError(w, http.StatusUnprocessableEntity, "RUN_REJECTED", e.Error())
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
	path := config.ProjectPath(root)
	resolved, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("PROJECT_CONFIG_UNAVAILABLE: %w", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("PERMISSION_DENIED: project config escapes canonical root")
	}
	return resolved, nil
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/runs/"), "/")
	id := parts[0]
	if id == "" {
		apiError(w, http.StatusNotFound, "RUN_ID_REQUIRED", "run id is required")
		return
	}
	if len(parts) == 1 {
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
	switch parts[1] {
	case "events":
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
		if err := s.Store.ResolveApproval(r.Context(), tail[0], decision); err != nil {
			apiError(w, http.StatusConflict, "APPROVAL_NOT_RESOLVED", err.Error())
			return
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
