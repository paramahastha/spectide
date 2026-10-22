//go:build darwin || linux

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ProcessLock is a kernel-managed exclusive lease for one canonical workspace.
// Unlike the SQLite row, it is shared by processes using different databases
// and is released automatically when the last inherited descriptor closes.
type ProcessLock struct {
	file *os.File
}

func processLockPath(canonical string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "spectide", "workspace-locks")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("WORKSPACE_LOCK_DIRECTORY_UNSAFE")
	}
	hash := sha256.Sum256([]byte(filepath.Clean(canonical)))
	return filepath.Join(dir, hex.EncodeToString(hash[:])+".lock"), nil
}

func openProcessLock(canonical string) (*os.File, error) {
	path, err := processLockPath(canonical)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// AcquireProcessLock acquires the canonical-workspace lease without waiting.
func AcquireProcessLock(canonical string) (*ProcessLock, error) {
	file, err := openProcessLock(canonical)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("WORKSPACE_LOCKED: another process owns the canonical workspace")
		}
		return nil, err
	}
	return &ProcessLock{file: file}, nil
}

// ProcessLockHeld reports whether another process currently holds the lease.
func ProcessLockHeld(canonical string) (bool, error) {
	file, err := openProcessLock(canonical)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if err = unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		return false, err
	}
	return false, nil
}

// File returns the descriptor that a provider child inherits. A live child
// therefore keeps the same workspace lease if the SpecTide process exits.
func (l *ProcessLock) File() *os.File {
	if l == nil {
		return nil
	}
	return l.file
}

func (l *ProcessLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	// Closing only this descriptor preserves the lock while a provider child
	// still owns an inherited duplicate of the same open file description.
	closeErr := l.file.Close()
	l.file = nil
	return closeErr
}
