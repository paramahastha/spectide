# CLI Contract: Shared Workspace Commands

All commands remain local-only. Normal-path commands resolve the platform default workspace; a root `--db PATH` override remains authoritative for scripts, tests, demos, and isolated environments.

## Normal commands

| Command | Contract |
|---|---|
| `spectide setup` | Resolve/create default workspace; display the canonical current directory; request confirmation before creating repository-local configuration; idempotently register the project; report readiness and exact next remediation; make the matching dashboard available. Repeated calls do not duplicate records or rewrite configuration. |
| `spectide add [PATH]` | Default `PATH` is `.`. Canonicalize/register exactly one existing directory in the default workspace, without copying configuration, provider profiles, routes, scopes, or secrets. Return created/existing status and readiness. |
| `spectide projects` / `spectide projects list` | List registered projects with canonical path, display name, and sanitized readiness. |
| `spectide dashboard` | Probe the normal loopback dashboard. Reuse it only when status identifies the expected workspace. Otherwise launch one managed instance, wait for matching readiness, then print/open `http://127.0.0.1:4747`. Report foreign or unverifiable occupancy without claiming ownership. |
| `spectide run "PROMPT"` | Infer and canonicalize the current directory, require a matching registered project, revalidate readiness/scope/capabilities/lock, then start the workflow. An unregistered directory returns its canonical path and `spectide add PATH` remediation. |
| `spectide run --project PATH "PROMPT"` | Advanced explicit selection. `PATH` must resolve to a registered project in the selected workspace; it does not silently register it. |

## Existing advanced commands

- `spectide --db PATH …` selects an isolated workspace and always wins over the default resolver.
- `spectide serve --host HOST --port PORT` remains available for explicit isolated/dashboard development. Loopback-only host enforcement remains mandatory.
- `spectide projects register PATH` remains a compatibility alias for deliberate registration and adopts the shared registration validation/result behavior.
- `spectide init` remains available for intentional project configuration initialization. `setup` is the only guided flow that requires a confirmation before it invokes initialization.

## Required diagnostics

| Situation | Required result |
|---|---|
| Current directory is unregistered | State canonical path; show `spectide add /absolute/path`; do not start a run. |
| Project lacks config, route, or authorization | State the readiness reason and the least-privilege remediation; do not start a run. |
| Default address serves same workspace | State the existing URL and recommend `spectide dashboard`. |
| Default address is occupied by another/unverifiable service | State that ownership is not confirmed and recommend `serve --port PORT`; do not claim the process is SpecTide. |
| Explicit/default workspaces differ | Explain history separation using safe workspace IDs; never infer or disclose raw database paths. |
| Legacy state needs migration | Display source/destination labels, require confirmation before copy, report backup/validation outcome, and leave source intact. |

## Exit behavior

- Invalid paths, malformed project selection, and failed confirmation return command errors without state changes.
- Missing readiness, authorization, or writer availability returns the existing blocked outcome class with actionable diagnostics.
- Dashboard timeout, foreign endpoint, and workspace-ID mismatch return non-success and never open a URL as verified.
