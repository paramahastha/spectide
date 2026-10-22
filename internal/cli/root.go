package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/spectide/spectide/internal/app"
	"github.com/spectide/spectide/internal/model"
	"github.com/spf13/cobra"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func Root() *cobra.Command {
	var db string
	root := &cobra.Command{Use: "spectide", Short: "Local, evidence-backed Spec-Kit orchestration", SilenceUsage: true}
	root.PersistentFlags().StringVar(&db, "db", "", "SQLite state database path")
	open := func() (*app.App, error) { return app.Open(db) }
	root.AddCommand(&cobra.Command{Use: "version", Run: func(c *cobra.Command, _ []string) { fmt.Fprintln(c.OutOrStdout(), "spectide 0.1.0-dev") }})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Report local provider readiness", RunE: func(c *cobra.Command, _ []string) error {
		out := map[string]any{"database": app.DefaultDBPath(), "providers": map[string]string{"fake": "SIMULATED_READY", "claude": probe("claude"), "codex": probe("codex"), "opencode": probe("opencode")}}
		return json.NewEncoder(c.OutOrStdout()).Encode(out)
	}})
	serve := &cobra.Command{Use: "serve", Short: "Serve loopback dashboard and API", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		host, _ := c.Flags().GetString("host")
		port, _ := c.Flags().GetInt("port")
		if host != "127.0.0.1" && host != "localhost" {
			return fmt.Errorf("only loopback host is allowed")
		}
		addr := host + ":" + strconv.Itoa(port)
		fmt.Fprintf(c.OutOrStdout(), "SpecTide listening at http://%s\n", addr)
		return http.ListenAndServe(addr, a.API.Handler())
	}}
	serve.Flags().String("host", "127.0.0.1", "loopback host")
	serve.Flags().Int("port", 4747, "loopback port")
	root.AddCommand(serve)
	run := &cobra.Command{Use: "run PROMPT", Args: cobra.ExactArgs(1), Short: "Start a durable simulated workflow", RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		project, _ := c.Flags().GetString("project")
		r, e := a.Engine.Start(context.Background(), project, args[0])
		if e == nil {
			fmt.Fprintln(c.OutOrStdout(), r.ID)
			_, e = a.Engine.Wait(context.Background(), r.ID)
		}
		return e
	}}
	run.Flags().String("project", ".", "registered local project label")
	root.AddCommand(run)
	demo := &cobra.Command{Use: "demo", Short: "Run the offline simulated demo", RunE: func(c *cobra.Command, _ []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		project, _ := c.Flags().GetString("project")
		r, e := a.Engine.Start(context.Background(), project, "Run deterministic offline SpecTide verification")
		if e == nil {
			fmt.Fprintf(c.OutOrStdout(), "SIMULATED run started: %s\n", r.ID)
			_, e = a.Engine.Wait(context.Background(), r.ID)
			if e == nil {
				fmt.Fprintln(c.OutOrStdout(), "SIMULATED run completed with fixture evidence")
			}
		}
		return e
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
	root.AddCommand(&cobra.Command{Use: "status RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
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
	root.AddCommand(&cobra.Command{Use: "report RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
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
	root.AddCommand(&cobra.Command{Use: "cancel RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		return a.Engine.Cancel(context.Background(), args[0])
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
	profiles := &cobra.Command{Use: "profiles", Short: "List configured sanitized profiles", Run: func(c *cobra.Command, _ []string) {
		a, e := open()
		if e != nil {
			fmt.Fprintln(c.ErrOrStderr(), e)
			return
		}
		defer a.Close()
		profiles, e := a.Store.Profiles(context.Background())
		if e != nil {
			fmt.Fprintln(c.ErrOrStderr(), e)
			return
		}
		_ = json.NewEncoder(c.OutOrStdout()).Encode(profiles)
	}}
	addProfile := &cobra.Command{Use: "add NAME", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		a, e := open()
		if e != nil {
			return e
		}
		defer a.Close()
		provider, _ := c.Flags().GetString("provider")
		executable, _ := c.Flags().GetString("executable")
		scope, _ := c.Flags().GetString("scope")
		p, e := a.Store.SaveProfile(context.Background(), model.Profile{Name: args[0], Provider: provider, Executable: executable, AllowedScopes: []string{scope}})
		if e != nil {
			return e
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(p)
	}}
	addProfile.Flags().String("provider", "", "provider name")
	addProfile.Flags().String("executable", "", "local executable")
	addProfile.Flags().String("scope", "personal-public", "allowed project scope")
	profiles.AddCommand(addProfile)
	root.AddCommand(profiles)
	root.AddCommand(&cobra.Command{Use: "models", Short: "Model discovery requires a configured provider profile", Run: func(c *cobra.Command, _ []string) {
		fmt.Fprintln(c.OutOrStdout(), "No provider model catalog has been probed.")
	}})
	root.AddCommand(&cobra.Command{Use: "config", Short: "Configuration management is pending", Run: func(c *cobra.Command, _ []string) {
		fmt.Fprintln(c.OutOrStdout(), "Configuration is not yet persisted.")
	}})
	root.AddCommand(&cobra.Command{Use: "init", Short: "Initialize a local project", Run: func(c *cobra.Command, _ []string) {
		fmt.Fprintln(c.OutOrStdout(), "SpecTide project initialization is pending Spec-Kit discovery support.")
	}})
	_ = os.Stdout
	_ = strings.Builder{}
	_ = time.Second
	return root
}
func probe(bin string) string {
	if _, e := exec.LookPath(bin); e != nil {
		return "NOT_FOUND"
	}
	return "FOUND_NOT_AUTHENTICATED"
}
