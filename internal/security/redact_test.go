package security

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	in := "Authorization: Bearer top-secret-token api_key=abc123456789012345 sk-abcdefghijklmnopqrstuvwxyz"
	out := Redact(in)
	for _, secret := range []string{"top-secret-token", "abc123456789012345", "sk-abcdefghijklmnopqrstuvwxyz"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret leaked: %s", out)
		}
	}
}
