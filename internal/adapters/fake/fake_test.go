package fake

import (
	"context"
	"errors"
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
