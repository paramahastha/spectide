package events

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizePayloadRedactsNestedCredentialsAndVariants(t *testing.T) {
	payload := map[string]any{
		"message": "Authorization: Bearer top-secret-value",
		"nested": map[string]string{
			"api-key": "private-value",
			"status":  "token=embedded-secret",
		},
		"items": []string{"password=another-secret", "safe"},
	}
	encoded, err := json.Marshal(SanitizePayload(payload))
	if err != nil {
		t.Fatal(err)
	}
	result := string(encoded)
	for _, secret := range []string{"top-secret-value", "private-value", "embedded-secret", "another-secret"} {
		if strings.Contains(result, secret) {
			t.Fatalf("sanitized payload leaked %q: %s", secret, result)
		}
	}
	if !strings.Contains(result, "[REDACTED]") || !strings.Contains(result, "safe") {
		t.Fatalf("sanitized payload lost redaction marker or safe value: %s", result)
	}
}
