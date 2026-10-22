package workspace

import (
	"path/filepath"
	"testing"
)

func TestLockerExcludesOtherRun(t *testing.T) {
	l := NewLocker()
	if e := l.Acquire("/a", "one"); e != nil {
		t.Fatal(e)
	}
	if e := l.Acquire("/a", "two"); e == nil {
		t.Fatal("expected conflict")
	}
	l.Release("/a", "one")
	if e := l.Acquire("/a", "two"); e != nil {
		t.Fatal(e)
	}
}
func TestCanonicalRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	if _, e := Canonical(other, []string{root}); e == nil {
		t.Fatal("expected denial")
	}
	if _, e := Canonical(filepath.Clean(root), []string{root}); e != nil {
		t.Fatal(e)
	}
}
