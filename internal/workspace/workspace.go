// Package workspace validates local roots and provides an in-process writer lease.
package workspace

import (
	"fmt"
	"path/filepath"
	"sync"
)

type Locker struct {
	mu      sync.Mutex
	holders map[string]string
}

func NewLocker() *Locker { return &Locker{holders: map[string]string{}} }
func Canonical(path string, roots []string) (string, error) {
	p, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", e
	}
	p, e = filepath.Abs(p)
	if e != nil {
		return "", e
	}
	for _, root := range roots {
		r, er := filepath.EvalSymlinks(root)
		if er != nil {
			continue
		}
		rel, er := filepath.Rel(r, p)
		if er == nil && rel != ".." && len(rel) >= 0 && (len(rel) < 3 || rel[:3] != ".."+string(filepath.Separator)) {
			return p, nil
		}
	}
	return "", fmt.Errorf("PERMISSION_DENIED: workspace escapes allowed roots")
}
func (l *Locker) Acquire(path, runID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if holder := l.holders[path]; holder != "" && holder != runID {
		return fmt.Errorf("WORKSPACE_LOCKED: %s", holder)
	}
	l.holders[path] = runID
	return nil
}
func (l *Locker) Release(path, runID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holders[path] == runID {
		delete(l.holders, path)
	}
}
