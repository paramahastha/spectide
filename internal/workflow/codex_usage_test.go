package workflow

import (
	"reflect"
	"testing"
)

func TestCodexUsageEvidenceKeepsOnlySafeTerminalFields(t *testing.T) {
	raw := []byte(`{"type":"turn.started","model":"ignored"}` + "\n" +
		`{"type":"turn.completed","model":"openai/gpt-5.2-codex","usage":{"input_tokens":42,"output_tokens":7,"cached_input_tokens":11},"account_email":"private@example.com"}` + "\n")
	want := map[string]any{"resolved_model": "openai/gpt-5.2-codex", "input_tokens": uint64(42), "output_tokens": uint64(7)}
	if got := codexUsageEvidence(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("usage evidence=%#v, want %#v", got, want)
	}
}

func TestCodexUsageEvidenceOmitsAbsentAndUnsafeValues(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"type":"turn.completed","usage":{}}`),
		[]byte(`{"type":"turn.completed","model":"token=secret","usage":{"input_tokens":-1,"output_tokens":1.5}}`),
	} {
		if got := codexUsageEvidence(raw); len(got) != 0 {
			t.Errorf("unsafe/partial usage=%#v", got)
		}
	}
}
