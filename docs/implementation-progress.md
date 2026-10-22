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

**Spec-Kit gate status at this checkpoint**

- Existing Spec-Kit 1.2.2.dev0 and integration command artifacts are present. `specify check` reports Claude Code, Codex CLI, and OpenCode available.
- The initial OpenCode invocation could not start because its log path is outside the writable workspace (`EPERM`). Later, OpenCode returned an upstream 503 for analyze; the installed Codex CLI executed the exact Spec-Kit analyze instruction in read-only mode and recorded its report in `specs/001-build-spectide/analysis.md`.
- T044 is complete in commit `57642cd`. Converge remains an end-of-implementation quality gate.

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

### Phase checkpoint — Spec-Kit artifact and quality report parsing

**Verified in this checkpoint**

- Added safe inspection of the canonical feature `spec.md`, `plan.md`, and `tasks.md`, rejecting symlinks that escape the feature directory.
- Added structured parsers for analysis findings and convergence outcomes.
- Added an append-only convergence validator that proves the preexisting task ledger is unchanged, rejects reused or out-of-order task IDs, and requires the next phase number.
- Parser tests passed, including malformed reports, secret-free output assumptions, symlink escapes, and changed/reused convergence task data. Task T014 is checked complete.

**Remaining**

- Parsing support does not execute agent commands. Workflow orchestration must still run installed Spec-Kit commands, persist their evidence, and handle remediation and convergence bounds.

### Phase checkpoint — quality registry and local verification command

**Verified in this checkpoint**

- Added a quality registry with explicit `PASS`, `FAIL`, `BLOCKED`, and `NOT_RUN` states, per-check timeouts, bounded summaries, and a required-check success predicate.
- Added `spectide verify --project DIR` for gofmt, go vet, Go tests, and build checks. The build output uses a temporary file and is removed after inspection.
- The command ran against this repository and all four checks passed. Evidence was emitted as JSON with command, exit code, duration, summary, and UTC start time.
- `go test ./...`, `go vet ./...`, and `make build` also passed. Task T015 is checked complete.

**Remaining**

- Check results are returned to the caller but are not yet persisted against a workflow run. The workflow's simulated `tests` stage remains visibly simulated and does not claim live test execution.

### Phase checkpoint — React dashboard source and embedded production build

**Verified in this checkpoint**

- Added the React 19, TypeScript, Vite dashboard project under `web/` and embedded its production build in `internal/webassets/static/`.
- The dashboard reads persisted runs, route decisions, attempts, gates, artifacts, approvals, profile readiness, and SSE events from the local API. It can start a labeled simulated run and request cancellation.
- The interface is responsive, keyboard-focus visible, uses semantic buttons/forms, and makes the simulated/live execution boundary explicit. It uses system fonts and makes no third-party network requests.
- `make test`, `make lint`, `make web-test`, `make web-build`, `make build`, and `git diff --check` passed. Frontend typecheck and two domain unit tests passed; production assets are embedded by Go.
- Tasks T002 and T030 are checked complete.

**Remaining**

- Browser end-to-end coverage and screenshot evidence are not complete. The local browser inspection attempt was blocked by the browser access gate for `http://127.0.0.1:4747`; no alternate browser path was used.
- API idempotency, project/config/approval endpoints, SSE high-water replay/backpressure, recovery, and workflow integration of real quality evidence remain open.

### Phase checkpoint — API idempotency, project approvals, and SSE replay

**Verified in this checkpoint**

- Added schema migration 2 for durable idempotency keys, scoped to a request hash. Reusing a key for the same request returns the original run; a changed request is rejected.
- Added strict bounded JSON decoding, structured API errors, project registration/list/config-read endpoints, and approval create/resolve endpoints. Registered project configuration is only read after revalidating the canonical path.
- Added paginated SSE replay with a captured high-water mark, 256-event pages, periodic heartbeats, 10-second write deadlines, and immediate exit after a client write error.
- Added tests for migration version, idempotency reuse/mismatch, event pagination boundaries, host/origin/CSRF, symlink path replacement, oversized requests, project configuration, approvals, and client disconnect.
- `go test ./internal/api ./internal/store ./internal/workflow` passed.
- The full `make test`, `make lint`, `make web-test`, `make web-build`, and `make build` sequence passed. The offline demo reached `COMPLETED` and remained labeled `SIMULATED`.
- Tasks T028, T029, and T029a are checked complete.

**Remaining**

- Config write/update endpoints, API action idempotency beyond run creation, and a real socket slow-reader throughput measurement remain open. Full recovery and live provider workflow execution remain unavailable.

### Phase checkpoint — cross-process run controls

**Verified in this checkpoint**

- Added migration 3 for durable run control requests.
- The workflow now supports pause/resume at stage boundaries and during a stage wait, while preserving remaining simulated stage time. Control requests from a separate CLI process are polled from SQLite; cancellation is also observed cross-process.
- Added `spectide pause RUN_ID`, active-run `spectide resume RUN_ID`, and local API pause/resume routes. Resume still refuses runs without an active worker and verified checkpoint; restart recovery is not claimed.
- Added a cross-engine test that pauses, resumes, then cancels runs through a separate workflow controller. `go test ./internal/workflow ./internal/store ./internal/api ./internal/cli` passed.

**Remaining**

- Recovery after owner-process termination, checkpoint fingerprint drift checks, and safe resume after restart remain incomplete.
- Full Go validation, lint, frontend tests/build, production Go build, and `git diff --check` passed for this checkpoint.
- T034 remains open because process reconciliation, safe restart resume, and checkpoint drift detection are not implemented.

### Phase checkpoint — runner cancellation cleanup

**Verified in this checkpoint**

- Unix provider subprocesses now run in their own process group. Context cancellation kills the group and waits for pipe cleanup; other supported platforms fall back to killing the direct child.
- Added a cancellation cleanup test proving a spawned child cannot finish a delayed file write after cancellation. Existing tests cover timeout behavior and the 1 MiB output cap.
- `go test ./internal/runner` passed. T048 is checked complete.

**Remaining**

- Provider stage orchestration and end-to-end retry/recovery evidence remain open.

### Phase checkpoint — deterministic lifecycle repetition

**Verified in this checkpoint**

- Added a 20-run deterministic acceptance test for SC-002. Every run must complete with all 12 stages, 12 routing decisions, and five passing gates whose evidence is labeled simulated.
- `go test ./internal/workflow -run TestTwentyDeterministicRunsCompleteWithRequiredEvidence -count=1` passed in 23 seconds. T049 is checked complete.

**Remaining**

- Failure injection, bounded remediation, convergence additions, and restart recovery scenarios remain open under T016 and T034–T036.

### Phase checkpoint — isolated offline smoke scripts

**Verified in this checkpoint**

- Added `scripts/demo.sh` and `scripts/smoke.sh`; both use a temporary SQLite database and remove its database/WAL files on exit.
- Updated `make demo` and `make smoke` to use the scripts. Smoke checks run `doctor`, complete the demo, and assert the output explicitly says `SIMULATED`.
- `make smoke` passed. T039 is checked complete.

**Remaining**

- The demo remains a deterministic simulated lifecycle; it does not invoke a live provider or official Spec-Kit stages.

### Phase checkpoint — functional requirement traceability

**Verified in this checkpoint**

- Added `docs/acceptance-ledger.md` with rows for FR-001 through FR-030 and SC-001 through SC-008, each showing current evidence and an honest acceptance status.
- The ledger explicitly records unresolved product behavior and analysis finding G1 (SC-008 has no defined P0 set). It does not label partial requirements as accepted.
- T042 is checked complete as a traceability artifact; `git diff --check` passed.

**Remaining**

- Open/partial rows require implementation and their linked evidence before release acceptance.

### Historical baseline (superseded by the checkpoints above)

### Completed and verified

- One master feature specification, plan, data model, contract, task ledger, checklist, and analyze report are present under `specs/001-build-spectide/`.
- Go module, Cobra binary, SQLite WAL-backed run/stage/event/gate persistence, deterministic simulated lifecycle, baseline loopback API, SSE replay, embedded dashboard shell, and API security baseline are implemented.
- `go test ./...`, `make build`, and the foreground offline demo completed successfully in this environment.

### Current truth

- The offline demo is `SIMULATED`; it is not a live Claude, Codex, or OpenCode integration pass.
- The earlier doctor run discovered Codex and OpenCode executables but did not authenticate or execute them. `specify check` later reported Claude Code, Codex CLI, and OpenCode available.
- The first aborted demo run was correctly retained as `CREATED`; no false completion was recorded. Foreground demo behavior was then corrected and produces a completed, evidenced simulated run.

### Initial implementation sequence (historical)

1. Implement profile/configuration, routing, authorization, and workspace lock foundations.
2. Implement provider adapter interface, sanitized runner, synthetic fixtures, and real capability-gated invocation builders.
3. Implement Spec-Kit detection and read-only analyze / append-only converge semantics.
4. Replace the embedded bootstrap dashboard with the planned React source build and tests.
5. Complete API endpoints, recovery, verification suites, docs, and then run convergence.

### Blockers

- No blocker to offline development.
- Live-provider smoke checks are `NOT_RUN` pending authorized profiles; they must remain so until manually verified.
