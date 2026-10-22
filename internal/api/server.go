package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/webassets"
	"github.com/spectide/spectide/internal/workflow"
	"io/fs"
	"net"
	"net/http"
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
	mux.HandleFunc("/api/v1/healthz", s.health)
	mux.HandleFunc("/api/v1/readyz", s.health)
	mux.HandleFunc("/api/v1/version", s.version)
	mux.HandleFunc("/api/v1/runs", s.runs)
	mux.HandleFunc("/api/v1/runs/", s.run)
	mux.HandleFunc("/api/v1/profiles", s.profiles)
	assets, _ := fs.Sub(webassets.Static, "static")
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return s.secure(mux)
}
func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profiles, err := s.Store.Profiles(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		jsonOut(w, 200, profiles)
	case http.MethodPost:
		var profile model.Profile
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&profile); err != nil {
			http.Error(w, "invalid profile", 400)
			return
		}
		created, err := s.Store.SaveProfile(r.Context(), profile)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		jsonOut(w, http.StatusCreated, created)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		if host == "" {
			host = r.Host
		}
		if host != "localhost" && host != "127.0.0.1" && host != "[::1]" {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.URL.Path != "/api/v1/healthz" {
			if r.Header.Get("X-SpecTide-CSRF") != s.CSRF {
				http.Error(w, "missing or invalid CSRF token", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				u, err := http.NewRequest(http.MethodGet, o, nil)
				if err != nil || u.URL.Scheme != "http" || u.URL.Host != r.Host || (u.URL.Hostname() != "localhost" && u.URL.Hostname() != "127.0.0.1" && u.URL.Hostname() != "::1") {
					http.Error(w, "invalid origin", http.StatusForbidden)
					return
				}
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
			http.Error(w, e.Error(), 500)
			return
		}
		jsonOut(w, 200, rs)
	case "POST":
		var in struct {
			Project string `json:"project"`
			Prompt  string `json:"prompt"`
			DryRun  bool   `json:"dry_run"`
		}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); e != nil || strings.TrimSpace(in.Project) == "" || strings.TrimSpace(in.Prompt) == "" {
			http.Error(w, "project and prompt are required", 400)
			return
		}
		if in.DryRun {
			jsonOut(w, 200, map[string]any{"dry_run": true, "provider": "fake", "stages": 12, "security": "local simulated route"})
			return
		}
		run, e := s.Engine.Start(r.Context(), in.Project, in.Prompt)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		jsonOut(w, 202, run)
	default:
		http.Error(w, "method not allowed", 405)
	}
}
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/runs/"), "/")
	id := parts[0]
	if id == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		report, e := s.Store.Report(r.Context(), id)
		if e != nil {
			http.Error(w, "run not found", 404)
			return
		}
		jsonOut(w, 200, report)
		return
	}
	switch parts[1] {
	case "events":
		s.events(w, r, id)
	case "cancel":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		_ = s.Engine.Cancel(context.Background(), id)
		jsonOut(w, 202, map[string]string{"status": "cancellation requested"})
	case "report":
		rep, e := s.Store.Report(r.Context(), id)
		if e != nil {
			http.Error(w, "run not found", 404)
			return
		}
		jsonOut(w, 200, rep)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) events(w http.ResponseWriter, r *http.Request, id string) {
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if r.URL.Query().Get("after") == "" {
		after = 0
	} else if err != nil || after < 0 {
		http.Error(w, "invalid event cursor", http.StatusBadRequest)
		return
	}
	if h := r.Header.Get("Last-Event-ID"); h != "" {
		after, err = strconv.ParseInt(h, 10, 64)
		if err != nil || after < 0 {
			http.Error(w, "invalid event cursor", http.StatusBadRequest)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unavailable", 500)
		return
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, e := s.Store.Events(r.Context(), id, after)
		if e != nil {
			return
		}
		for _, event := range events {
			b, _ := json.Marshal(event)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Seq, event.Type, b)
			after = event.Seq
		}
		f.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			f.Flush()
		}
	}
}
