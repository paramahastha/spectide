package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
)

type derivedQualityCheck struct {
	Key               string   `json:"key"`
	Binary            string   `json:"binary"`
	Args              []string `json:"args"`
	Required          bool     `json:"required"`
	RequireEmptyOut   bool     `json:"require_empty_output"`
	TimeoutSeconds    int      `json:"timeout_seconds"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
}

func (e *Engine) qualityRegistryForRun(ctx context.Context, run model.Run, effective effectiveRunConfiguration) (*quality.Registry, error) {
	if effective.QualitySelectionPolicy != "DISCOVER_AFTER_IMPLEMENTATION" {
		return effective.qualityRegistry()
	}
	manifestHash, err := qualityManifestHash(run.Project)
	if err != nil {
		return nil, err
	}
	events, err := e.store.Events(ctx, run.ID, 0)
	if err != nil {
		return nil, err
	}
	var frozen []derivedQualityCheck
	for _, event := range events {
		if event.Type != "quality.checks.derived" {
			continue
		}
		if event.Payload["manifest_sha256"] != manifestHash {
			return nil, fmt.Errorf("QUALITY_CHECK_MANIFEST_DRIFT")
		}
		encoded, marshalErr := json.Marshal(event.Payload["checks"])
		if marshalErr != nil || json.Unmarshal(encoded, &frozen) != nil || len(frozen) == 0 {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: frozen derived quality checks are invalid")
		}
	}
	if len(frozen) == 0 {
		registry, discoverErr := quality.Discover(run.Project, nil)
		if discoverErr != nil {
			return nil, discoverErr
		}
		checks := make([]derivedQualityCheck, 0, len(registry.Checks()))
		for _, check := range registry.Checks() {
			checks = append(checks, derivedQualityCheck{Key: check.Key, Binary: check.Binary, Args: append([]string(nil), check.Args...), Required: check.Required, RequireEmptyOut: check.RequireEmptyOut, TimeoutSeconds: int(check.Timeout / time.Second), UnavailableReason: check.UnavailableReason})
		}
		sort.Slice(checks, func(i, j int) bool { return checks[i].Key < checks[j].Key })
		if err = e.store.Event(ctx, run.ID, "tests", "quality.checks.derived", "info", map[string]any{
			"provenance":      "workspace project manifests discovered after implementation",
			"manifest_sha256": manifestHash, "checks": checks,
		}); err != nil {
			return nil, err
		}
		frozen = checks
	}
	registry := quality.NewRegistry(nil)
	for _, check := range frozen {
		if check.TimeoutSeconds < 1 {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: derived quality check timeout is invalid")
		}
		if err = registry.Register(quality.Check{Key: check.Key, Binary: check.Binary, Args: append([]string(nil), check.Args...), Required: check.Required, RequireEmptyOut: check.RequireEmptyOut, Timeout: time.Duration(check.TimeoutSeconds) * time.Second, UnavailableReason: check.UnavailableReason}); err != nil {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: derived quality check is invalid")
		}
	}
	return registry, nil
}

func qualityManifestHash(projectRoot string) (string, error) {
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", err
	}
	paths := []string{"go.mod", "go.work", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "pyproject.toml", "pytest.ini", "requirements.txt", "setup.cfg", "tox.ini"}
	var material strings.Builder
	for _, relative := range paths {
		absolute := filepath.Join(root, relative)
		info, statErr := os.Lstat(absolute)
		if os.IsNotExist(statErr) {
			material.WriteString(relative + "=ABSENT\n")
			continue
		}
		if statErr != nil {
			return "", statErr
		}
		resolved, resolveErr := filepath.EvalSymlinks(absolute)
		if resolveErr != nil {
			return "", resolveErr
		}
		rel, relErr := filepath.Rel(root, resolved)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !info.Mode().IsRegular() {
			return "", fmt.Errorf("QUALITY_CHECK_MANIFEST_PATH_INVALID")
		}
		data, readErr := os.ReadFile(resolved)
		if readErr != nil {
			return "", readErr
		}
		digest := sha256.Sum256(data)
		material.WriteString(filepath.ToSlash(relative) + "=" + hex.EncodeToString(digest[:]) + "\n")
	}
	digest := sha256.Sum256([]byte(material.String()))
	return hex.EncodeToString(digest[:]), nil
}
