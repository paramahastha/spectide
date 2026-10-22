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
  "allowed_scopes": ["/absolute/path/to/repository"],
  "execution": {
    "default_stage_timeout_seconds": 1800,
    "stage_timeout_seconds": {"analyze": 600},
    "analyze_remediation_limit": 3,
    "test_repair_limit": 4,
    "convergence_round_limit": 5
  }
}
```

Routing modes are `manual`, `hybrid`, and `automatic`. Manual mode requires explicit routes for each stage; hybrid mode uses stage overrides and otherwise the default; automatic mode uses a deterministic risk explanation and the configured default. Supported providers are `fake`, `claude`, `codex`, and `opencode`.

A route may list explicit fallbacks by adding `fallback_authorized: true` and a `fallback` array containing provider/profile routes. Each fallback must resolve to a stored profile for the same project scope and pass the same persisted capability checks as the primary route; nested fallbacks are rejected. The route preview and immutable run configuration retain this fallback intent for audit. Automatic provider switching is not currently verified; a primary invocation failure remains a persisted failure rather than an unapproved fallback attempt.

`allowed_scopes` is an optional list of existing canonical directory roots. When configured, workflow launch and route preview reject projects outside those roots. Stored non-fake profiles also need an allowed root containing the project. A route preview reports `NOT_RUN` and does not create a run.

`execution` is optional. Omitted loop limits resolve to 3 analyze remediations, 4 test repairs, and 5 convergence rounds; overrides may lower but not exceed those limits. The default provider-stage timeout is 1,800 seconds. Per-stage timeout overrides and the default accept 1 through 14,400 seconds. At run creation, SpecTide snapshots the effective stage routes, approved profile executable/configuration/scope and capability references, timeout and loop settings, and required-check definitions. A resumed run restores those frozen settings, and a provider launch fails closed if its profile references drift.

Initialize or validate settings with `spectide init --project PATH` and `spectide config validate --file PATH/.spectide/config.json`. Keep credentials in the provider's own account tooling; do not put secrets in SpecTide JSON or command arguments.
