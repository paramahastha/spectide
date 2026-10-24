// Package speckit discovers installed Spec-Kit integrations without pretending
// that agent instruction files are standalone shell commands.
package speckit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Installation struct {
	Binary        string
	ProjectRoot   string
	Integration   string
	Verified      bool
	Commands      map[string]string
	CommandFiles  map[string]string
	CommandHashes map[string]string
}

func Discover(root string) (Installation, error) {
	abs, e := filepath.Abs(root)
	if e != nil {
		return Installation{}, e
	}
	abs, e = filepath.EvalSymlinks(abs)
	if e != nil {
		return Installation{}, fmt.Errorf("SPEC_KIT_PROJECT_ROOT_INVALID: %w", e)
	}
	info, e := os.Stat(filepath.Join(abs, ".specify"))
	if e != nil || !info.IsDir() {
		return Installation{}, fmt.Errorf("SPEC_KIT_NOT_INITIALIZED: .specify is missing")
	}
	bin, e := exec.LookPath("specify")
	if e != nil {
		return Installation{ProjectRoot: abs}, fmt.Errorf("EXECUTABLE_NOT_FOUND: specify")
	}
	integration, commands, files, hashes, err := discoverCommandFiles(abs)
	if err != nil {
		return Installation{Binary: bin, ProjectRoot: abs}, err
	}
	return Installation{Binary: bin, ProjectRoot: abs, Integration: integration, Verified: len(commands) > 0, Commands: commands, CommandFiles: files, CommandHashes: hashes}, nil
}

func discoverCommandFiles(abs string) (string, map[string]string, map[string]string, map[string]string, error) {
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, nil, nil, err
	}
	abs = canonical
	commands := map[string]string{}
	files := map[string]string{}
	hashes := map[string]string{}
	integration := ""
	manifestDir := filepath.Join(abs, ".specify", "integrations")
	manifests, _ := filepath.Glob(filepath.Join(manifestDir, "*.manifest.json"))
	sort.Strings(manifests)
	for _, manifestPath := range manifests {
		b, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest struct {
			Integration string            `json:"integration"`
			Files       map[string]string `json:"files"`
		}
		if json.Unmarshal(b, &manifest) != nil {
			continue
		}
		rels := make([]string, 0, len(manifest.Files))
		for rel := range manifest.Files {
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		for _, rel := range rels {
			stage, ok := stageFromPath(rel)
			if !ok {
				continue
			}
			if filepath.IsAbs(rel) || filepath.Clean(filepath.FromSlash(rel)) != filepath.FromSlash(rel) {
				return "", nil, nil, nil, fmt.Errorf("SPEC_KIT_MANIFEST_PATH_INVALID: %s", rel)
			}
			target := filepath.Join(abs, filepath.FromSlash(rel))
			resolved, resolveErr := filepath.EvalSymlinks(target)
			if resolveErr != nil {
				if os.IsNotExist(resolveErr) {
					continue
				}
				return "", nil, nil, nil, resolveErr
			}
			inside, relErr := filepath.Rel(abs, resolved)
			if relErr != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
				return "", nil, nil, nil, fmt.Errorf("PERMISSION_DENIED: Spec-Kit command template escapes project root")
			}
			content, readErr := os.ReadFile(resolved)
			if readErr != nil {
				return "", nil, nil, nil, readErr
			}
			want, decodeErr := hex.DecodeString(manifest.Files[rel])
			if decodeErr != nil || len(want) != sha256.Size {
				return "", nil, nil, nil, fmt.Errorf("SPEC_KIT_MANIFEST_HASH_INVALID: %s", rel)
			}
			got := sha256.Sum256(content)
			if !equalBytes(want, got[:]) {
				return "", nil, nil, nil, fmt.Errorf("SPEC_KIT_COMMAND_INTEGRITY_MISMATCH: %s", rel)
			}
			commands[stage] = "/speckit." + stage
			files[stage] = rel
			hashes[stage] = manifest.Files[rel]
			if integration == "" {
				integration = manifest.Integration
			}
		}
	}
	if integration == "" {
		integration = "unknown"
	}
	return integration, commands, files, hashes, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
func RequireStage(in Installation, stage string) (string, error) {
	cmd := in.Commands[stage]
	if cmd == "" {
		return "", fmt.Errorf("SPEC_KIT_STAGE_UNAVAILABLE: %s", stage)
	}
	return cmd, nil
}

func stageFromPath(rel string) (string, bool) {
	known := []string{"constitution", "specify", "clarify", "plan", "checklist", "tasks", "analyze", "implement", "converge"}
	s := filepath.ToSlash(rel)
	for _, stage := range known {
		if strings.HasSuffix(s, "/speckit."+stage+".md") || strings.HasSuffix(s, "/speckit-"+stage+"/SKILL.md") {
			return stage, true
		}
	}
	return "", false
}

// StagePrompt expands an installed official command template as a prompt. It
// never executes markdown or treats workspace command files as shell scripts.
func StagePrompt(in Installation, stage, arguments string) (string, error) {
	if _, err := RequireStage(in, stage); err != nil {
		return "", err
	}
	rel := in.CommandFiles[stage]
	if rel == "" {
		return "", fmt.Errorf("SPEC_KIT_COMMAND_TEMPLATE_UNAVAILABLE: %s", stage)
	}
	root, err := filepath.EvalSymlinks(in.ProjectRoot)
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	r, err := filepath.Rel(root, target)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("PERMISSION_DENIED: Spec-Kit command template escapes project root")
	}
	b, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	if in.Verified {
		want, decodeErr := hex.DecodeString(in.CommandHashes[stage])
		got := sha256.Sum256(b)
		if decodeErr != nil || len(want) != sha256.Size || !equalBytes(want, got[:]) {
			return "", fmt.Errorf("SPEC_KIT_COMMAND_INTEGRITY_MISMATCH: %s", rel)
		}
	}
	body := string(b)
	if strings.HasPrefix(body, "---") {
		if i := strings.Index(body[3:], "\n---"); i >= 0 {
			body = body[3+i+4:]
			body = strings.TrimLeft(body, "\r\n")
		}
	}
	if strings.Contains(body, "$ARGUMENTS") {
		body = strings.ReplaceAll(body, "$ARGUMENTS", arguments)
	} else if strings.TrimSpace(arguments) != "" {
		body += "\n\nUser task: " + arguments
	}
	return body, nil
}
