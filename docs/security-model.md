# Security model

The HTTP server rejects non-loopback hosts and mutation requests without the local anti-CSRF header. Browser endpoints do not accept arbitrary shell commands or paths. Provider processes receive an explicit argument vector, a minimized base environment, only allowlisted provider configuration variables, an absolute working directory, bounded timeouts, and capped output. Process policy is checked before launch.

Provider output and event payloads are redacted before they are returned, persisted, replayed, or streamed. Sensitive payload field names are redacted recursively, including hyphenated and underscored variants. Checkpoints use the same sanitizer. Approval records have explicit pending, approved, and denied states and can only be resolved once. Profile and configured workspace scopes are deny-first during routing and workflow launch.
