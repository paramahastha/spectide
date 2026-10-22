package security

import (
	"strings"
	"testing"
	"time"
)

func TestValidateProcessPolicy(t *testing.T) {
	base := ProcessPolicy{Binary: "/usr/bin/agent", Dir: "/workspace", Timeout: time.Minute, MinTime: time.Second, MaxTime: time.Hour}
	if err := ValidateProcess(base); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	tests := []struct {
		name string
		edit func(*ProcessPolicy)
		want string
	}{
		{"relative executable with path", func(p *ProcessPolicy) { p.Binary = "./agent" }, "PERMISSION_DENIED"},
		{"relative directory", func(p *ProcessPolicy) { p.Dir = "workspace" }, "PERMISSION_DENIED"},
		{"unapproved environment", func(p *ProcessPolicy) { p.Env = []string{"API_TOKEN=secret"} }, "PROCESS_ENV_NOT_ALLOWED"},
		{"malformed environment", func(p *ProcessPolicy) { p.Env = []string{"CODEX_HOME"} }, "INVALID_PROCESS_ENV"},
		{"timeout below minimum", func(p *ProcessPolicy) { p.Timeout = time.Millisecond }, "INVALID_STAGE_TIMEOUT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := base
			tt.edit(&policy)
			if err := ValidateProcess(policy); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want %s", err, tt.want)
			}
		})
	}
}
