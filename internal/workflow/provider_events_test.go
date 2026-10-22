package workflow

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/store"
)

func TestProviderEventPublisherPersistsBeforeStageClose(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	run, err := state.CreateRun(ctx, t.TempDir(), "streaming provider event")
	if err != nil {
		t.Fatal(err)
	}
	publisher := newProviderEventPublisher(state, run.ID, "plan", "codex", "attempt-123")
	publisher.Publish(adapters.Event{Type: "codex.message", Summary: "structured provider event"})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events, listErr := state.Events(ctx, run.ID, 0)
		if listErr != nil {
			t.Fatal(listErr)
		}
		for _, event := range events {
			if event.Type != "provider.event" {
				continue
			}
			if event.Payload["attempt_id"] != "attempt-123" || event.Payload["event_index"] != float64(1) {
				t.Fatalf("provider event lost attempt/order linkage: %+v", event)
			}
			if err = publisher.Close(); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = publisher.Close()
	t.Fatal("provider event was not persisted until the stage publisher closed")
}
