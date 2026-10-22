package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

func TestProcessLockIsSharedAndInheritedByLiveProviderChild(t *testing.T) {
	if os.Getenv("SPECTIDE_PROCESS_LOCK_HELPER") == "1" {
		file := os.NewFile(3, "workspace-lock")
		if file == nil {
			os.Exit(2)
		}
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}
	project := t.TempDir()
	lock, err := AcquireProcessLock(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AcquireProcessLock(project); err == nil {
		t.Fatal("same workspace lock was acquired twice")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProcessLockIsSharedAndInheritedByLiveProviderChild$")
	command.Env = append(os.Environ(), "SPECTIDE_PROCESS_LOCK_HELPER=1")
	command.ExtraFiles = []*os.File{lock.File()}
	if err = command.Start(); err != nil {
		_ = lock.Close()
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	held, err := ProcessLockHeld(project)
	if err != nil || !held {
		t.Fatalf("provider child did not retain the workspace lock: held=%v err=%v", held, err)
	}
	if err = command.Wait(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		held, err = ProcessLockHeld(project)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("workspace lock remained held after provider child exited")
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
