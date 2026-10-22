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
	"sync"
	"sync/atomic"
	"time"

	"github.com/spectide/spectide/internal/security"
)

type Invocation struct {
	Binary                   string
	Args, Env                []string
	ExtraFiles               []*os.File
	Dir                      string
	Stdin                    []byte
	Timeout                  time.Duration
	ExpectedExecutablePath   string
	ExpectedExecutableSHA256 string
	ExpectedWorkingDirectory string
	ExpectedConfigDirectory  string
	EnforceConfigDirectory   bool
	StreamStdout             func([]byte) error
}
type Result struct {
	Stdout, Stderr  string
	StdoutTruncated bool
	ExitCode        int
	ProcessID       int
	StartedAt       time.Time
	FinishedAt      time.Time
	Duration        time.Duration
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

type streamingBuffer struct {
	buffer    *cappedBuffer
	callback  func([]byte) error
	streamErr error
	truncated bool
}

func (s *streamingBuffer) Write(p []byte) (int, error) {
	kept := len(p)
	if kept > s.buffer.remaining {
		kept = s.buffer.remaining
	}
	if kept < len(p) {
		s.truncated = true
	}
	_, _ = s.buffer.Write(p)
	if kept > 0 && s.callback != nil && s.streamErr == nil {
		if err := s.callback(append([]byte(nil), p[:kept]...)); err != nil {
			s.streamErr = err
		}
	}
	return len(p), nil
}

func Run(ctx context.Context, in Invocation) (Result, error) {
	policy := security.ProcessPolicy{Binary: in.Binary, Dir: in.Dir, Env: in.Env, Timeout: in.Timeout, MinTime: minStageTimeout, MaxTime: maxStageTimeout, ExpectedExecutablePath: in.ExpectedExecutablePath, ExpectedExecutableSHA256: in.ExpectedExecutableSHA256, ExpectedWorkingDirectory: in.ExpectedWorkingDirectory, ExpectedConfigDirectory: in.ExpectedConfigDirectory, EnforceConfigDirectory: in.EnforceConfigDirectory}
	if err := security.ValidateProcess(policy); err != nil {
		return Result{}, err
	}
	executable := in.Binary
	if in.ExpectedExecutablePath != "" {
		reference, err := security.ResolveExecutableReference(in.Binary)
		if err != nil || reference.Path != in.ExpectedExecutablePath || reference.SHA256 != in.ExpectedExecutableSHA256 {
			return Result{}, fmt.Errorf("EXECUTABLE_REFERENCE_DRIFT")
		}
		executable = reference.Path
	}
	workingDirectory := in.Dir
	if in.ExpectedWorkingDirectory != "" {
		resolved, err := filepath.EvalSymlinks(in.Dir)
		if err != nil {
			return Result{}, fmt.Errorf("WORKING_DIRECTORY_INVALID: %w", err)
		}
		expected, err := filepath.EvalSymlinks(in.ExpectedWorkingDirectory)
		if err != nil || filepath.Clean(resolved) != filepath.Clean(expected) {
			return Result{}, fmt.Errorf("WORKING_DIRECTORY_REFERENCE_DRIFT")
		}
		workingDirectory = resolved
	}
	timeout := in.Timeout
	if timeout == 0 {
		timeout = defaultStageTimeout
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, executable, in.Args...)
	cmd.ExtraFiles = in.ExtraFiles
	configureProcessGroup(cmd)
	cmd.Dir = workingDirectory
	home, _ := os.UserHomeDir()
	cmd.Env = []string{"PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + os.TempDir()}
	for _, value := range in.Env {
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Stdin = bytes.NewReader(in.Stdin)
	var escalationOnce sync.Once
	var escalationStarted atomic.Bool
	escalationDone := make(chan struct{})
	requestTermination := func() error {
		if err := terminateProcessGroup(cmd); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		escalationOnce.Do(func() {
			escalationStarted.Store(true)
			go func() {
				defer close(escalationDone)
				timer := time.NewTimer(terminationGrace)
				defer timer.Stop()
				<-timer.C
				// The process-group leader may already have exited while a child
				// still owns the inherited pipes. Keep escalation alive until the
				// grace period expires, independent of cmd.Run's return time.
				_ = killProcessGroup(cmd)
			}()
		})
		return nil
	}
	cmd.Cancel = requestTermination
	cmd.WaitDelay = terminationGrace
	out, errOut := &cappedBuffer{remaining: maxOutputBytes}, &cappedBuffer{remaining: maxOutputBytes}
	streamOut := &streamingBuffer{buffer: out, callback: in.StreamStdout}
	cmd.Stdout = streamOut
	cmd.Stderr = errOut
	err := cmd.Run()
	if ctx.Err() != nil || errors.Is(err, exec.ErrWaitDelay) {
		// CommandContext may stop watching once the group leader exits, even
		// though descendants still hold its pipes. Reassert termination after
		// WaitDelay detects that the command group outlived the leader.
		_ = requestTermination()
	}
	if escalationStarted.Load() {
		<-escalationDone
	}
	finished := time.Now()
	res := Result{Stdout: security.Redact(out.String()), Stderr: security.Redact(errOut.String()), StdoutTruncated: streamOut.truncated, StartedAt: start.UTC(), FinishedAt: finished.UTC(), Duration: finished.Sub(start)}
	if cmd.Process != nil {
		res.ProcessID = cmd.Process.Pid
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && parentCtx.Err() == nil {
			return res, fmt.Errorf("STAGE_TIMEOUT: provider stage exceeded %s", timeout)
		}
		return res, fmt.Errorf("RUN_CANCELLED: provider stage was cancelled")
	}
	if streamOut.streamErr != nil {
		return res, fmt.Errorf("PROVIDER_STREAM_INVALID")
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
