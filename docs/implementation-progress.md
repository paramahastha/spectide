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

### Quality-gate clarification checkpoint

- The master spec now makes all FR-001–FR-030 and SC-001–SC-008 release requirements, defines SC-006's client/event/backlog/latency profile, and sets default bounds of 3 analyze rounds, 4 test repairs, and 5 converge rounds with a stated no-progress rule.
- A second read-only Spec-Kit analyze found missing task ownership for FR-003 and FR-027, an SC-001 threshold mismatch, and two plan placeholders. Added T050/T051 for immutable attempts and backup/retention; aligned SC-001 to 2 seconds, aligned the plan's migration path, and removed generic plan placeholders.
- `make smoke` now launches the actual loopback server on an ephemeral port and verifies `/healthz` within the two-second startup window before checking the simulated demo. `go test ./internal/api ./internal/cli` passed.
- The analyze gate is being rerun after these resolutions. Its report and final clean/remaining findings will be recorded in `specs/001-build-spectide/analysis.md`.
- A subsequent analyze pass reported zero ambiguity, duplication, or critical findings, with 100% requirement coverage. It requested the plan list all packages/scripts, clarified the T053-before-T048 dependency, and noted T001's missing story association. Those edits are now in the current artifacts; another read-only pass will verify them.
- Latest analyze also specified the implementation work now tracked by T053/T048: 1 MiB per output stream, 30-minute stage timeout default with 1-second–4-hour bounds, and 2-second graceful cancellation. These tasks are now complete and the runner suite passes.

### Spec-Kit analysis checkpoint — 2026-10-22

- Re-ran the repository's read-only Spec-Kit analyze instructions after resolving the task overlap and workflow-simulation clarifications. Result: 38/38 requirements mapped to 56 tasks, zero ambiguities, zero duplications, zero critical findings, and no constitution conflicts.
- Recorded the full report in `specs/001-build-spectide/analysis.md`; T044a and T054 are complete. The OpenCode command returned HTTP 503, so the equivalent repository instructions ran through installed Codex CLI in read-only mode. This is not represented as an official Spec-Kit invocation.
- Convergence remains open (T045). Work is not release complete; numerous implementation and acceptance tasks remain. Browser E2E/screenshots remain blocked by the previously recorded local browser access gate, and live provider runs remain NOT_RUN.

### Phase checkpoint — provider runner timeout and graceful cleanup

**Verified in this checkpoint**

- Provider invocations default to a 30-minute timeout and accept configured durations from 1 second through 4 hours; out-of-range values fail with `INVALID_STAGE_TIMEOUT`.
- Expiration is reported as `STAGE_TIMEOUT`; parent cancellation is reported as `RUN_CANCELLED`.
- Cancellation sends SIGTERM to the Unix process group, then escalates to SIGKILL after 2 seconds. Other supported platforms use interrupt signaling followed by direct-child kill fallback.
- Tests cover both 1 MiB output caps, timeout boundaries/default, actual timeout reporting, graceful signal handling before escalation, existing cancellation cleanup, and output redaction.
- `go test ./internal/runner -count=1` passed. Tasks T053 and T048 are complete.

**Remaining**

- Provider invocations remain unconnected to canonical workflow stages, and cross-platform process-group semantics still need platform CI coverage. Full suite validation will follow the next implementation slice.

### Phase checkpoint — typed provider errors and invocation contracts

**Verified in this checkpoint**

- Added `ProviderError` with stable codes, readable messages, and preserved underlying causes. Executable lookup, version probes, malformed/oversized event lines, unsupported effort, and denied read-only invocations use typed errors.
- Contract tests inspect errors via `errors.As`, verify preserved causes, and verify provider-specific read-only and effort policies remain fail-closed.
- `go test ./internal/adapters/... -count=1` passed. T018 is complete.

**Remaining**

- At this checkpoint, fake adapter scripted scenarios were still pending; canonical workflow integration remains open (T014).

### Phase checkpoint — scripted fake provider behavior

- The fake adapter can replay ordered event steps, deterministic failures, bounded cancellation-aware delays, and retry-after rate limits. Returned failures use typed `ProviderError` codes; event summaries are labeled `SIMULATED`, redacted, and contain no raw payload. Event types are constrained to a small safe character set.
- Contract tests cover successful scripts, secret redaction, typed failure, rate-limit metadata, and context cancellation. `go test ./internal/adapters/... -count=1` passed. T019 is complete.
- The scripted adapter is test support; canonical workflow orchestration still uses the existing deterministic lifecycle and does not invoke adapters (T014 remains open).

### Phase checkpoint — immutable run configuration and attempts

**Verified in this checkpoint**

- Added migration 4 for effective run configurations, storing canonical JSON and a SHA-256 integrity digest in the same transaction as run creation. Configured idempotency keys now include the effective route configuration.
- Reports include the immutable configuration record and its digest. Reads detect digest mismatch; SQLite triggers reject configuration update/delete and stage attempt update/delete.
- Stage attempt inserts must start at 1 and continue without gaps, wait for the previous attempt to become terminal, and preserve the original route and position. Attempts remain visible as separate stage rows.
- Tests cover migration registration, configuration hash/report persistence, idempotent reuse and changed-config rejection, mutation denial, append sequencing, immutable routing, and preserved failure history.
- `go test ./internal/store ./internal/workflow -count=1` passed. T050 is complete.

**Remaining**

- Retry orchestration that creates subsequent attempts and verified restart recovery remain open (T016, T034, T036).

### Phase checkpoint — reference-host smoke measurement

- Updated smoke validation to capture macOS/Linux identity, CPU model, installed memory, architecture, build command, and monotonic process-start-to-first-`/healthz` response time.
- On the reference host, `make smoke` passed on macOS 15.1 / Apple M4 / 16 GiB / arm64. The first HTTP 200 arrived 0.295 ms after process launch, within the 2-second SC-001 limit; the offline demo also completed with `SIMULATED` evidence.
- Added the measured result and honest outstanding PASS/FAIL/BLOCKED/NOT_RUN limits to `docs/final-verification.md`; this is a progress report, not a release approval. T039 is complete; T046 remains open.

### Phase checkpoint — local project and approval CLI commands

- Added `spectide projects list`, `spectide projects register PATH` (canonicalizes symlinks and requires an existing directory), and `spectide approvals list/create/resolve` over the existing durable repositories.
- CLI tests verify canonical registration/listing and an approval's pending-to-approved lifecycle with a second resolution rejected.
- `go test ./internal/cli -count=1` passed. At this checkpoint, T037 is complete; run exit semantics and offline model/probe details are still tracked separately.

### Phase checkpoint — offline profile and model probes

- Added `spectide profiles probe NAME` and `spectide models probe PROFILE`. Both use only the profile's executable version command through the bounded runner and never inspect credential files; output explicitly reports authentication and model catalog as `NOT_PROBED`.
- CLI tests use `/bin/echo` as a harmless version probe and assert the output makes no authentication/model readiness claim. `go test ./internal/cli -count=1` passed. At this checkpoint T025 is complete; T017's exit-code work followed in the next checkpoint.

### Phase checkpoint — stable run exit codes

- Added explicit process statuses: success 0, command/usage error 2, blocked run 3, failed run 4, and cancelled run 5. The binary entrypoint now uses the CLI mapping; unavailable live routes are surfaced as BLOCKED rather than generic command failure.
- Tests cover all run-state mappings, generic errors, success, and a project configured for an unavailable live route.
- `go test ./internal/cli ./cmd/spectide -count=1` passed. T017 is complete.

### Phase checkpoint — unified CI and verification target

- Added `make format-check`, `make security`, and `make verify`. The unified target runs formatting, Go vet/tests, dashboard typecheck/tests/build, Go binary build, tracked-file private-key/provider-token pattern scan, and the offline smoke path.
- Updated GitHub Actions to use Go from `go.mod`, Node 22, read-only repository permissions, and `make verify`. Updated contributor and changelog documentation.
- `make verify` passed end to end, including loopback startup and simulated demo. Latest SC-001 measurement was 0.152 ms on macOS 15.1 / Apple M4 / 16 GiB / arm64. T003, T041, and T043 are complete.
- Browser E2E/accessibility scans and SC-006 SSE load measurement remain separate, unrun acceptance work (T031, T043a).

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
