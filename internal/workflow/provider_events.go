package workflow

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/store"
)

const providerEventQueueSize = 128

type providerEventPublisher struct {
	state     *store.Store
	runID     string
	stage     string
	provider  string
	attemptID string
	queue     chan adapters.Event
	done      chan struct{}
	dropped   atomic.Int64
	mu        sync.Mutex
	err       error
}

func newProviderEventPublisher(state *store.Store, runID, stage, provider, attemptID string) *providerEventPublisher {
	publisher := &providerEventPublisher{state: state, runID: runID, stage: stage, provider: provider, attemptID: attemptID, queue: make(chan adapters.Event, providerEventQueueSize), done: make(chan struct{})}
	go publisher.run()
	return publisher
}

// Publish is called on the provider output reader. It never waits for storage
// or a viewer; a saturated queue drops the event and reports the count later.
func (p *providerEventPublisher) Publish(event adapters.Event) {
	select {
	case p.queue <- event:
	default:
		p.dropped.Add(1)
	}
}

func (p *providerEventPublisher) run() {
	defer close(p.done)
	index := 0
	for event := range p.queue {
		index++
		err := p.state.Event(context.Background(), p.runID, p.stage, "provider.event", "info", map[string]any{
			"provider": p.provider, "provider_event_type": safeProviderEventType(event.Type),
			"summary": "structured provider event", "execution": "LIVE",
			"attempt_id": p.attemptID, "event_index": index,
		})
		if err != nil {
			p.mu.Lock()
			if p.err == nil {
				p.err = err
			}
			p.mu.Unlock()
		}
	}
	if count := p.dropped.Load(); count > 0 {
		if err := p.state.Event(context.Background(), p.runID, p.stage, "provider.events.dropped", "warning", map[string]any{
			"provider": p.provider, "attempt_id": p.attemptID, "dropped_count": count, "execution": "LIVE",
		}); err != nil {
			p.mu.Lock()
			if p.err == nil {
				p.err = err
			}
			p.mu.Unlock()
		}
	}
}

func (p *providerEventPublisher) Close() error {
	close(p.queue)
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}
