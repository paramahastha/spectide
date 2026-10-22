package workflow

import (
	"strings"
	"testing"
)

func TestValidateOwnerArtifactChangeRequiresProgressOnlyInOwner(t *testing.T) {
	before := map[string]string{"spec.md": "s0", "plan.md": "p0", "tasks.md": "t0"}
	valid := map[string]string{"spec.md": "s1", "plan.md": "p0", "tasks.md": "t0"}
	if err := validateOwnerArtifactChange(before, valid, "spec.md"); err != nil {
		t.Fatalf("owner-only change rejected: %v", err)
	}
	if err := validateOwnerArtifactChange(before, before, "spec.md"); err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_NO_PROGRESS") {
		t.Fatalf("unchanged owner was accepted: %v", err)
	}
	changedOther := map[string]string{"spec.md": "s1", "plan.md": "p1", "tasks.md": "t0"}
	if err := validateOwnerArtifactChange(before, changedOther, "spec.md"); err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_CHANGED_NON_OWNER_ARTIFACT") {
		t.Fatalf("non-owner change was accepted: %v", err)
	}
}
