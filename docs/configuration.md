# Configuration

SpecTide reads non-secret JSON settings from the user-global file and the project's `.spectide/config.json`. Values from the project override global values; stage routes merge by stage. Profiles and credential material are stored separately. SpecTide does not read or copy provider credential files.

The user-global file is `spectide/config.json` under the operating system's user configuration directory. On macOS this is typically `~/Library/Application Support/spectide/config.json`; the CLI's `config` command prints the effective settings.

Example project configuration:

```json
{
  "version": 1,
  "project": "/absolute/path/to/repository",
  "routing_mode": "hybrid",
  "default_route": {
    "provider": "fake",
    "profile": "fake-demo",
    "model": "deterministic",
    "effort": "none"
  },
  "stage_routes": {
    "analyze": {
      "provider": "fake",
      "profile": "fake-demo",
      "model": "deterministic",
      "effort": "none"
    }
  },
  "allowed_scopes": ["/absolute/path/to/repository"]
}
```

Routing modes are `manual`, `hybrid`, and `automatic`. Manual mode requires explicit routes for each stage; hybrid mode uses stage overrides and otherwise the default; automatic mode uses a deterministic risk explanation and the configured default. Supported providers are `fake`, `claude`, `codex`, and `opencode`.

A route may list explicit fallbacks by adding `fallback_authorized: true` and a `fallback` array containing provider/profile routes. Each fallback must resolve to a stored profile for the same project scope and pass the same persisted capability checks as the primary route; nested fallbacks are rejected. The route preview and immutable run configuration retain this fallback intent for audit. Automatic provider switching is not currently verified; a primary invocation failure remains a persisted failure rather than an unapproved fallback attempt.

`allowed_scopes` is an optional list of existing canonical directory roots. When configured, workflow launch and route preview reject projects outside those roots. Stored non-fake profiles also need an allowed root containing the project. A route preview reports `NOT_RUN` and does not create a run.

Initialize or validate settings with `spectide init --project PATH` and `spectide config validate --file PATH/.spectide/config.json`. Keep credentials in the provider's own account tooling; do not put secrets in SpecTide JSON or command arguments.
