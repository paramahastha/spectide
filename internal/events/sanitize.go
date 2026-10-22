// Package events sanitizes persisted event payloads before they enter storage,
// replay, or a live stream.
package events

import "github.com/spectide/spectide/internal/security"

// SanitizePayload recursively redacts credential-like values and sensitive
// field names while preserving the JSON-compatible shape of event data.
func SanitizePayload(value any) any {
	switch x := value.(type) {
	case string:
		return security.Redact(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, value := range x {
			if sensitiveKey(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = SanitizePayload(value)
			}
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(x))
		for key, value := range x {
			if sensitiveKey(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = security.Redact(value)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = SanitizePayload(value)
		}
		return out
	case []string:
		out := make([]string, len(x))
		for i, value := range x {
			out[i] = security.Redact(value)
		}
		return out
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	key = security.NormalizeKey(key)
	switch key {
	case "token", "apikey", "authorization", "secret", "password", "credential":
		return true
	default:
		return false
	}
}
