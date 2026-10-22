// Package runner launches provider processes with argument vectors, not shells.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/security"
)

type Invocation struct {
	Binary    string
	Args, Env []string
	Dir       string
	Stdin     []byte
}
type Result struct {
	Stdout, Stderr string
	ExitCode       int
	Duration       time.Duration
}

const maxOutputBytes = 1 << 20

// cappedBuffer bounds memory use while continuing to drain the child pipe.
type cappedBuffer struct {
	b         bytes.Buffer
	remaining int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if c.remaining > 0 {
		keep := len(p)
		if keep > c.remaining {
			keep = c.remaining
		}
		_, _ = c.b.Write(p[:keep])
		c.remaining -= keep
	}
	return n, nil
}
func (c *cappedBuffer) String() string {
	s := c.b.String()
	if c.remaining == 0 {
		s += "\n[SpecTide: output truncated at 1 MiB]"
	}
	return s
}

func Run(ctx context.Context, in Invocation) (Result, error) {
	if strings.TrimSpace(in.Binary) == "" {
		return Result{}, fmt.Errorf("EXECUTABLE_NOT_FOUND: empty executable")
	}
	if filepath.Base(in.Binary) != in.Binary && !filepath.IsAbs(in.Binary) {
		return Result{}, fmt.Errorf("PERMISSION_DENIED: executable must be a command name or absolute path")
	}
	if in.Dir != "" && !filepath.IsAbs(in.Dir) {
		return Result{}, fmt.Errorf("PERMISSION_DENIED: working directory must be absolute")
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, in.Binary, in.Args...)
	cmd.Dir = in.Dir
	home, _ := os.UserHomeDir()
	cmd.Env = []string{"PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + os.TempDir()}
	for _, value := range in.Env {
		name, _, ok := strings.Cut(value, "=")
		if !ok {
			return Result{}, fmt.Errorf("INVALID_PROCESS_ENV: expected NAME=value")
		}
		switch name {
		case "CLAUDE_CONFIG_DIR", "CODEX_HOME", "OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR":
		default:
			return Result{}, fmt.Errorf("PROCESS_ENV_NOT_ALLOWED: %s", name)
		}
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Stdin = bytes.NewReader(in.Stdin)
	out, errOut := &cappedBuffer{remaining: maxOutputBytes}, &cappedBuffer{remaining: maxOutputBytes}
	cmd.Stdout = out
	cmd.Stderr = errOut
	err := cmd.Run()
	res := Result{Stdout: security.Redact(out.String()), Stderr: security.Redact(errOut.String()), Duration: time.Since(start)}
	if exit, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exit.ExitCode()
		return res, fmt.Errorf("provider exited with %d", res.ExitCode)
	}
	if err != nil {
		return res, err
	}
	return res, nil
}
