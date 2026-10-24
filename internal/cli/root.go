package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/app"
	"github.com/spectide/spectide/internal/buildinfo"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/events"
	"github.com/spectide/spectide/internal/model"
	projectsvc "github.com/spectide/spectide/internal/projects"
	"github.com/spectide/spectide/internal/quality"
	"github.com/spectide/spectide/internal/routing"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/speckit"
	"github.com/spectide/spectide/internal/store"
	"github.com/spf13/cobra"
)

func writeJSON(out io.Writer, value any) error {
	encoded, err := json.Marshal(events.SanitizePayload(value))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, security.Redact(string(encoded)))
	return err
}

func commandOutput(c *cobra.Command) io.Writer {
	quiet, _ := c.Root().PersistentFlags().GetBool("quiet")
	if quiet {
		return io.Discard
	}
	return c.OutOrStdout()
}

func confirmLegacyImport(reader *bufio.Reader, output io.Writer, source, destination string) (bool, error) {
	fmt.Fprintf(output, "Legacy state: %s\nNew workspace: %s\nImport this state with a private backup? [y/N] ", source, destination)
	answer, err := reader.ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

func persistedCapability(c adapters.Capability) model.ProfileCapability {
	return model.ProfileCapability{
		Provider: string(c.Provider), ExecutableFound: c.ExecutableFound, Version: c.Version,
		ApprovedExecutablePath: c.ApprovedExecutablePath, ExecutableSHA256: c.ExecutableSHA256,
		ApprovedConfigDirectory: c.ApprovedConfigDirectory,
		Authenticated:           c.Authenticated, StreamingEvents: c.StreamingEvents,
		ModelOverride: c.ModelOverride, EffortOverride: c.EffortOverride,
		ModelCatalogStatus: c.ModelCatalogStatus, ModelValues: append([]string(nil), c.ModelValues...), AuthenticatedProviders: append([]string(nil), c.AuthenticatedProviders...),
		EffortValues: append([]string(nil), c.EffortValues...), SafetyModes: append([]string(nil), c.SafetyModes...), CommandCapabilities: append([]string(nil), c.CommandCapabilities...),
		ReadOnlyExecution: c.ReadOnlyExecution, WorkspaceWrite: c.WorkspaceWrite,
		Warnings: c.Warnings,
	}
}

func Root() *cobra.Command {
	var db string
	root := &cobra.Command{Use: "spectide", Short: "Local, evidence-backed Spec-Kit orchestration", SilenceUsage: true}
	root.PersistentFlags().StringVar(&db, "db", "", "SQLite state database path")
	root.PersistentFlags().Bool("no-color", false, "disable ANSI color output (the current CLI emits no ANSI color)")
	root.PersistentFlags().Bool("quiet", false, "suppress normal stdout; errors remain on stderr")
	root.PersistentFlags().Bool("verbose", false, "write command diagnostics to stderr")
	root.PersistentPreRun = func(c *cobra.Command, _ []string) {
		verbose, _ := c.Root().PersistentFlags().GetBool("verbose")
		if verbose {
			fmt.Fprintf(c.ErrOrStderr(), "spectide: command=%s\n", c.CommandPath())
		}
	}
	open := func() (*app.App, error) { return app.Open(db) }
	register := func(c *cobra.Command, raw string) error {
		a, err := open()
		if err != nil {
			return err
		}
		defer a.Close()
		project, created, err := a.Projects.Register(c.Context(), raw, "")
		if err != nil {
			return err
		}
		if created {
			fmt.Fprintf(commandOutput(c), "Added %s (%s)\n", project.DisplayName, project.CanonicalPath)
		} else {
			fmt.Fprintf(commandOutput(c), "Already added %s (%s)\n", project.DisplayName, project.CanonicalPath)
		}
		return nil
	}
	root.AddCommand(&cobra.Command{Use: "add [PATH]", Args: cobra.MaximumNArgs(1), Short: "Add an existing project to the shared workspace", RunE: func(c *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		return register(c, path)
	}})
	root.AddCommand(&cobra.Command{Use: "setup", Args: cobra.NoArgs, Short: "Set up the current project in the shared workspace", RunE: func(c *cobra.Command, _ []string) error {
		canonical, err := projectsvc.Canonical(".")
		if err != nil {
			return err
		}
		reader := bufio.NewReader(c.InOrStdin())
		fmt.Fprintf(commandOutput(c), "Project: %s\nAdd this repository to the shared workspace? [y/N] ", canonical)
		answer, err := reader.ReadString('\n')
		if err != nil && len(answer) == 0 {
			return err
		}
		if strings.ToLower(strings.TrimSpace(answer)) != "y" && strings.ToLower(strings.TrimSpace(answer)) != "yes" {
			return fmt.Errorf("SETUP_DECLINED")
		}
		if db == "" {
			legacy := app.LegacyDBPath()
			if legacy != "" {
				legacyInfo, legacyErr := os.Lstat(legacy)
				if legacyErr == nil {
					destination, absErr := filepath.Abs(app.DefaultDBPath())
					if absErr != nil {
						return absErr
					}
					if !legacyInfo.Mode().IsRegular() || legacyInfo.Mode()&os.ModeSymlink != 0 {
						return fmt.Errorf("LEGACY_IMPORT_SOURCE_INVALID: %s", legacy)
					}
					if _, destinationErr := os.Lstat(destination); destinationErr == nil {
						fmt.Fprintf(commandOutput(c), "Legacy state found at %s; destination already exists at %s, so no import was attempted.\n", legacy, destination)
					} else if !os.IsNotExist(destinationErr) {
						return fmt.Errorf("LEGACY_IMPORT_DESTINATION_UNAVAILABLE")
					} else {
						confirmed, confirmErr := confirmLegacyImport(reader, commandOutput(c), legacy, destination)
						if confirmErr != nil {
							return confirmErr
						}
						if confirmed {
							backup, importErr := store.ImportLegacyWorkspace(c.Context(), legacy, destination, true)
							if importErr != nil {
								return importErr
							}
							fmt.Fprintf(commandOutput(c), "Legacy state imported; source retained. Private backup: %s\n", backup)
						} else {
							fmt.Fprintln(commandOutput(c), "Legacy state import declined; the source was left unchanged.")
						}
					}
				} else if !os.IsNotExist(legacyErr) {
					return fmt.Errorf("LEGACY_IMPORT_SOURCE_UNAVAILABLE")
				}
			}
		}
		projectConfig, err := config.SafeProjectPath(canonical)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(projectConfig); os.IsNotExist(statErr) {
			if err = config.Save(projectConfig, config.Default()); err != nil {
				return fmt.Errorf("PROJECT_CONFIG_CREATE_FAILED: %w", err)
			}
		} else if statErr != nil {
			return fmt.Errorf("PROJECT_CONFIG_UNAVAILABLE")
		}
		a, err := open()
		if err != nil {
			return err
		}
		defer a.Close()
		project, created, err := a.Projects.Register(c.Context(), canonical, "")
		if err != nil {
			return err
		}
		readiness := a.Projects.Readiness(c.Context(), project)
		url, dashboardErr := a.EnsureDashboard(c.Context(), db)
		if created {
			fmt.Fprintf(commandOutput(c), "Added %s (%s)\n", project.DisplayName, project.CanonicalPath)
		}
		fmt.Fprintf(commandOutput(c), "Readiness: %s — %s\n", readiness.Status, readiness.Reason)
		if readiness.Action != "" {
			fmt.Fprintf(commandOutput(c), "Next: %s\n", readiness.Action)
		}
		if dashboardErr != nil {
			return dashboardErr
		}
		fmt.Fprintf(commandOutput(c), "SpecTide dashboard ready: %s\n", url)
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "version", Run: func(c *cobra.Command, _ []string) { fmt.Fprintf(commandOutput(c), "spectide %s\n", buildinfo.Version) }})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Report local provider and Spec-Kit readiness", RunE: func(c *cobra.Command, _ []string) error {
		providers := map[string]string{"fake": "SIMULATED_READY", "claude": probe("claude"), "codex": probe("codex"), "opencode": probe("opencode")}
		_, specErr := speckit.Discover(".")
		specStatus := "READY"
		if specErr != nil {
			specStatus = specErr.Error()
		}
		return writeJSON(commandOutput(c), map[string]any{"database": app.DefaultDBPath(), "providers": providers, "spec_kit": specStatus, "live_workflows": "AVAILABLE_FOR_CONFIGURED_AND_AUTHORIZED_ROUTES"})
	}})
	serve := &cobra.Command{Use: "serve", Short: "Serve loopback dashboard and API", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		if e = a.Store.RecoverAbandonedRuns(c.Context(), time.Now().UTC()); e != nil {
			return fmt.Errorf("startup recovery: %w", e)
		}
		if e = a.Store.PruneRetention(c.Context(), time.Now().UTC()); e != nil {
			return fmt.Errorf("retention maintenance: %w", e)
		}
		go a.Store.RunRetentionMaintenance(c.Context(), time.Hour, func(err error) {
			log.Printf("SpecTide retention maintenance failed: %v", err)
		})
		host, _ := c.Flags().GetString("host")
		port, _ := c.Flags().GetInt("port")
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return fmt.Errorf("only loopback host is allowed")
		}
		addr := netJoin(host, port)
		listener, e := net.Listen("tcp", addr)
		if e != nil {
			if port == 4747 {
				url, reuseErr := a.EnsureDashboard(c.Context(), db)
				if reuseErr != nil {
					return reuseErr
				}
				fmt.Fprintf(commandOutput(c), "This workspace already has a verified dashboard at %s; use `spectide dashboard` to open it.\n", url)
				return nil
			}
			return fmt.Errorf("CUSTOM_PORT_OCCUPIED: cannot bind %s; choose another loopback port: %w", addr, e)
		}
		defer listener.Close()
		a.API.DashboardURL = "http://" + listener.Addr().String()
		fmt.Fprintf(commandOutput(c), "SpecTide listening at http://%s\n", listener.Addr().String())
		return http.Serve(listener, a.API.Handler())
	}}
	serve.Flags().String("host", "127.0.0.1", "loopback host")
	serve.Flags().Int("port", 4747, "local port")
	root.AddCommand(serve)
	run := &cobra.Command{Use: "run PROMPT", Args: cobra.ExactArgs(1), Short: "Start a workflow using the configured project routes", RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		project, _ := c.Flags().GetString("project")
		r, e := a.Engine.Start(context.Background(), project, args[0])
		if e != nil {
			if strings.HasPrefix(e.Error(), "LIVE_STAGE_EXECUTION_UNAVAILABLE:") || strings.HasPrefix(e.Error(), "WORKSPACE_LOCKED:") || strings.HasPrefix(e.Error(), "PROFILE_NOT_FOUND:") || strings.HasPrefix(e.Error(), "PROFILE_PROVIDER_MISMATCH:") || strings.HasPrefix(e.Error(), "PROFILE_NOT_ALLOWED:") {
				return &CommandError{Code: ExitBlocked, Err: e}
			}
			return e
		}
		if r.State == model.Blocked {
			report, reportErr := a.Store.Report(context.Background(), r.ID)
			if reportErr != nil {
				return reportErr
			}
			fmt.Fprintf(commandOutput(c), "%s BLOCKED run: %s\n", report.Acceptance.Execution, r.ID)
			return runStateError(r.State)
		}
		fmt.Fprintf(commandOutput(c), "Configured workflow run started: %s\n", r.ID)
		result, e := a.Engine.Wait(context.Background(), r.ID)
		if e != nil {
			return e
		}
		report, e := a.Store.Report(context.Background(), result.ID)
		if e != nil {
			return e
		}
		if result.State != model.Completed {
			fmt.Fprintf(commandOutput(c), "%s run ended %s: %s\n", report.Acceptance.Execution, result.State, result.ID)
			return runStateError(result.State)
		}
		if report.Acceptance.Execution == "SIMULATED" {
			fmt.Fprintln(commandOutput(c), "SIMULATED run completed with fixture evidence")
		} else {
			fmt.Fprintf(commandOutput(c), "%s run completed with persisted evidence\n", report.Acceptance.Execution)
		}
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
		fmt.Fprintf(commandOutput(c), "Configured demo run started: %s\n", r.ID)
		result, e := a.Engine.Wait(context.Background(), r.ID)
		if e != nil {
			return e
		}
		report, e := a.Store.Report(context.Background(), result.ID)
		if e != nil {
			return e
		}
		if result.State != model.Completed {
			fmt.Fprintf(commandOutput(c), "%s demo run ended %s: %s\n", report.Acceptance.Execution, result.State, result.ID)
			return runStateError(result.State)
		}
		if report.Acceptance.Execution == "SIMULATED" {
			fmt.Fprintln(commandOutput(c), "SIMULATED run completed with fixture evidence")
		} else {
			fmt.Fprintf(commandOutput(c), "%s demo run completed with persisted evidence\n", report.Acceptance.Execution)
		}
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
		return writeJSON(commandOutput(c), v)
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
			return writeJSON(commandOutput(c), v)
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
	root.AddCommand(&cobra.Command{Use: "pause RUN_ID", Args: cobra.ExactArgs(1), Short: "Pause an active run at a safe stage boundary", RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		return a.Engine.Pause(args[0])
	}})
	root.AddCommand(&cobra.Command{Use: "resume RUN_ID", Args: cobra.ExactArgs(1), Short: "Resume a safely checkpointed run", RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		if e = a.Engine.Resume(args[0]); e != nil {
			return e
		}
		result, e := a.Engine.Wait(context.Background(), args[0])
		if e != nil {
			return e
		}
		if result.State != model.Completed {
			return runStateError(result.State)
		}
		fmt.Fprintf(commandOutput(c), "Run %s completed with durable evidence\n", result.ID)
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "dashboard", Short: "Open the verified dashboard for the shared workspace", RunE: func(c *cobra.Command, _ []string) error {
		a, err := open()
		if err != nil {
			return err
		}
		defer a.Close()
		url, err := a.EnsureDashboard(c.Context(), db)
		if err != nil {
			return err
		}
		fmt.Fprintf(commandOutput(c), "SpecTide dashboard ready: %s\n", url)
		return nil
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
		return writeJSON(commandOutput(c), v)
	}})
	projects := &cobra.Command{Use: "projects", Short: "List and register local projects", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		v, e := a.Store.Projects(c.Context())
		if e != nil {
			return e
		}
		for i := range v {
			v[i].Readiness = a.Projects.Readiness(c.Context(), v[i])
		}
		return writeJSON(commandOutput(c), v)
	}}
	projects.AddCommand(&cobra.Command{Use: "list", Short: "List registered local projects", RunE: projects.RunE})
	projects.AddCommand(&cobra.Command{Use: "register PATH", Args: cobra.ExactArgs(1), Short: "Register an existing local project directory", RunE: func(c *cobra.Command, args []string) error {
		abs, e := filepath.Abs(args[0])
		if e != nil {
			return e
		}
		canonical, e := filepath.EvalSymlinks(abs)
		if e != nil {
			return e
		}
		info, e := os.Stat(canonical)
		if e != nil || !info.IsDir() {
			return fmt.Errorf("project path must be an existing directory")
		}
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		project, _, e := a.Projects.Register(c.Context(), canonical, filepath.Base(canonical))
		if e != nil {
			return e
		}
		return writeJSON(commandOutput(c), project)
	}})
	root.AddCommand(projects)
	approvals := &cobra.Command{Use: "approvals", Short: "Manage durable run approvals"}
	approvals.AddCommand(&cobra.Command{Use: "list RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		v, e := a.Store.Approvals(c.Context(), args[0])
		if e != nil {
			return e
		}
		return writeJSON(commandOutput(c), v)
	}})
	approvals.AddCommand(&cobra.Command{Use: "create RUN_ID ACTION", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		v, e := a.Store.CreateApproval(c.Context(), model.Approval{RunID: args[0], Action: args[1]})
		if e != nil {
			return e
		}
		return writeJSON(commandOutput(c), v)
	}})
	approvals.AddCommand(&cobra.Command{Use: "resolve APPROVAL_ID APPROVED|DENIED", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		decision := strings.ToUpper(args[1])
		if e = a.Store.ResolveApproval(c.Context(), args[0], decision); e != nil {
			return e
		}
		return writeJSON(commandOutput(c), map[string]string{"approval_id": args[0], "decision": decision})
	}})
	root.AddCommand(approvals)
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
		return writeJSON(commandOutput(c), p)
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
		return writeJSON(commandOutput(c), p)
	}}
	add.Flags().String("provider", "", "provider name")
	add.Flags().String("executable", "", "local executable name or path")
	add.Flags().String("scope", "personal-public", "comma-separated allowed project scopes")
	profiles.AddCommand(add)
	profiles.AddCommand(&cobra.Command{Use: "probe NAME", Args: cobra.ExactArgs(1), Short: "Check provider status and capabilities without exposing credentials", RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		registered, e := a.Store.Profiles(c.Context())
		if e != nil {
			return e
		}
		for _, profile := range registered {
			if profile.Name != args[0] {
				continue
			}
			capability, probeErr := adapters.ProbeProvider(c.Context(), adapters.Profile{Provider: adapters.ProviderName(profile.Provider), Executable: profile.Executable, ConfigDir: profile.ConfigDirectory})
			if probeErr != nil {
				return probeErr
			}
			if _, probeErr = a.Store.SaveProfileCapability(c.Context(), profile.Name, persistedCapability(capability)); probeErr != nil {
				return probeErr
			}
			return writeJSON(commandOutput(c), capability)
		}
		return fmt.Errorf("PROFILE_NOT_FOUND: %s", args[0])
	}})
	root.AddCommand(profiles)
	models := &cobra.Command{Use: "models", Short: "Report local model discovery readiness", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		ps, e := a.Store.Profiles(context.Background())
		if e != nil {
			return e
		}
		type profileModels struct {
			Profile string   `json:"profile"`
			Status  string   `json:"catalog"`
			Models  []string `json:"models,omitempty"`
		}
		catalogs := []profileModels{}
		for _, p := range ps {
			status := "NOT_PROBED"
			var values []string
			if p.Capability != nil {
				status = p.Capability.ModelCatalogStatus
				values = p.Capability.ModelValues
			}
			catalogs = append(catalogs, profileModels{Profile: p.Name, Status: status, Models: values})
		}
		return writeJSON(commandOutput(c), map[string]any{"profiles": catalogs})
	}}
	models.AddCommand(&cobra.Command{Use: "probe PROFILE", Args: cobra.ExactArgs(1), Short: "Probe provider status and model catalog without exposing credentials", RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		registered, e := a.Store.Profiles(c.Context())
		if e != nil {
			return e
		}
		for _, profile := range registered {
			if profile.Name != args[0] {
				continue
			}
			capability, probeErr := adapters.ProbeProvider(c.Context(), adapters.Profile{Provider: adapters.ProviderName(profile.Provider), Executable: profile.Executable, ConfigDir: profile.ConfigDirectory})
			if probeErr != nil {
				return probeErr
			}
			if _, probeErr = a.Store.SaveProfileCapability(c.Context(), profile.Name, persistedCapability(capability)); probeErr != nil {
				return probeErr
			}
			return writeJSON(commandOutput(c), map[string]any{"profile": profile.Name, "executable_version": capability.Version, "authentication": capability.Authenticated, "model_catalog": capability.ModelCatalogStatus, "models": capability.ModelValues})
		}
		return fmt.Errorf("PROFILE_NOT_FOUND: %s", args[0])
	}})
	root.AddCommand(models)
	configCheck := func(c *cobra.Command) error {
		path, _ := c.Flags().GetString("file")
		project, _ := c.Flags().GetString("project")
		prompt, _ := c.Flags().GetString("prompt")
		cfg, e := config.LoadEffective(config.GlobalPath(), path)
		if e != nil {
			return e
		}
		state, e := open()
		if e != nil {
			return e
		}
		defer state.Close()
		profiles, e := state.Store.Profiles(c.Context())
		if e != nil {
			return e
		}
		decisions, validationErr := routing.ValidateLaunch(cfg, project, prompt, profiles)
		preview := map[string]any{"dry_run": true, "execution": "NOT_RUN", "mode": cfg.RoutingMode, "decisions": decisions}
		result := map[string]any{"valid": validationErr == nil, "preview": preview}
		if validationErr != nil {
			result["blocked_reasons"] = []string{validationErr.Error()}
		}
		if e = writeJSON(commandOutput(c), result); e != nil {
			return e
		}
		if validationErr != nil {
			return commandError(ExitBlocked, "ROUTE_PREVIEW_BLOCKED: "+validationErr.Error())
		}
		return nil
	}
	conf := &cobra.Command{Use: "config", Short: "Inspect or validate project configuration", RunE: func(c *cobra.Command, _ []string) error {
		path, _ := c.Flags().GetString("file")
		v, e := config.LoadEffective(config.GlobalPath(), path)
		if e != nil {
			return e
		}
		return writeJSON(commandOutput(c), v)
	}}
	conf.Flags().String("file", config.ProjectPath("."), "configuration file path")
	validate := &cobra.Command{Use: "validate", RunE: func(c *cobra.Command, _ []string) error { return configCheck(c) }}
	preview := &cobra.Command{Use: "preview", Short: "Preview launch-authorized routes without starting a run", RunE: func(c *cobra.Command, _ []string) error { return configCheck(c) }}
	for _, command := range []*cobra.Command{validate, preview} {
		command.Flags().String("file", config.ProjectPath("."), "configuration file path")
		command.Flags().String("project", ".", "workspace directory to authorize")
		command.Flags().String("prompt", "", "task intent for automatic risk routing")
		conf.AddCommand(command)
	}
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
		path, e := config.SafeProjectPath(abs)
		if e != nil {
			return e
		}
		v, e := config.LoadEffective(config.GlobalPath(), path)
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
		fmt.Fprintf(commandOutput(c), "Initialized %s\n", path)
		if _, e = os.Stat(filepath.Join(abs, ".specify")); e != nil {
			fmt.Fprintln(commandOutput(c), "Spec-Kit is not initialized here; use the installed `specify` CLI to initialize an authorized integration.")
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
		registry, e := quality.Discover(abs, nil)
		if e != nil {
			return e
		}
		results := registry.Run(c.Context(), abs)
		if e = writeJSON(commandOutput(c), results); e != nil {
			return e
		}
		if !registry.RequiredPassed(results) {
			return fmt.Errorf("QUALITY_CHECK_FAILED")
		}
		return nil
	}}
	verify.Flags().String("project", ".", "project directory to validate")
	root.AddCommand(verify)
	workflow := &cobra.Command{Use: "workflow", Short: "Inspect the installed canonical SpecTide workflow"}
	workflow.AddCommand(&cobra.Command{Use: "list", Short: "List canonical workflow stages", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		return writeJSON(commandOutput(c), map[string]any{"stages": config.StageKeys})
	}})
	workflow.AddCommand(&cobra.Command{Use: "explain", Short: "Explain stage ownership and execution boundaries", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		stages := make([]map[string]any, 0, len(config.StageKeys))
		for _, stage := range config.StageKeys {
			owner, readOnly := "provider", false
			switch stage {
			case "analyze", "final_review":
				readOnly = true
			case "tests", "report":
				owner = "local"
			}
			stages = append(stages, map[string]any{"stage": stage, "owner": owner, "read_only": readOnly})
		}
		return writeJSON(commandOutput(c), map[string]any{"stages": stages, "notes": []string{"analyze and final_review are read-only", "tests and report are local engine stages", "live provider stages require verified installed capabilities"}})
	}})
	root.AddCommand(workflow)
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

func runStateError(state model.RunState) error {
	switch state {
	case model.Blocked:
		return commandError(ExitBlocked, "run ended BLOCKED")
	case model.Failed:
		return commandError(ExitFailed, "run ended FAILED")
	case model.Cancelled:
		return commandError(ExitCancelled, "run ended CANCELLED")
	default:
		return commandError(ExitCommand, fmt.Sprintf("run ended %s", state))
	}
}

func netJoin(host string, port int) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}
