package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ProcessPolicy is the complete allowlist for a locally launched provider
// process. Values are validated before exec receives them.
type ProcessPolicy struct {
	Binary                   string
	Dir                      string
	Env                      []string
	Timeout                  time.Duration
	MinTime                  time.Duration
	MaxTime                  time.Duration
	ExpectedExecutablePath   string
	ExpectedExecutableSHA256 string
	ExpectedWorkingDirectory string
	ExpectedConfigDirectory  string
	EnforceConfigDirectory   bool
}

type ExecutableReference struct {
	Path   string
	SHA256 string
}

// ResolveExecutableReference resolves command names and symlinks to an
// absolute regular executable and hashes its bytes for capability binding.
func ResolveExecutableReference(binary string) (ExecutableReference, error) {
	if strings.TrimSpace(binary) == "" {
		return ExecutableReference{}, fmt.Errorf("EXECUTABLE_NOT_FOUND: empty executable")
	}
	resolved := binary
	if !filepath.IsAbs(resolved) {
		var err error
		resolved, err = exec.LookPath(binary)
		if err != nil {
			return ExecutableReference{}, fmt.Errorf("EXECUTABLE_NOT_FOUND: %w", err)
		}
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return ExecutableReference{}, err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return ExecutableReference{}, fmt.Errorf("EXECUTABLE_REFERENCE_INVALID: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return ExecutableReference{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return ExecutableReference{}, fmt.Errorf("EXECUTABLE_NOT_EXECUTABLE")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return ExecutableReference{}, fmt.Errorf("EXECUTABLE_PERMISSIONS_UNSAFE")
	}
	file, err := os.Open(canonical)
	if err != nil {
		return ExecutableReference{}, err
	}
	defer file.Close()
	h := sha256.New()
	if _, err = io.Copy(h, file); err != nil {
		return ExecutableReference{}, err
	}
	return ExecutableReference{Path: canonical, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// ResolveConfigDirectory establishes a canonical, existing directory path.
// It only inspects directory metadata; it never reads provider credentials.
func ResolveConfigDirectory(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("CONFIG_DIRECTORY_MUST_BE_ABSOLUTE")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("CONFIG_DIRECTORY_INVALID: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("CONFIG_DIRECTORY_INVALID: not a directory")
	}
	return filepath.Clean(canonical), nil
}

// ValidateProcess rejects executable, directory, environment, and timeout
// values outside SpecTide's constrained subprocess boundary.
func ValidateProcess(p ProcessPolicy) error {
	if strings.TrimSpace(p.Binary) == "" {
		return fmt.Errorf("EXECUTABLE_NOT_FOUND: empty executable")
	}
	if filepath.Base(p.Binary) != p.Binary && !filepath.IsAbs(p.Binary) {
		return fmt.Errorf("PERMISSION_DENIED: executable must be a command name or absolute path")
	}
	if p.Dir != "" && !filepath.IsAbs(p.Dir) {
		return fmt.Errorf("PERMISSION_DENIED: working directory must be absolute")
	}
	if p.ExpectedExecutablePath != "" {
		reference, err := ResolveExecutableReference(p.Binary)
		if err != nil {
			return err
		}
		if reference.Path != p.ExpectedExecutablePath || reference.SHA256 != p.ExpectedExecutableSHA256 {
			return fmt.Errorf("EXECUTABLE_REFERENCE_DRIFT")
		}
	}
	if p.ExpectedWorkingDirectory != "" {
		workingDirectory, err := filepath.EvalSymlinks(p.Dir)
		if err != nil {
			return fmt.Errorf("WORKING_DIRECTORY_INVALID: %w", err)
		}
		expected, err := filepath.Abs(p.ExpectedWorkingDirectory)
		if err != nil || filepath.Clean(workingDirectory) != filepath.Clean(expected) {
			return fmt.Errorf("WORKING_DIRECTORY_REFERENCE_DRIFT")
		}
	}
	if p.EnforceConfigDirectory {
		configDirectory, err := ResolveConfigDirectory(p.ExpectedConfigDirectory)
		if err != nil {
			return err
		}
		configured := ""
		for _, value := range p.Env {
			name, path, ok := strings.Cut(value, "=")
			if !ok {
				continue
			}
			switch name {
			case "CLAUDE_CONFIG_DIR", "CODEX_HOME", "OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR":
				resolved, resolveErr := ResolveConfigDirectory(path)
				if resolveErr != nil || resolved != configDirectory || configured != "" {
					return fmt.Errorf("CONFIG_DIRECTORY_REFERENCE_DRIFT")
				}
				configured = resolved
			}
		}
		if configured != configDirectory {
			return fmt.Errorf("CONFIG_DIRECTORY_REFERENCE_DRIFT")
		}
	}
	for _, value := range p.Env {
		name, _, ok := strings.Cut(value, "=")
		if !ok || name == "" {
			return fmt.Errorf("INVALID_PROCESS_ENV: expected NAME=value")
		}
		switch name {
		case "CLAUDE_CONFIG_DIR", "CODEX_HOME", "OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR":
		default:
			return fmt.Errorf("PROCESS_ENV_NOT_ALLOWED: %s", name)
		}
	}
	if p.Timeout != 0 && (p.Timeout < p.MinTime || p.Timeout > p.MaxTime) {
		return fmt.Errorf("INVALID_STAGE_TIMEOUT: duration must be between %s and %s", p.MinTime, p.MaxTime)
	}
	return nil
}
