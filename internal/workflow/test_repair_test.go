package workflow

import (
	"context"
	"strings"
	"testing"
)

func TestRunTestRepairGateAllowsBaselineAndRepairsUntilPass(t *testing.T) {
	t.Run("baseline passes", func(t *testing.T) {
		repairs := 0
		got, err := RunTestRepairGate(context.Background(), TestRepairHooks{
			Test:   func(context.Context) (map[string]string, error) { return map[string]string{"unit": "PASS"}, nil },
			Repair: func(context.Context, map[string]string) error { repairs++; return nil },
		}, DefaultLoopLimits())
		if err != nil || got["unit"] != "PASS" || repairs != 0 {
			t.Fatalf("verdicts=%v err=%v repairs=%d", got, err, repairs)
		}
	})
	t.Run("repair passes", func(t *testing.T) {
		runs, repairs := 0, 0
		got, err := RunTestRepairGate(context.Background(), TestRepairHooks{
			Test: func(context.Context) (map[string]string, error) {
				runs++
				if runs == 1 {
					return map[string]string{"unit": "FAIL"}, nil
				}
				return map[string]string{"unit": "PASS"}, nil
			},
			Repair: func(_ context.Context, failed map[string]string) error {
				repairs++
				if failed["unit"] != "FAIL" {
					t.Fatalf("repair received verdicts %v", failed)
				}
				return nil
			},
		}, DefaultLoopLimits())
		if err != nil || got["unit"] != "PASS" || runs != 2 || repairs != 1 {
			t.Fatalf("verdicts=%v err=%v runs=%d repairs=%d", got, err, runs, repairs)
		}
	})
}

func TestRunTestRepairGateStopsOnNoProgressAndAfterFourRepairs(t *testing.T) {
	t.Run("no progress", func(t *testing.T) {
		runs := 0
		_, err := RunTestRepairGate(context.Background(), TestRepairHooks{
			Test: func(context.Context) (map[string]string, error) {
				runs++
				return map[string]string{"unit": "FAIL"}, nil
			},
			Repair: func(context.Context, map[string]string) error { return nil },
		}, DefaultLoopLimits())
		if err == nil || !strings.Contains(err.Error(), "TEST_REPAIR_NO_PROGRESS") || runs != 2 {
			t.Fatalf("err=%v runs=%d", err, runs)
		}
	})
	t.Run("four repairs", func(t *testing.T) {
		runs, repairs := 0, 0
		_, err := RunTestRepairGate(context.Background(), TestRepairHooks{
			Test: func(context.Context) (map[string]string, error) {
				runs++
				keys := []string{"unit", "lint", "unit", "lint", "unit"}
				return map[string]string{keys[runs-1]: "FAIL"}, nil
			},
			Repair: func(context.Context, map[string]string) error { repairs++; return nil },
		}, DefaultLoopLimits())
		if err == nil || !strings.Contains(err.Error(), "TEST_REPAIR_EXHAUSTED") || repairs != 4 || runs != 5 {
			t.Fatalf("err=%v runs=%d repairs=%d", err, runs, repairs)
		}
	})
}

func TestRunTestRepairGateDoesNotRepairUnavailableQualityChecks(t *testing.T) {
	repairs := 0
	_, err := RunTestRepairGate(context.Background(), TestRepairHooks{Test: func(context.Context) (map[string]string, error) { return map[string]string{"unit": "BLOCKED"}, nil }, Repair: func(context.Context, map[string]string) error { repairs++; return nil }}, DefaultLoopLimits())
	if err == nil || !strings.Contains(err.Error(), "REQUIRED_TEST_CHECK_BLOCKED") || repairs != 0 {
		t.Fatalf("err=%v repairs=%d", err, repairs)
	}
}
