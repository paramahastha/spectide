package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/speckit"
)

func TestWorkspaceOwnerRemediationRejectsSourceAndContractSideEffects(t *testing.T) {
	before := map[string]stageFile{
		"specs/001-product/spec.md":          {SHA256: "spec-before"},
		"specs/001-product/contracts/api.md": {SHA256: "contract-before"},
		"internal/service.go":                {SHA256: "source-before"},
	}
	ownerOnly := map[string]stageFile{
		"specs/001-product/spec.md":          {SHA256: "spec-after"},
		"specs/001-product/contracts/api.md": {SHA256: "contract-before"},
		"internal/service.go":                {SHA256: "source-before"},
	}
	if err := validateWorkspaceOwnerChange(before, ownerOnly, "specs/001-product/spec.md"); err != nil {
		t.Fatalf("owner-only workspace edit rejected: %v", err)
	}
	for name, after := range map[string]map[string]stageFile{
		"source mutation": {
			"specs/001-product/spec.md":          {SHA256: "spec-after"},
			"specs/001-product/contracts/api.md": {SHA256: "contract-before"},
			"internal/service.go":                {SHA256: "source-after"},
		},
		"contract mutation": {
			"specs/001-product/spec.md":          {SHA256: "spec-after"},
			"specs/001-product/contracts/api.md": {SHA256: "contract-after"},
			"internal/service.go":                {SHA256: "source-before"},
		},
		"new unrelated file": {
			"specs/001-product/spec.md":          {SHA256: "spec-after"},
			"specs/001-product/contracts/api.md": {SHA256: "contract-before"},
			"internal/service.go":                {SHA256: "source-before"},
			"README.md":                          {SHA256: "new"},
		},
		"deleted contract": {
			"specs/001-product/spec.md": {SHA256: "spec-after"},
			"internal/service.go":       {SHA256: "source-before"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateWorkspaceOwnerChange(before, after, "specs/001-product/spec.md"); err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_CHANGED_NON_OWNER_ARTIFACT") {
				t.Fatalf("non-owner workspace mutation accepted: %v", err)
			}
		})
	}
}

func TestAnalysisOwnerMayUseOnlyPlanReferencedDesignContracts(t *testing.T) {
	project := t.TempDir()
	feature := filepath.Join(project, "specs", "001-product")
	contract := filepath.Join(feature, "contracts", "api.md")
	if err := os.MkdirAll(filepath.Dir(contract), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(feature, "plan.md"), []byte("API decision: [contract](contracts/api.md)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contract, []byte("# API contract\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	finding := speckit.Finding{ID: "A1", Category: "API contract", Location: "contracts/api.md:12", Severity: "HIGH", Summary: "Contract response is underspecified", Resolution: "Define response per FR-020"}
	owner, template, err := resolveAnalysisOwner(project, feature, finding)
	want := filepath.ToSlash(filepath.Join("specs", "001-product", "contracts", "api.md"))
	if err != nil || owner != want || template != "plan" {
		t.Fatalf("contract owner=%q template=%q err=%v want=%q", owner, template, err, want)
	}
	if err = os.WriteFile(filepath.Join(feature, "plan.md"), []byte("No contract reference here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = resolveAnalysisOwner(project, feature, finding); err == nil || !strings.Contains(err.Error(), "ANALYSIS_CONTRACT_NOT_REFERENCED_BY_PLAN") {
		t.Fatalf("unreferenced contract was accepted as owner: %v", err)
	}
	if err = os.WriteFile(filepath.Join(feature, "plan.md"), []byte("contracts/api.md is referenced\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(contract); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(t.TempDir(), "outside.md"), contract); err != nil {
		t.Fatal(err)
	}
	if _, _, err = resolveAnalysisOwner(project, feature, finding); err == nil {
		t.Fatal("contract symlink escape was accepted as owner")
	}
}

func TestWorkspaceFingerprintDetectsSourceAndDesignContractMutation(t *testing.T) {
	root := t.TempDir()
	contract := filepath.Join(root, "specs", "001-product", "contracts", "api.md")
	if err := os.MkdirAll(filepath.Dir(contract), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contract, []byte("contract v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := workspaceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "service.go"), []byte("package example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := workspaceFingerprint(root)
	if err != nil || before == after {
		t.Fatalf("workspace fingerprint missed non-canonical source write: before=%s after=%s err=%v", before, after, err)
	}
	before, _ = workspaceFingerprint(root)
	if err = os.WriteFile(contract, []byte("contract v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err = workspaceFingerprint(root)
	if err != nil || before == after {
		t.Fatalf("workspace fingerprint missed design-contract write: before=%s after=%s err=%v", before, after, err)
	}
}
