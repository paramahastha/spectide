package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spectide/spectide/internal/app"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
	"github.com/spectide/spectide/internal/speckit"
	"github.com/spf13/cobra"
)

func Root() *cobra.Command {
	var db string
	root := &cobra.Command{Use: "spectide", Short: "Local, evidence-backed Spec-Kit orchestration", SilenceUsage: true}
	root.PersistentFlags().StringVar(&db, "db", "", "SQLite state database path")
	open := func() (*app.App, error) { return app.Open(db) }
	root.AddCommand(&cobra.Command{Use: "version", Run: func(c *cobra.Command, _ []string) { fmt.Fprintln(c.OutOrStdout(), "spectide 0.1.0-dev") }})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Report local provider and Spec-Kit readiness", RunE: func(c *cobra.Command, _ []string) error {
		providers := map[string]string{"fake": "SIMULATED_READY", "claude": probe("claude"), "codex": probe("codex"), "opencode": probe("opencode")}
		_, specErr := speckit.Discover(".")
		specStatus := "READY"
		if specErr != nil {
			specStatus = specErr.Error()
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"database": app.DefaultDBPath(), "providers": providers, "spec_kit": specStatus, "live_workflows": "BLOCKED_UNTIL_ROUTE_CONFIGURATION_AND_STAGE_EXECUTION_ARE_COMPLETE"})
	}})
	serve := &cobra.Command{Use: "serve", Short: "Serve loopback dashboard and API", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		host, _ := c.Flags().GetString("host")
		port, _ := c.Flags().GetInt("port")
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return fmt.Errorf("only loopback host is allowed")
		}
		addr := netJoin(host, port)
		fmt.Fprintf(c.OutOrStdout(), "SpecTide listening at http://%s\n", addr)
		return http.ListenAndServe(addr, a.API.Handler())
	}}
	serve.Flags().String("host", "127.0.0.1", "loopback host")
	serve.Flags().Int("port", 4747, "local port")
	root.AddCommand(serve)
	run := &cobra.Command{Use: "run PROMPT", Args: cobra.ExactArgs(1), Short: "Start an offline simulated workflow (live workflow execution is not yet enabled)", RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		project, _ := c.Flags().GetString("project")
		r, e := a.Engine.Start(context.Background(), project, args[0])
		if e != nil {
			return e
		}
		fmt.Fprintf(c.OutOrStdout(), "SIMULATED run started: %s\n", r.ID)
		result, e := a.Engine.Wait(context.Background(), r.ID)
		if e != nil {
			return e
		}
		if result.State != model.Completed {
			return fmt.Errorf("run ended %s", result.State)
		}
		fmt.Fprintln(c.OutOrStdout(), "SIMULATED run completed with fixture evidence")
		return nil
	}}
	run.Flags().String("project", ".", "registered local project label")
	root.AddCommand(run)
	demo := &cobra.Command{Use: "demo", Short: "Run the deterministic offline demonstration", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		project, _ := c.Flags().GetString("project")
		r, e := a.Engine.Start(context.Background(), project, "Run deterministic offline SpecTide verification")
		if e != nil {
			return e
		}
		fmt.Fprintf(c.OutOrStdout(), "SIMULATED run started: %s\n", r.ID)
		result, e := a.Engine.Wait(context.Background(), r.ID)
		if e != nil {
			return e
		}
		if result.State != model.Completed {
			return fmt.Errorf("run ended %s", result.State)
		}
		fmt.Fprintln(c.OutOrStdout(), "SIMULATED run completed with fixture evidence")
		return nil
	}}
	demo.Flags().String("project", "examples/fake-provider-demo", "demo project path")
	root.AddCommand(demo)
	root.AddCommand(&cobra.Command{Use: "list", Short: "List durable runs", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		v, e := a.Store.Runs(context.Background())
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(v)
	}})
	for _, name := range []string{"status", "report"} {
		commandName := name
		root.AddCommand(&cobra.Command{Use: commandName + " RUN_ID", Args: cobra.ExactArgs(1), Short: "Show durable run evidence", RunE: func(c *cobra.Command, args []string) error {
			a, e := open()
			if e != nil {
				return e
			}
			defer a.Close()
			v, e := a.Store.Report(context.Background(), args[0])
			if e != nil {
				return e
			}
			return json.NewEncoder(c.OutOrStdout()).Encode(v)
		}})
	}
	root.AddCommand(&cobra.Command{Use: "cancel RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		return a.Engine.Cancel(context.Background(), args[0])
	}})
	root.AddCommand(&cobra.Command{Use: "resume RUN_ID", Args: cobra.ExactArgs(1), Short: "Resume a safely checkpointed run", RunE: func(c *cobra.Command, args []string) error {
		return fmt.Errorf("RESUME_UNAVAILABLE: run %s has no verified safe checkpoint", args[0])
	}})
	root.AddCommand(&cobra.Command{Use: "dashboard", Short: "Print the local dashboard URL", Run: func(c *cobra.Command, _ []string) {
		fmt.Fprintln(c.OutOrStdout(), "Start `spectide serve` then open http://127.0.0.1:4747")
	}})
	root.AddCommand(&cobra.Command{Use: "logs RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		v, e := a.Store.Events(context.Background(), args[0], 0)
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(v)
	}})
	profiles := &cobra.Command{Use: "profiles", Short: "Manage sanitized local profiles", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		p, e := a.Store.Profiles(context.Background())
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(p)
	}}
	add := &cobra.Command{Use: "add NAME", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		provider, _ := c.Flags().GetString("provider")
		executable, _ := c.Flags().GetString("executable")
		scope, _ := c.Flags().GetString("scope")
		if executable == "" {
			executable = provider
		}
		if !validProvider(provider) {
			return fmt.Errorf("provider must be claude, codex, opencode, or fake")
		}
		p, e := a.Store.SaveProfile(context.Background(), model.Profile{Name: args[0], Provider: provider, Executable: executable, AllowedScopes: splitCSV(scope)})
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(p)
	}}
	add.Flags().String("provider", "", "provider name")
	add.Flags().String("executable", "", "local executable name or path")
	add.Flags().String("scope", "personal-public", "comma-separated allowed project scopes")
	profiles.AddCommand(add)
	root.AddCommand(profiles)
	root.AddCommand(&cobra.Command{Use: "models", Short: "Report local model discovery readiness", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		ps, e := a.Store.Profiles(context.Background())
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"profiles": ps, "catalog": "NOT_PROBED", "reason": "Model IDs must be validated by each provider CLI in its authorized profile."})
	}})
	conf := &cobra.Command{Use: "config", Short: "Inspect or validate project configuration", RunE: func(c *cobra.Command, _ []string) error {
		path, _ := c.Flags().GetString("file")
		v, e := config.Load(path)
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(v)
	}}
	conf.Flags().String("file", config.ProjectPath("."), "configuration file path")
	conf.AddCommand(&cobra.Command{Use: "validate", RunE: func(c *cobra.Command, _ []string) error {
		path, _ := c.Flags().GetString("file")
		v, e := config.Load(path)
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"valid": true, "config": v})
	}})
	conf.Commands()[0].Flags().String("file", config.ProjectPath("."), "configuration file path")
	preview := &cobra.Command{Use: "preview", Short: "Preview deterministic routes without starting a run", RunE: func(c *cobra.Command, _ []string) error {
		file, _ := c.Flags().GetString("file")
		prompt, _ := c.Flags().GetString("prompt")
		cfg, e := config.Load(file)
		if e != nil {
			return e
		}
		decisions := make([]config.Decision, 0, len(config.StageKeys))
		for _, stage := range config.StageKeys {
			d, e := cfg.Resolve(stage, prompt)
			if e != nil {
				return e
			}
			decisions = append(decisions, d)
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"mode": cfg.RoutingMode, "decisions": decisions})
	}}
	preview.Flags().String("file", config.ProjectPath("."), "configuration file path")
	preview.Flags().String("prompt", "", "task intent for automatic risk routing")
	conf.AddCommand(preview)
	root.AddCommand(conf)
	initCmd := &cobra.Command{Use: "init", Short: "Initialize local SpecTide project settings", RunE: func(c *cobra.Command, _ []string) error {
		rootPath, _ := c.Flags().GetString("project")
		abs, e := filepath.Abs(rootPath)
		if e != nil {
			return e
		}
		if st, e := os.Stat(abs); e != nil || !st.IsDir() {
			return fmt.Errorf("project path must be an existing directory")
		}
		path := config.ProjectPath(abs)
		v, e := config.Load(path)
		if e != nil {
			return e
		}
		v.Project = abs
		if v.AllowedScopes == nil {
			v.AllowedScopes = []string{abs}
		}
		if e = config.Save(path, v); e != nil {
			return e
		}
		fmt.Fprintf(c.OutOrStdout(), "Initialized %s\n", path)
		if _, e = os.Stat(filepath.Join(abs, ".specify")); e != nil {
			fmt.Fprintln(c.OutOrStdout(), "Spec-Kit is not initialized here; use the installed `specify` CLI to initialize an authorized integration.")
		}
		return nil
	}}
	initCmd.Flags().String("project", ".", "project directory")
	root.AddCommand(initCmd)
	verify := &cobra.Command{Use: "verify", Short: "Run the registered local quality checks", RunE: func(c *cobra.Command, _ []string) error {
		project, _ := c.Flags().GetString("project")
		abs, e := filepath.Abs(project)
		if e != nil {
			return e
		}
		if st, e := os.Stat(abs); e != nil || !st.IsDir() {
			return fmt.Errorf("project path must be an existing directory")
		}
		registry := quality.DefaultRegistry(nil)
		results := registry.Run(c.Context(), abs)
		if e = json.NewEncoder(c.OutOrStdout()).Encode(results); e != nil {
			return e
		}
		if !registry.RequiredPassed(results) {
			return fmt.Errorf("QUALITY_CHECK_FAILED")
		}
		return nil
	}}
	verify.Flags().String("project", ".", "project directory to validate")
	root.AddCommand(verify)
	return root
}

func probe(bin string) string {
	if _, e := exec.LookPath(bin); e != nil {
		return "NOT_FOUND"
	}
	return "FOUND_NOT_AUTHENTICATED"
}
func validProvider(p string) bool {
	return p == "claude" || p == "codex" || p == "opencode" || p == "fake"
}
func splitCSV(value string) []string {
	var out []string
	for _, v := range strings.Split(value, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
func netJoin(host string, port int) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}
