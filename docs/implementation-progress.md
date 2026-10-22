# Implementation Progress

## Checkpoint: 2026-10-22

### Phase checkpoint — persistence and local security hardening

**Verified in this checkpoint**

- Run transitions are validated; terminal states cannot be overwritten.
- The recovery primitive changes abandoned `RUNNING` runs to `BLOCKED` and appends a durable `run.interrupted` event with a reason. It is not wired to general application startup because each CLI read command opens the DB too; automatic stale-process detection remains incomplete. Resume remains unavailable until safe checkpoint recovery is implemented.
- User prompts and nested event payloads are redacted before persistence, including values stored under secret-shaped keys.
- Provider subprocess stdout/stderr are bounded to 1 MiB each and continue draining after the cap to avoid unbounded memory growth or child-process blockage.
- Browser CSRF token is randomly generated for each API server process; dashboard obtains it from same-origin `/api/v1/version`.
- Origin comparison is exact and event cursors reject malformed or negative values.
- Dashboard now displays persisted run routes, stage progress, quality gates, activity events, and cancel control. All displayed run data comes from the local API.
- `go test ./...` and `make build` passed after these changes.

**Spec-Kit gate status**

- Existing Spec-Kit 1.2.2.dev0 and integration command artifacts are present. `specify check` reports Claude Code, Codex CLI, and OpenCode available.
- The OpenCode invocation attempt could not start because its log path is outside the writable workspace (`EPERM`); no model/provider run was started. The analyze/converge agent commands still need to be executed in a compatible agent runtime. Do not mark this gate passed.

**Commit status**

- Git was initialized and the first verified checkpoint is `a2e49dc` (`Harden local persistence and dashboard controls`).

### Phase checkpoint — configuration and CLI readiness

**Verified in this checkpoint**

- Added versioned project JSON configuration with schema checks, atomic replacement, and mode `0600` writes. Missing project configuration resolves to documented hybrid/fake defaults.
- Implemented `spectide init`, `config`/`config validate`, provider profile registration/listing, provider readiness diagnostics, run history/status/report/logs, offline demo, dashboard URL, and explicit safe-resume refusal when no verified checkpoint exists.
- The `run` help and output identify the current runner as simulated; live workflows are not claimed as implemented.
- Added API tests for lookalike origins and invalid SSE cursors, plus runner output-limit/cancellation, config permissions/validation, state-transition/restart recovery, and nested-secret redaction coverage.
- `go test ./...`, `go vet ./...`, `make build`, `spectide --help`, and `spectide config validate` passed.

**Remaining in this phase family**

- Global/project configuration precedence and route-to-profile validation remain incomplete. Project records and workspace locks are now persisted, but policy authorization is not wired into live execution.
- The CLI does not yet execute real Spec-Kit/provider stages. `resume` is deliberately blocked without safe checkpoints.
- The configuration/CLI checkpoint is committed as `6d8df80`.

### Phase checkpoint — versioned persistence and adapter event contracts

**Verified in this checkpoint**

- Added embedded migration `internal/store/migrations/001_initial.sql` and `schema_migrations` version tracking. It creates projects, routing decisions, artifacts, checkpoints, approvals, and exclusive workspace-lock tables in addition to the existing run state tables.
- Runs canonicalize their project path, register the project, and acquire a SQLite uniqueness-backed writer lease. A contending run becomes `BLOCKED`; the lease is released after the workflow reaches a terminal result.
- Every simulated stage has a persisted routing decision in the report. This is evidence for the simulated route only.
- Claude, Codex, and OpenCode adapters accept bounded JSONL fixture streams, reject malformed/oversized events, and discard raw payloads.
- `go test ./...` passed, including migration, state transition, one-writer contention, cancellation/output-bound, event parsing, and secret-redaction coverage.

**Remaining in this phase family**

- Artifact/checkpoint/approval repositories and transactional tests are not implemented yet; the schema tables alone are not feature completion.
- Recovery is available as a persistence primitive but is intentionally not invoked on every `App.Open`; doing so could misclassify a live run when read-only CLI commands open the database. Safe single-daemon ownership and stale-process reconciliation are still required.
- Provider process invocation is not yet orchestrated by the workflow engine. Provider live smoke checks remain `NOT_RUN`.

### Phase checkpoint — Spec-Kit command mapping and configured stage routing

**Verified in this checkpoint**

- Spec-Kit discovery now reads installed integration manifests and verifies matching command templates instead of assuming OpenCode paths. Stage templates are loaded from within the canonical project root, YAML front matter is removed, and arguments are substituted without executing repository text.
- Project routing modes (`manual`, `hybrid`, `automatic`) resolve per stage and persist auditable decisions. Configured non-fake routes fail closed before a run is created while live workflow invocation is unavailable.
- Provider probes now report `NOT_PROBED` for authentication and perform a bounded version-only invocation. The runner passes a minimized environment with explicitly approved provider configuration variables.
- Adapter invocation policies use provider-specific read-only/workspace-write switches; unsupported OpenCode read-only and Claude effort requests are rejected.
- Ledger tasks T020–T023 were marked complete against their fixture, probe, and subprocess scope. Routing policy capability/fallback validation remains open under T024.
- Targeted package tests, `go test ./...`, and `go vet ./...` passed. `make build` produced the arm64 binary; Go printed a sandboxed module-cache stat-write warning after the build.

**Spec-Kit quality gates**

- `specify` reports the installed OpenCode integration and required command artifacts. The supported OpenCode CLI accepts `opencode run [message]`, not `--command`; `/speckit.analyze` is currently running through the installed prompt interface. No analysis result has been recorded yet, and no convergence run has started.

**Remaining in this phase family**

- Live provider stage orchestration, quality evidence collection, analyzer parsing/remediation bounds, real append-only convergence loop, and checkpoint resume remain incomplete. A non-fake route is rejected rather than simulated.
- The React/TypeScript source dashboard and browser tests, expanded API and SSE behavior, full recovery, and release acceptance evidence remain incomplete.

### Phase checkpoint — durable evidence repositories

**Verified in this checkpoint**

- Added transactional repositories for artifact evidence, append-only workspace checkpoints, and pending/resolved approvals. Artifact paths must be workspace-relative and SHA-256 values valid; checkpoint JSON is validated and recursively redacted before persistence.
- Run reports now include artifact, checkpoint, and approval evidence alongside stages, gates, and routing decisions. Workspace locks and routing-decision persistence were already available and are covered by workflow tests.
- `go test ./internal/store ./internal/workflow ./internal/api` passed, including repository round trips, path rejection, checkpoint secret redaction, approval immutability, and report projection.
- Task T047 is checked complete.

**Remaining**

- Checkpoints do not yet enable resume; workspace fingerprint verification, owner reconciliation, and immutable attempt-history semantics remain incomplete.

### Earlier checkpoint summary

### Completed and verified

- One master feature specification, plan, data model, contract, task ledger, and checklist are present under `specs/001-build-spectide/`. A formal Spec-Kit analyze report has not been produced yet.
- Go module, Cobra binary, SQLite WAL-backed run/stage/event/gate persistence, deterministic simulated lifecycle, baseline loopback API, SSE replay, embedded dashboard shell, and API security baseline are implemented.
- `go test ./...`, `make build`, and the foreground offline demo completed successfully in this environment.

### Current truth

- The offline demo is `SIMULATED`; it is not a live Claude, Codex, or OpenCode integration pass.
- The earlier doctor run discovered Codex and OpenCode executables but did not authenticate or execute them. `specify check` later reported Claude Code, Codex CLI, and OpenCode available.
- The first aborted demo run was correctly retained as `CREATED`; no false completion was recorded. Foreground demo behavior was then corrected and produces a completed, evidenced simulated run.

### Immediate next work

1. Implement profile/configuration, routing, authorization, and workspace lock foundations.
2. Implement provider adapter interface, sanitized runner, synthetic fixtures, and real capability-gated invocation builders.
3. Implement Spec-Kit detection and read-only analyze / append-only converge semantics.
4. Replace the embedded bootstrap dashboard with the planned React source build and tests.
5. Complete API endpoints, recovery, verification suites, docs, and then run convergence.

### Blockers

- No blocker to offline development.
- Live-provider smoke checks are `NOT_RUN` pending authorized profiles; they must remain so until manually verified.
