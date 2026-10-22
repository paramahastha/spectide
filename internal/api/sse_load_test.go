package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
)

type loadClientResult struct {
	lastSeq int64
	count   int
	err     error
}

// Run with SPECTIDE_RUN_SSE_LOAD=1 go test ./internal/api -run TestSSEModerateLoadAcceptance -count=1.
// The opt-in keeps the 100-second acceptance profile out of the fast unit suite.
func TestSSEModerateLoadAcceptance(t *testing.T) {
	if os.Getenv("SPECTIDE_RUN_SSE_LOAD") != "1" {
		t.Skip("set SPECTIDE_RUN_SSE_LOAD=1 to run the 100-second SSE acceptance profile")
	}
	const (
		clients       = 10
		eventsPerSec  = 100
		duration      = 100 * time.Second
		backlogEvents = 10_000
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := store.Open(t.TempDir() + "/sse-load.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, err := s.CreateRun(ctx, t.TempDir(), "SSE moderate-load acceptance")
	if err != nil {
		t.Fatal(err)
	}
	backlogTx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	backlogTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < backlogEvents; i++ {
		payload, marshalErr := json.Marshal(map[string]int{"index": i})
		if marshalErr != nil {
			backlogTx.Rollback()
			t.Fatal(marshalErr)
		}
		if _, err = backlogTx.ExecContext(ctx, `INSERT INTO events VALUES(?,?,?,?,?,?,?)`, run.ID, i+2, backlogTimestamp, "", "load.backlog", "info", string(payload)); err != nil {
			backlogTx.Rollback()
			t.Fatalf("persist backlog event %d: %v", i, err)
		}
	}
	if err = backlogTx.Commit(); err != nil {
		t.Fatal(err)
	}
	backlogHighWater, err := s.EventHighWater(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(s, nil).Handler())
	defer server.Close()

	backlogDrained := make(chan int, clients)
	results := make(chan loadClientResult, clients)
	latencies := make(chan time.Duration, clients*(backlogEvents+1+eventsPerSec*int(duration/time.Second)))
	liveLatencies := make(chan time.Duration, clients*eventsPerSec*int(duration/time.Second))
	client := &http.Client{Timeout: 2 * time.Minute}
	for i := 0; i < clients; i++ {
		response, getErr := client.Get(fmt.Sprintf("%s/api/v1/runs/%s/events?after=0", server.URL, run.ID))
		if getErr != nil {
			t.Fatalf("connect SSE client %d: %v", i, getErr)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Fatalf("client %d status=%d", i, response.StatusCode)
		}
		go readLoadEvents(response, backlogHighWater, eventsPerSec*int(duration/time.Second), backlogDrained, latencies, liveLatencies, results)
	}
	for i := 0; i < clients; i++ {
		select {
		case <-backlogDrained:
		case <-time.After(30 * time.Second):
			t.Fatal("clients did not drain the 10,000-event persisted backlog")
		}
	}
	t.Logf("all %d clients replayed %d persisted events", clients, backlogEvents)

	interval := time.Second / eventsPerSec
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	start := time.Now()
	for i := 0; i < eventsPerSec*int(duration/time.Second); i++ {
		<-ticker.C
		if err = s.Event(ctx, run.ID, "load", "load.live", "info", map[string]any{"index": i}); err != nil {
			t.Fatalf("persist live event %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < duration {
		t.Fatalf("event producer completed in %s, expected at least %s", elapsed, duration)
	}
	values := make([]time.Duration, 0, clients*eventsPerSec*int(duration/time.Second))
	allValues := make([]time.Duration, 0, clients*(backlogEvents+1+eventsPerSec*int(duration/time.Second)))
	for i := 0; i < clients; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("SSE client failed after %d events: %v", result.count, result.err)
			}
			if result.count != eventsPerSec*int(duration/time.Second) {
				t.Fatalf("SSE client received %d live events", result.count)
			}
			if result.lastSeq < backlogHighWater+int64(result.count) {
				t.Fatalf("SSE sequence ended at %d, backlog high-water=%d live=%d", result.lastSeq, backlogHighWater, result.count)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for SSE clients to receive live events")
		}
	}
	close(latencies)
	for latency := range latencies {
		allValues = append(allValues, latency)
	}
	close(liveLatencies)
	for latency := range liveLatencies {
		values = append(values, latency)
	}
	if len(values) != clients*eventsPerSec*int(duration/time.Second) {
		t.Fatalf("measured %d delivery latencies, expected %d", len(values), clients*eventsPerSec*int(duration/time.Second))
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	sort.Slice(allValues, func(i, j int) bool { return allValues[i] < allValues[j] })
	liveP95 := values[(len(values)*95+99)/100-1]
	allP95 := allValues[(len(allValues)*95+99)/100-1]
	t.Logf("profile: clients=%d events_per_second=%d duration=%s persisted_backlog=%d", clients, eventsPerSec, elapsed, backlogEvents)
	t.Logf("all non-heartbeat frames delivered=%d, p95 persisted-timestamp-to-SSE-receipt=%s", len(allValues), allP95)
	t.Logf("live frames delivered=%d, p95 persisted-timestamp-to-SSE-receipt=%s", len(values), liveP95)
	if len(allValues) != clients*(backlogEvents+1+eventsPerSec*int(duration/time.Second)) {
		t.Fatalf("measured %d total frame latencies, expected %d", len(allValues), clients*(backlogEvents+1+eventsPerSec*int(duration/time.Second)))
	}
	if allP95 > time.Second {
		t.Fatalf("SC-006 all-frame p95=%s exceeds 1s", allP95)
	}
}

func readLoadEvents(response *http.Response, backlogHighWater int64, expectedLive int, backlogDrained chan<- int, latencies, liveLatencies chan<- time.Duration, results chan<- loadClientResult) {
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var currentID int64
	var lastID int64
	count := 0
	backlogSent := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			currentID, _ = strconv.ParseInt(strings.TrimPrefix(line, "id: "), 10, 64)
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event model.Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			results <- loadClientResult{currentID, count, err}
			return
		}
		if currentID != event.Seq || currentID != lastID+1 {
			results <- loadClientResult{currentID, count, fmt.Errorf("missing or duplicate SSE event: last=%d id=%d data_seq=%d", lastID, currentID, event.Seq)}
			return
		}
		lastID = currentID
		latencies <- time.Since(event.Timestamp)
		if event.Seq <= backlogHighWater {
			if event.Seq == backlogHighWater && !backlogSent {
				backlogSent = true
				backlogDrained <- 1
			}
			continue
		}
		if event.Type == "load.live" {
			liveLatencies <- time.Since(event.Timestamp)
			count++
			if count == expectedLive {
				results <- loadClientResult{currentID, count, nil}
				return
			}
		}
	}
	err := scanner.Err()
	if err == nil {
		err = fmt.Errorf("SSE stream ended before all live events arrived")
	}
	results <- loadClientResult{currentID, count, err}
}
