//go:build !darwin && !linux

package workspace

import (
	"fmt"
	"os"
)

// ProcessLock is unavailable on platforms where SpecTide has no verified
// kernel file-lock implementation; execution fails closed there.
type ProcessLock struct{}

func AcquireProcessLock(string) (*ProcessLock, error) {
	return nil, fmt.Errorf("WORKSPACE_PROCESS_LOCK_UNSUPPORTED")
}

func ProcessLockHeld(string) (bool, error) {
	return false, fmt.Errorf("WORKSPACE_PROCESS_LOCK_UNSUPPORTED")
}
func (*ProcessLock) File() *os.File { return nil }
func (*ProcessLock) Close() error   { return nil }
