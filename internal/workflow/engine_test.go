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
			if len(rep.Stages) != 12 || len(rep.Gates) != 5 {
				t.Fatalf("stages=%d gates=%d", len(rep.Stages), len(rep.Gates))
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("workflow did not complete")
}
