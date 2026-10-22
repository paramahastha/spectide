package quality

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
	"github.com/spectide/spectide/internal/workspace"
)

func TestRunControlledCancelsRegisteredCheckFromSecondController(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controls.db")
	owner, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	controller, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	run, err := owner.CreateRun(ctx, t.TempDir(), "cancel long local check")
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "started")
	registry := NewRegistry(nil)
	if err = registry.Register(Check{Key: "long_check", Binary: "python3", Args: []string{"-c", "import pathlib,time; pathlib.Path(" + strconv.Quote(marker) + ").touch(); time.sleep(30)"}, Required: true, Timeout: 40 * time.Second}); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		evidence []Evidence
		err      error
	}, 1)
	started := time.Now()
	go func() {
		evidence, runErr := registry.RunControlled(ctx, run.Project, func(ctx context.Context) (string, error) { return owner.RunControl(ctx, run.ID) }, nil)
		result <- struct {
			evidence []Evidence
			err      error
		}{evidence, runErr}
	}()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("registered local check did not start")
		case <-ticker.C:
			if _, err = os.Stat(marker); err == nil {
				goto cancel
			}
		}
	}
cancel:
	if err = controller.RequestRunControl(ctx, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-result:
		if completed.err != nil {
			t.Fatalf("controlled execution err=%v", completed.err)
		}
		if len(completed.evidence) != 1 || completed.evidence[0].Verdict != Blocked || completed.evidence[0].ProcessID <= 0 {
			t.Fatalf("cancel evidence=%+v", completed.evidence)
		}
		if elapsed := time.Since(started); elapsed > 4*time.Second {
			t.Fatalf("cancel cleanup exceeded bound: %s", elapsed)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("local check process did not stop after cross-controller cancellation")
	}
}

func TestRegisteredCheckRetainsWorkspaceLockAfterOwnerDescriptorCloses(t *testing.T) {
	project := t.TempDir()
	lock, err := workspace.AcquireProcessLock(project)
	if err != nil {
		t.Fatal(err)
	}
	started := filepath.Join(t.TempDir(), "quality-child-started")
	registry := NewRegistry(nil)
	if err := registry.Register(Check{Key: "lock_child", Binary: "/bin/sh", Args: []string{"-c", "touch " + strconv.Quote(started) + "; sleep 1"}, Required: true, Timeout: 5 * time.Second}); err != nil {
		_ = lock.Close()
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, runErr := registry.RunControlled(context.Background(), project, nil, nil, lock.File())
		result <- runErr
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(started); err != nil {
		_ = lock.Close()
		t.Fatal("registered check did not start")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	held, err := workspace.ProcessLockHeld(project)
	if err != nil || !held {
		t.Fatalf("local quality child did not retain workspace lock: held=%v err=%v", held, err)
	}
	if _, err := workspace.AcquireProcessLock(project); err == nil {
		t.Fatal("second process acquired workspace while local quality child was still running")
	}
	select {
	case runErr := <-result:
		if runErr != nil {
			t.Fatalf("quality run err=%v", runErr)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("local quality child did not exit within bound")
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		held, err = workspace.ProcessLockHeld(project)
		if err == nil && !held {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("workspace lock remained held after local quality child exited")
}

func TestQualityCheckChildKeepsWorkspaceLockAfterControllerExits(t *testing.T) {
	if os.Getenv("SPECTIDE_QUALITY_LOCK_OWNER_HELPER") == "1" {
		project := os.Getenv("SPECTIDE_QUALITY_LOCK_PROJECT")
		started := os.Getenv("SPECTIDE_QUALITY_LOCK_STARTED")
		finished := os.Getenv("SPECTIDE_QUALITY_LOCK_FINISHED")
		lock, err := workspace.AcquireProcessLock(project)
		if err != nil {
			os.Exit(2)
		}
		registry := NewRegistry(nil)
		if err = registry.Register(Check{Key: "detached_check", Binary: "/bin/sh", Args: []string{"-c", "touch " + strconv.Quote(started) + "; sleep 1; touch " + strconv.Quote(finished)}, Required: true, Timeout: 5 * time.Second}); err != nil {
			os.Exit(3)
		}
		go func() {
			_, _ = registry.RunControlled(context.Background(), project, nil, nil, lock.File())
		}()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err = os.Stat(started); err == nil {
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(4)
	}

	project := t.TempDir()
	markers := t.TempDir()
	started, finished := filepath.Join(markers, "started"), filepath.Join(markers, "finished")
	command := exec.Command(os.Args[0], "-test.run=^TestQualityCheckChildKeepsWorkspaceLockAfterControllerExits$")
	command.Env = append(os.Environ(), "SPECTIDE_QUALITY_LOCK_OWNER_HELPER=1", "SPECTIDE_QUALITY_LOCK_PROJECT="+project, "SPECTIDE_QUALITY_LOCK_STARTED="+started, "SPECTIDE_QUALITY_LOCK_FINISHED="+finished)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if command.ProcessState != nil {
			t.Fatal("lock-owner helper exited before starting its quality child")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(started); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal("quality child did not start")
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("lock-owner helper did not exit after starting its child: %v", err)
	}
	held, err := workspace.ProcessLockHeld(project)
	if err != nil || !held {
		t.Fatalf("quality child did not retain lease after owner process exit: held=%v err=%v", held, err)
	}
	if _, err := workspace.AcquireProcessLock(project); err == nil {
		t.Fatal("second process acquired workspace while the detached quality child was running")
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(finished); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(finished); err != nil {
		t.Fatal("detached quality child did not finish")
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		held, err = workspace.ProcessLockHeld(project)
		if err == nil && !held {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("workspace lease remained after detached quality child exited")
}
