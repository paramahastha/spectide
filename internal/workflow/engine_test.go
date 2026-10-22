package workflow

import (
	"context"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
	"path/filepath"
	"testing"
	"time"
)

func TestFakeLifecyclePersistsEvidence(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "spectide.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	eng := New(s)
	r, e := eng.Start(context.Background(), "demo", "Build a test")
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := s.GetRun(context.Background(), r.ID)
		if got.State == model.Completed {
			rep, e := s.Report(context.Background(), r.ID)
			if e != nil {
				t.Fatal(e)
			}
			if len(rep.Stages) != 12 || len(rep.Decisions) != 12 || len(rep.Gates) != 5 {
				t.Fatalf("stages=%d decisions=%d gates=%d", len(rep.Stages), len(rep.Decisions), len(rep.Gates))
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("workflow did not complete")
}

func TestOnlyOneActiveWriterPerWorkspace(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "spectide.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	eng := New(s)
	first, e := eng.Start(context.Background(), "same-workspace", "first")
	if e != nil {
		t.Fatal(e)
	}
	second, e := eng.Start(context.Background(), "same-workspace", "second")
	if e != nil {
		t.Fatal(e)
	}
	if first.ID == second.ID {
		t.Fatal("runs share an ID")
	}
	got, e := s.GetRun(context.Background(), second.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.State != model.Blocked {
		t.Fatalf("second run state=%s", got.State)
	}
	if _, e = eng.Wait(context.Background(), first.ID); e != nil {
		t.Fatal(e)
	}
	third, e := eng.Start(context.Background(), "same-workspace", "third")
	if e != nil {
		t.Fatal(e)
	}
	got, e = s.GetRun(context.Background(), third.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.State == model.Blocked {
		t.Fatal("workspace lock was not released after completion")
	}
}
