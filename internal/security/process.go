package security

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ProcessPolicy is the complete allowlist for a locally launched provider
// process. Values are validated before exec receives them.
type ProcessPolicy struct {
	Binary  string
	Dir     string
	Env     []string
	Timeout time.Duration
	MinTime time.Duration
	MaxTime time.Duration
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
