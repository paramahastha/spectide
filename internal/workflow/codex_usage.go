package workflow

import (
	"encoding/json"
	"regexp"
	"strings"
)

var safeResolvedModel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,119}$`)

// codexUsageEvidence keeps only bounded, typed fields from Codex's terminal
// JSON event. Raw provider output is never persisted in attempt evidence.
func codexUsageEvidence(raw []byte) map[string]any {
	var evidence map[string]any
	for _, line := range strings.Split(string(raw), "\n") {
		if len(line) == 0 || len(line) > adaptersMaxEventLine {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil || event["type"] != "turn.completed" {
			continue
		}
		candidate := map[string]any{}
		for _, key := range []string{"resolved_model", "model"} {
			if model, ok := event[key].(string); ok && safeResolvedModel.MatchString(model) {
				candidate["resolved_model"] = model
				break
			}
		}
		usage, _ := event["usage"].(map[string]any)
		for source, target := range map[string]string{"input_tokens": "input_tokens", "output_tokens": "output_tokens"} {
			if number, ok := usage[source].(float64); ok && number >= 0 && number <= 1e12 && number == float64(uint64(number)) {
				candidate[target] = uint64(number)
			}
		}
		if len(candidate) > 0 {
			evidence = candidate
		}
	}
	return evidence
}

const adaptersMaxEventLine = 1 << 20
