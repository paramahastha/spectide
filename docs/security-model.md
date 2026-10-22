# Security model

The HTTP server rejects non-loopback hosts and mutation requests without the local anti-CSRF header. Browser endpoints do not accept arbitrary shell commands or paths. Provider output is redacted before it is returned from the process runner. Profile scopes are deny-first during routing.
