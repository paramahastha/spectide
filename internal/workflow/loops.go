package workflow

import "fmt"

// LoopLimits defines the per-run remediation bounds from FR-009. The initial
// baseline analysis is not counted as a remediation round.
type LoopLimits struct {
	AnalyzeRemediations int
	TestRepairs         int
	ConvergenceRounds   int
}

func DefaultLoopLimits() LoopLimits {
	return LoopLimits{AnalyzeRemediations: 3, TestRepairs: 4, ConvergenceRounds: 5}
}

type LoopBudget struct {
	limits              LoopLimits
	analyzeRemediations int
	testRepairs         int
	convergenceRounds   int
}

func NewLoopBudget(limits LoopLimits) (*LoopBudget, error) {
	if limits.AnalyzeRemediations < 1 || limits.TestRepairs < 1 || limits.ConvergenceRounds < 1 {
		return nil, fmt.Errorf("INVALID_LOOP_LIMITS: all per-run limits must be positive")
	}
	return &LoopBudget{limits: limits}, nil
}

// AnalysisRemediation records one owner-artifact edit followed by a fresh
// read-only analysis. It rejects an unchanged artifact before spending budget.
func (b *LoopBudget) AnalysisRemediation(previousArtifactHash, currentArtifactHash string) error {
	if previousArtifactHash == "" || currentArtifactHash == "" || previousArtifactHash == currentArtifactHash {
		return fmt.Errorf("ANALYZE_REMEDIATION_NO_PROGRESS")
	}
	if b.analyzeRemediations >= b.limits.AnalyzeRemediations {
		return fmt.Errorf("ANALYZE_REMEDIATION_EXHAUSTED")
	}
	b.analyzeRemediations++
	return nil
}

func (b *LoopBudget) TestRepair() error {
	if b.testRepairs >= b.limits.TestRepairs {
		return fmt.Errorf("TEST_REPAIR_EXHAUSTED")
	}
	b.testRepairs++
	return nil
}

// ConvergenceRound records a round only when valid task additions change the
// task-ledger hash or required gate verdicts make progress.
func (b *LoopBudget) ConvergenceRound(previousTaskHash, currentTaskHash string, previousGates, currentGates map[string]string) error {
	if previousTaskHash == currentTaskHash && equalGateVerdicts(previousGates, currentGates) {
		return fmt.Errorf("CONVERGENCE_NO_PROGRESS")
	}
	if b.convergenceRounds >= b.limits.ConvergenceRounds {
		return fmt.Errorf("CONVERGENCE_EXHAUSTED")
	}
	b.convergenceRounds++
	return nil
}

func (b *LoopBudget) Counts() LoopLimits {
	return LoopLimits{AnalyzeRemediations: b.analyzeRemediations, TestRepairs: b.testRepairs, ConvergenceRounds: b.convergenceRounds}
}

func (b *LoopBudget) Limits() LoopLimits { return b.limits }

func equalGateVerdicts(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, verdict := range a {
		if b[key] != verdict {
			return false
		}
	}
	return true
}
