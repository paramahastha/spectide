package quality

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
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
