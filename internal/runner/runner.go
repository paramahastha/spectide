// Package runner launches provider processes with argument vectors, not shells.
package runner

import (
	"bytes"
	"context"
	"errors"
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
	Timeout   time.Duration
}
type Result struct {
	Stdout, Stderr string
	ExitCode       int
	Duration       time.Duration
}

const maxOutputBytes = 1 << 20

const (
	defaultStageTimeout = 30 * time.Minute
	minStageTimeout     = time.Second
	maxStageTimeout     = 4 * time.Hour
	terminationGrace    = 2 * time.Second
)

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
	timeout := in.Timeout
	if timeout == 0 {
		timeout = defaultStageTimeout
	}
	if timeout < minStageTimeout || timeout > maxStageTimeout {
		return Result{}, fmt.Errorf("INVALID_STAGE_TIMEOUT: duration must be between %s and %s", minStageTimeout, maxStageTimeout)
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, in.Binary, in.Args...)
	configureProcessGroup(cmd)
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
	stopEscalation := make(chan struct{})
	cmd.Cancel = func() error {
		if err := terminateProcessGroup(cmd); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		go func() {
			timer := time.NewTimer(terminationGrace)
			defer timer.Stop()
			select {
			case <-timer.C:
				_ = killProcessGroup(cmd)
			case <-stopEscalation:
			}
		}()
		return nil
	}
	cmd.WaitDelay = terminationGrace
	out, errOut := &cappedBuffer{remaining: maxOutputBytes}, &cappedBuffer{remaining: maxOutputBytes}
	cmd.Stdout = out
	cmd.Stderr = errOut
	err := cmd.Run()
	close(stopEscalation)
	res := Result{Stdout: security.Redact(out.String()), Stderr: security.Redact(errOut.String()), Duration: time.Since(start)}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && parentCtx.Err() == nil {
			return res, fmt.Errorf("STAGE_TIMEOUT: provider stage exceeded %s", timeout)
		}
		return res, fmt.Errorf("RUN_CANCELLED: provider stage was cancelled")
	}
	if exit, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exit.ExitCode()
		return res, fmt.Errorf("provider exited with %d", res.ExitCode)
	}
	if err != nil {
		return res, err
	}
	return res, nil
}
