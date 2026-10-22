package fake

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/adapters"
)

var _ adapters.Adapter = Adapter{}

func TestScriptedFakeAdapterEmitsOnlyLabeledSummaries(t *testing.T) {
	a := Adapter{Script: []ScriptStep{{Type: "message", Summary: "completed"}, {Summary: "api_key=abcdef0123456789"}}}
	events, err := a.Execute(context.Background())
	if err != nil || len(events) != 2 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	if events[0].Type != "fake.message" || events[0].Summary != "SIMULATED: completed" || events[1].Summary == "" {
		t.Fatalf("unexpected fake events: %+v", events)
	}
	for _, event := range events {
		if event.Raw != nil || strings.Contains(event.Summary, "api_key=abcdef0123456789") {
			t.Fatalf("raw or secret-bearing event escaped: %+v", event)
		}
	}
}

func TestScriptedFakeAdapterReturnsTypedFailure(t *testing.T) {
	_, err := (Adapter{Script: []ScriptStep{{Type: "started"}, {FailureCode: "SCRIPTED_FAILURE", FailureMessage: "fixture failed"}}}).Execute(context.Background())
	var providerErr *adapters.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "SCRIPTED_FAILURE" {
		t.Fatalf("failure=%T %v", err, err)
	}
}

func TestScriptedFakeAdapterReturnsRateLimitMetadata(t *testing.T) {
	_, err := (Adapter{Script: []ScriptStep{{RetryAfter: 250 * time.Millisecond}}}).Execute(context.Background())
	var providerErr *adapters.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "RATE_LIMITED" || providerErr.RetryAfter != 250*time.Millisecond {
		t.Fatalf("rate limit=%+v err=%v", providerErr, err)
	}
}

func TestScriptedFakeAdapterHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := (Adapter{Script: []ScriptStep{{Delay: time.Hour}}}).Execute(ctx)
	var providerErr *adapters.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "RUN_CANCELLED" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation=%T %v", err, err)
	}
}

func TestCheckedInScenariosDriveFakeAdapter(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "testdata", "fake-provider", "scenarios.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string][]struct {
		DelayMS        int64  `json:"delay_ms"`
		RetryAfterMS   int64  `json:"retry_after_ms"`
		Type           string `json:"type"`
		Summary        string `json:"summary"`
		FailureCode    string `json:"failure_code"`
		FailureMessage string `json:"failure_message"`
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	toScript := func(name string) []ScriptStep {
		t.Helper()
		fixture, ok := fixtures[name]
		if !ok {
			t.Fatalf("fixture %q missing", name)
		}
		steps := make([]ScriptStep, 0, len(fixture))
		for _, step := range fixture {
			steps = append(steps, ScriptStep{Delay: time.Duration(step.DelayMS) * time.Millisecond, RetryAfter: time.Duration(step.RetryAfterMS) * time.Millisecond, Type: step.Type, Summary: step.Summary, FailureCode: step.FailureCode, FailureMessage: step.FailureMessage})
		}
		return steps
	}
	if events, err := (Adapter{Script: toScript("success")}).Execute(context.Background()); err != nil || len(events) != 2 {
		t.Fatalf("success events=%+v err=%v", events, err)
	}
	if _, err := (Adapter{Script: toScript("failure")}).Execute(context.Background()); err == nil || !strings.Contains(err.Error(), "SCRIPTED_FAILURE") {
		t.Fatalf("failure fixture error=%v", err)
	}
	if _, err := (Adapter{Script: toScript("rate_limit")}).Execute(context.Background()); err == nil || !strings.Contains(err.Error(), "RATE_LIMITED") {
		t.Fatalf("rate-limit fixture error=%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := (Adapter{Script: toScript("cancellation")}).Execute(ctx); err == nil || !strings.Contains(err.Error(), "RUN_CANCELLED") {
		t.Fatalf("cancellation fixture error=%v", err)
	}
}
