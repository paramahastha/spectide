package security

import (
	"os"
	"path/filepath"
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

func TestExecutableReferenceRejectsSymlinkReplacementAndContentDrift(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "provider-a")
	second := filepath.Join(t.TempDir(), "provider-b")
	link := filepath.Join(dir, "provider")
	for path, body := range map[string]string{first: "#!/bin/sh\necho version-a\n", second: "#!/bin/sh\necho version-b\n"} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	reference, err := ResolveExecutableReference(link)
	canonicalFirst, canonicalErr := filepath.EvalSymlinks(first)
	if err != nil || canonicalErr != nil || reference.Path != canonicalFirst || reference.SHA256 == "" {
		t.Fatalf("executable reference=%+v err=%v", reference, err)
	}
	policy := ProcessPolicy{Binary: link, Dir: dir, ExpectedExecutablePath: reference.Path, ExpectedExecutableSHA256: reference.SHA256}
	if err = ValidateProcess(policy); err != nil {
		t.Fatalf("initial approved executable rejected: %v", err)
	}
	if err = os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if err = ValidateProcess(policy); err == nil || !strings.Contains(err.Error(), "EXECUTABLE_REFERENCE_DRIFT") {
		t.Fatalf("executable symlink replacement accepted: %v", err)
	}
	if err = os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(link, []byte("#!/bin/sh\necho replaced\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy.Binary = link
	if err = ValidateProcess(policy); err == nil || !strings.Contains(err.Error(), "EXECUTABLE_REFERENCE_DRIFT") {
		t.Fatalf("binary content replacement accepted: %v", err)
	}
}

func TestExecutableReferenceRequiresExecutablePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(path, []byte("provider"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExecutableReference(path); err == nil || !strings.Contains(err.Error(), "EXECUTABLE_NOT_EXECUTABLE") {
		t.Fatalf("non-executable provider accepted: %v", err)
	}
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExecutableReference(path); err == nil || !strings.Contains(err.Error(), "EXECUTABLE_PERMISSIONS_UNSAFE") {
		t.Fatalf("world-writable provider accepted: %v", err)
	}
}

func TestConfigAndWorkingDirectoryReferencesRejectSymlinkRetargeting(t *testing.T) {
	root := t.TempDir()
	configA, configB := filepath.Join(root, "config-a"), filepath.Join(t.TempDir(), "config-b")
	workA, workB := filepath.Join(root, "work-a"), filepath.Join(root, "work-b")
	for _, path := range []string{configA, configB, workA, workB} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	configLink, workLink := filepath.Join(root, "config"), filepath.Join(root, "work")
	if err := os.Symlink(configA, configLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(workA, workLink); err != nil {
		t.Fatal(err)
	}
	approvedConfig, err := ResolveConfigDirectory(configLink)
	if err != nil {
		t.Fatal(err)
	}
	approvedWork, err := filepath.EvalSymlinks(workA)
	if err != nil {
		t.Fatal(err)
	}
	policy := ProcessPolicy{Binary: "/bin/echo", Dir: workLink, ExpectedWorkingDirectory: approvedWork, ExpectedConfigDirectory: approvedConfig, EnforceConfigDirectory: true, Env: []string{"CODEX_HOME=" + configLink}}
	if err = ValidateProcess(policy); err != nil {
		t.Fatalf("initial approved directories rejected: %v", err)
	}
	if err = os.Remove(configLink); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(configB, configLink); err != nil {
		t.Fatal(err)
	}
	if err = ValidateProcess(policy); err == nil || !strings.Contains(err.Error(), "CONFIG_DIRECTORY_REFERENCE_DRIFT") {
		t.Fatalf("configuration symlink retarget was accepted: %v", err)
	}
	if err = os.Remove(configLink); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(configA, configLink); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(workLink); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(workB, workLink); err != nil {
		t.Fatal(err)
	}
	if err = ValidateProcess(policy); err == nil || !strings.Contains(err.Error(), "WORKING_DIRECTORY_REFERENCE_DRIFT") {
		t.Fatalf("working-directory symlink retarget was accepted: %v", err)
	}
}

func TestWorkingDirectoryReferenceRejectsApprovedPathReplacement(t *testing.T) {
	parent := t.TempDir()
	approved := filepath.Join(parent, "approved")
	outside := t.TempDir()
	if err := os.Mkdir(approved, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(approved, filepath.Join(parent, "renamed-approved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, approved); err != nil {
		t.Fatal(err)
	}
	approvedCanonical, err := filepath.EvalSymlinks(filepath.Join(parent, "renamed-approved"))
	if err != nil {
		t.Fatal(err)
	}
	policy := ProcessPolicy{Binary: "/bin/echo", Dir: approved, ExpectedWorkingDirectory: approvedCanonical}
	if err := ValidateProcess(policy); err == nil || !strings.Contains(err.Error(), "WORKING_DIRECTORY_REFERENCE_DRIFT") {
		t.Fatalf("replacement workspace alias passed approval binding: %v", err)
	}
}
