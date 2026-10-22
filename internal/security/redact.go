// Package security holds host-boundary protections shared by providers, events, and APIs.
package security

import "regexp"

var sensitive = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)\b(sk-[a-z0-9_-]{12,}|gh[pousr]_[a-zA-Z0-9_]{12,}|xox[baprs]-[a-zA-Z0-9-]{12,})`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)\s*[:=]\s*)[^\s"',]+`),
}

// Redact removes common credential forms before they enter a database, event stream, or report.
// It is defense in depth; callers must still minimize inherited environments.
func Redact(input string) string {
	out := input
	for _, pattern := range sensitive {
		out = pattern.ReplaceAllStringFunc(out, func(match string) string {
			if i := regexp.MustCompile(`[:=]`).FindStringIndex(match); i != nil {
				return match[:i[1]] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return out
}
