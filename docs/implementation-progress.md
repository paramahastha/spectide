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
- `make verify` passed end to end, including loopback startup and simulated demo; its first recorded timing used separate monotonic origins and was later corrected below. T003, T041, and T043 are complete.
- Browser E2E/accessibility scans and SC-006 SSE load measurement remain separate, unrun acceptance work (T031, T043a).

### SC-001 measurement correction — 2026-10-22

- The first monotonic measurement implementation launched a separate Python process for each timestamp. On this host those processes reported process-relative monotonic origins, so the resulting 0.295 ms, 0.152 ms, and later negative measurement were invalid.
- Reworked `scripts/smoke.sh` so one Python process starts the server, requests `/healthz`, measures both points using one monotonic clock, and terminates the server. Corrected measurements: 26.624 ms (`make smoke`), then 18.830 ms (`make verify`). The full `make verify` target passed with the corrected check. Earlier values are explicitly superseded in `docs/final-verification.md`.

### Phase checkpoint — backup, restore, and evidence retention

- Added schema migration 5 for the detailed-history pruning marker.
- `Store.Backup` uses SQLite `VACUUM INTO`, private mode `0600`, an atomic no-overwrite destination, and same-database rejection. `RestoreBackup` checks SQLite integrity and supported schema version before atomically creating a new private file, then applies supported forward migrations; it never overwrites an existing target.
- Retention expires event/checkpoint detail older than 90 days for terminal runs. At creation, when 1,000 detailed runs already exist, the oldest terminal details are pruned first. Event cursors/types, run summaries, stage outcomes, routes, gates, approvals, and report metadata remain. If no terminal slot is available, run creation fails with `RETENTION_CAP_ACTIVE`.
- Tests cover private backup permissions, round-trip restore, refusing overwrite/newer schema, 90-day expiration with active-run protection, terminal-first pruning, preserved verdicts, and all-active cap rejection.
- `go test ./internal/store -count=1` passed. T051 is complete.

### Phase checkpoint — authorized route preview

- Replaced the API's hardcoded fake dry-run response with route decisions resolved from the selected existing project's configuration. The response explicitly says `execution: NOT_RUN` and includes no provider invocation.
- The preview resolves the project through symlinks, requires an existing directory, and rejects project configuration symlinks that escape the canonical root. It does not create a run or alter state.
- API tests verify all configured routes are returned without creating a run and that a config-file symlink escape is rejected. `go test ./internal/api -count=1` passed.
- T024 remains open for profile/provider/model/capability validation and direct/fallback authorization through actual launch policy.

### Phase checkpoint — global and project configuration precedence

- Added non-secret global configuration under the OS user config directory, with project `.spectide/config.json` values overriding global values and stage routes merged by stage.
- Workflow start, API dry-run and project configuration reads, and CLI config display/validation/preview now use the same effective configuration. API project config reads retain canonical-root and symlink-boundary checks.
- Tests cover default/global/project precedence and missing-layer defaults. The full `make verify` target passed after wiring the effective config into workflow start, CLI, and API. T008 is complete; corrected SC-001 measurement was 19.318 ms on the reference host.

### Phase checkpoint — canonical workspace and allowed-scope enforcement

- Workflow start now requires an existing directory, resolves symlinks to its canonical path, and applies effective configured allowed scopes before project registration, durable run creation, or provider routing. Rejected or out-of-scope paths create no run.
- Updated lifecycle fixtures to use real directories and added missing-path, outside-scope, and in-scope symlink coverage. Focused `go test ./internal/workflow ./internal/api ./internal/workspace` and full `make verify` passed. T009 is complete. Latest SC-001 measurement is 20.369 ms on the reference host.

### Phase checkpoint — process policy and event sanitization

- Extracted subprocess executable, working-directory, environment, and timeout checks into a typed `security.ProcessPolicy`, used by the runner before launching any child process.
- Centralized recursive event/checkpoint payload redaction in `internal/events`, covering map and slice variants as well as sensitive field names. Added explicit approval status constants while preserving the existing one-way durable approval lifecycle.
- Focused `go test ./internal/security ./internal/events ./internal/runner ./internal/store` and full `make verify` passed. T010 is complete. Latest SC-001 measurement is 16.634 ms on the reference host.

### Phase checkpoint — strict transitions and durable stage cursors

- Confirmed run state transitions reject terminal-state mutation and pair each accepted transition with an event in one transaction. Workflow execution now writes an append-only checkpoint after each successful stage, recording the completed/next stage, immutable run-configuration hash, and canonical workspace identity hash.
- Checkpoints are cursor evidence only. The hash does not cover workspace contents; lease-expired restart resume remains denied until T034 verifies checkpointed artifacts, Git HEAD, and managed paths.
- Focused `go test ./internal/workflow ./internal/store` and full `make verify` passed. T011's foundational transition and checkpoint work is complete. Latest SC-001 measurement is 17.148 ms on the reference host.

### Route-preview policy follow-up

- API route preview now applies effective `allowed_scopes` through the same canonical-root check used by workflow start. Out-of-scope previews return `ROUTE_PREVIEW_BLOCKED`, persist no run, and do not claim execution.
- Added shared route validation requiring stored non-fake profiles to exist, match the configured provider, and authorize the canonical project root. Both preview and workflow start use it; tests prove unauthorized profiles are rejected before run creation.
- Focused `go test ./internal/routing ./internal/workflow ./internal/api` passed. A full-gate regression in CLI status mapping was fixed, and `make verify` then passed. T024 remains open for capability-backed model/effort/read-only/write validation, fallback authorization, and live launch integration. Latest SC-001 measurement is 19.558 ms on the reference host.

### Durable blocked outcome for unavailable live routes

- Valid, scope-authorized non-fake routes now create an immutable run record and persist the first unavailable stage as `SKIPPED`, `stage.blocked`, `execution: NOT_RUN`, and a `BLOCKED` quality gate. The run is terminal `BLOCKED`; the CLI prints `BLOCKED run` rather than labeling it simulated. Unauthorized profile/workspace paths still create no run.
- Effective run configuration marks only fake routes simulated. Spec-Kit discovery checks whether the configured official stage command exists, and emits a specific `SPEC_KIT_STAGE_UNAVAILABLE` reason when it does not. No live provider process is invoked.
- Focused workflow/CLI/API/Spec-Kit tests and full `make verify` passed. T014 remains open for verified stage prompt invocation and live adapter integration; T024 remains open for capability validation and fallback policy. Latest SC-001 measurement is 17.535 ms on the reference host.

### Phase checkpoint — explicit fake-provider workspace and scenarios

- Added an explicit portable fake route to the example workspace and checked-in scripted scenarios for success, typed failure, rate limit, and cancellation. Adapter tests load and execute each scenario fixture so future changes are checked against deterministic contract behavior.
- `go test ./internal/adapters/fake` and full `make verify` passed. T038 is complete. Latest SC-001 measurement is 20.874 ms on the reference host.

### Phase checkpoint — user and contributor documentation

- Added verified local usage, configuration precedence, provider readiness, safe recovery, troubleshooting, and API documentation with an OpenAPI 3.1 contract. Corrected existing quickstart references to match implemented CLI flags and clearly stated that restart resume and CLI backup/restore are unavailable.
- Linked the guides from the root README. `docs/openapi.yaml` parses as YAML, CLI examples were checked against the implemented command flags, and full `make verify` passed. T040 is complete. Latest SC-001 measurement is 19.188 ms on the reference host.

### Phase checkpoint — SC-006 SSE moderate-load acceptance

- Added the opt-in `make load-test-sse` profile with one local server, ten concurrent SSE clients, a persisted 10,000-event backlog, and 10,000 live events at 100/s across 100 seconds. The test checks contiguous event sequences for each client and measures persisted-timestamp-to-frame-receipt latency for every non-heartbeat backlog and live frame.
- Result: all ten clients received 20,001 frames each (200,010 total); all-frame p95 was 270.133 ms, below 1 second. The initial sequential fixture exceeded the threshold at 1.018 s; a single-transaction backlog fixture measures the event timestamps at the batch's persistence point and avoids setup time being counted as delivery lag.
- `make load-test-sse` and full `make verify` passed. T043a and SC-006 are complete. Latest SC-001 measurement is 20.717 ms on the reference host.

### Spec-Kit discovery and invocation foundation

- Discovery now validates each declared Spec-Kit command path against the project root and verifies its SHA-256 from the integration manifest. `StagePrompt` checks the command hash again immediately before use to reject template changes after discovery.
- Added `speckit.ExecuteStage`, which expands only a verified command template, builds an explicit provider adapter invocation rooted at the canonical project, enforces provider/profile match, applies read-only mode to analyze, runs through the bounded runner, and normalizes output. No workflow starts this live path until authenticated capability checks are integrated.
- Focused `go test ./internal/speckit ./internal/workflow` and full `make verify` passed; the doctor smoke reports Spec-Kit `READY`. T014 remains open for workflow-level capability-backed adapter orchestration. Latest SC-001 measurement is 19.514 ms on the reference host.

### Fail-closed unprobed profile authentication

- Tightened the routing capability policy to accept only the explicit `AUTHENTICATED` state. Empty and `NOT_PROBED` states now block route resolution instead of passing through; the policy continues to reject explicit unauthenticated profiles.
- Focused `go test ./internal/routing` and full `make verify` passed. T024 remains open for persisting capabilities and wiring this policy to workflow launch. Latest SC-001 measurement is 18.958 ms on the reference host.

### Bounded workflow-loop budget foundation

- Added a per-run budget controller with the required 3 analyze-remediation, 4 test-repair, and 5 convergence defaults. It rejects unchanged analyze artifacts, rejects convergence rounds with unchanged task-ledger hashes and gate verdicts, and returns explicit exhaustion codes.
- Focused `go test ./internal/workflow -run LoopBudget -count=1` and full `make verify` passed. The budget policy is not yet driven by official stage outcomes, so T016 remains open for workflow-level remediation and retry orchestration. Latest SC-001 measurement is 20.640 ms on the reference host.

### Read-only analyze remediation driver foundation

- Added `RunAnalyzeGate`: it runs a baseline analysis, fingerprints managed artifacts before/after each analysis to enforce read-only behavior, routes one blocking finding per remediation to `spec.md`, `plan.md`, or `tasks.md` by finding ownership, and repeats analysis after that owner-artifact change. Unresolved owners fail closed. Critical/high findings block; medium findings block only when their category and text identify an unsatisfied security boundary, acceptance criterion, or prerequisite. The baseline does not spend a remediation round.
- Focused tests passed for severity policy, owner mapping, baseline handling, read-only mutation rejection, no progress, and exhaustion after exactly three remediation rounds. T016 remains open pending integration of the remediation and retry hooks with persisted canonical stage outcomes.

### Bounded test-repair driver foundation

- Added `RunTestRepairGate` with a baseline test pass, repair callbacks receiving only non-passing checks, a four-repair per-run cap, and deterministic verdict-set progress detection. Missing evidence and invalid verdicts fail closed.
- `go test ./internal/workflow -run RunTestRepairGate -count=1` passed for initial pass, repaired pass, unchanged failure, and exact four-repair exhaustion. T016 remains open until analyze, test repair, and convergence hooks are driven by persisted canonical stage outcomes in the run engine.

### Workflow convergence driver foundation

- Added `RunConvergence` to consume parsed convergence reports through explicit hooks. It snapshots the same product specification path, task ledger, and required gates; rejects path replacement, task rewrites/nonprogress, and report/task mismatches; implements only persisted append-only additions; reruns required tests; and enforces the five-round limit. A converged report is accepted only when the ledger is unchanged and all required gates pass.
- Added `RunFinalReview` to return review findings through fix → test → convergence under the shared per-run budget. It preserves the same product specification path and stops when the same review finding recurs without progress.
- Focused Spec-Kit/workflow convergence and final-review tests passed, including mismatch, replacement-path, append-limit, and repeated-finding cases. T035 remains open until the live stage runner consumes these hooks. No official convergence result is claimed.

### Phase checkpoint — durable run-owner leases

- Added migration 6 and per-run heartbeat leases. The workflow writes its owner lease atomically with the workspace lock, refreshes it while the worker is alive, and removes it at finish.
- Explicit recovery now blocks only running/paused runs with missing or expired owner leases; fresh workers are left alone. It appends a durable `run.interrupted` event with `WORKSPACE_DRIFT`, preserves run evidence, and releases the abandoned writer lock.
- Cross-process pause/resume still works with a fresh owner. Resume after owner expiry returns `WORKSPACE_DRIFT` and does not restart work. Automatic restart resume remains unavailable until checkpoint identity, immutable config, artifact hashes, Git HEAD, and managed-path fingerprints are verified.
- Tests cover migration 6 and stale paused-run recovery/lock release; `go test ./internal/store ./internal/workflow -count=1` passed. T034 remains open for safe checkpoint validation and resume.

### Phase checkpoint — persisted profile capabilities and fail-closed routing

- Added schema migration 7 and persisted versioned profile capability snapshots. Profile edits invalidate stale snapshots; warnings are redacted; provider mismatch and invalid authentication states are rejected. Version-only `profiles probe`/`models probe` results persist as `NOT_PROBED` for authentication and do not claim read-only or workspace-write verification.
- Route preview and run start now validate persisted executable/version, authentication, model/effort override, and stage execution-mode evidence. Unverified live profiles are rejected before run creation; tests cover denial, persistence, redaction, profile-edit invalidation, and a verified capability fixture. The CLI probe remains intentionally unable to assert authentication.
- Focused `go test ./internal/store ./internal/routing ./internal/workflow ./internal/api ./internal/cli -count=1` passed. Full `make verify` passed, including Go tests/vet, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke. Latest SC-001 measurement: 19.635 ms on the reference host.
- At this checkpoint, T024 remained open for fallback authorization. The later checkpoint below completes T024's routing-policy scope; live provider stage execution remains unavailable under T014. T016, T031, T034–T036, T045, and T046 remain open as recorded in [the canonical task ledger](../specs/001-build-spectide/tasks.md).
- Updated [the acceptance ledger](acceptance-ledger.md) and the blueprint’s current-status checklist to match this evidence. No official Spec-Kit analyze or converge result is claimed.

### Phase checkpoint — explicit fallback authorization and boundary validation

- Configured routes may now carry a bounded, single-level fallback list with explicit `fallback_authorized: true`. Route validation requires each fallback profile to match its provider, authorize the same canonical project, and satisfy persisted capabilities. Unauthorized, nested, missing-profile, and cross-project fallbacks fail before preview/run creation.
- The route decision returned by preview and stored in the immutable run configuration retains the authorized fallback intent. The workflow does not execute provider fallbacks yet; live adapter execution remains T014.
- Added config, routing, and API tests, including a cross-project dry-run rejection with zero runs created. Full `make verify` passed; latest SC-001 measurement is 20.573 ms. T024 is now complete; T014, T016, T031, T034–T036, T045, and T046 remain open.

### Phase checkpoint — verified Spec-Kit live-stage workflow path

- The workflow now registers the Claude, Codex, and OpenCode adapters and can execute a live canonical stage only after profile authorization, verified Spec-Kit command discovery, and a single complete product-spec directory check. Each invocation revalidates profile scope/capability to catch profile edits after run preflight.
- Live stage stdout is parsed in memory; only redacted normalized event summaries and bounded metadata are persisted. Analyze compares hashes of `spec.md`, `plan.md`, and `tasks.md` before/after execution. Converge parses its report and enforces an unchanged task ledger for clean convergence or append-only validation for task additions. Provider cancellation interrupts the bounded subprocess, and stage attempts now persist an explicit `RUNNING` state (schema migration 8).
- A fixture-backed engine test executes a verified plan command through an adapter and confirms the run remains `BLOCKED` with blocked quality gates instead of claiming simulated PASS results. `go test ./internal/workflow ./internal/speckit ./internal/store -count=1` and full `make verify` passed. Latest SC-001 measurement is 22.051 ms.
- T014 is complete in the canonical task ledger. No real provider run was possible: installed provider CLIs report unauthenticated, and official Spec-Kit convergence remains unrun. T016/T035 quality-loop wiring, T031, T034, T036, T045, and T046 remain open.

### Phase checkpoint — persisted live test-repair outcomes

- Live runs now execute the default registered local quality checks at the canonical `tests` stage, persist each check verdict and the aggregate `required_tests` gate, and checkpoint the completed stage. The four-attempt test-repair driver invokes the authorized `implement` route with only failed check keys and verdicts; unavailable repair routes and no-progress/exhaustion outcomes fail closed.
- The fixture-backed live engine test uses a minimal valid Go workspace to verify all local checks pass and persist `required_tests=PASS`, while analyze, convergence, final-review, and simulated-E2E gates remain `BLOCKED`. Existing focused tests cover the 4-repair limit and no-progress stop. T016 remains open: live analyze remediation and convergence/final-review loop outcomes still need engine integration, and the full end-to-end exhaustion matrix is T036.
- Targeted validation: `go test ./internal/workflow -run 'TestEngineExecutesVerifiedLiveStageAndBlocksSimulatedQualityCompletion|TestRunTestRepairGate|TestDefaultLoopLimits|TestLoopBudget' -count=1` passed. Full `make verify` passed: Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke. SC-001 measured 20.021 ms on the recorded macOS 15.1 / Apple M4 / 16 GiB / arm64 host. The offline demo remained explicitly `SIMULATED`.
- Follow-up in this same T016 checkpoint: live `analyze` now runs the bounded read-only analysis/remediation loop, persists analysis and remediation events plus `analyze_clean`, and verifies each remediation changes its owning canonical artifact only. The engine fixture executes a verified analyze route and asserts its persisted PASS gate. Owner-only mutation checks and full `make verify` passed, with SC-001 at 21.228 ms.

### Phase checkpoint — integrated bounded analyze, test, convergence, and final review

- Live analyze remediation, local test repair, append-only convergence, and final review now run through one shared per-run 3/4/5 budget. Each loop consumes the same in-memory budget; completed quality-stage checkpoints persist the counters for recovery/audit evidence.
- Convergence consumes the verified live Spec-Kit `converge` command, checks report/task-ledger equality, implements only newly appended tasks, and reruns tests. Final review uses the explicitly configured read-only `final_review` route with the verified `analyze` template; findings cycle through fix, tests, and convergence while preserving the same canonical spec path.
- The engine fixture executes live plan/analyze/converge/final-review routes and asserts `analyze_clean`, `required_tests`, `converged`, and `final_review` are PASS. The simulated-E2E gate remains blocked, so the mixed live/fixture run cannot claim full completion. Shared-budget, exhaustion, no-progress, owner-only mutation, append-only, and same-spec tests pass.
- Full `make verify` passed. Latest SC-001: 20.894 ms on macOS 15.1 / Apple M4 / 16 GiB / arm64; offline demo remains `SIMULATED`. T016 and T035 are complete in the canonical task ledger. T031, T034, T036, T045, and T046 remain open.

### Phase checkpoint — dashboard state coverage and accessibility evidence

- Added route-preview and profile-readiness views backed by the local dry-run/profile endpoints. Added explicit disconnected, blocked/error, empty, skipped, not-run, and simulated labels with semantic status/alert roles and visible keyboard focus styling.
- Added 26 Vitest/jsdom checks covering dashboard state pairs, run controls, dry-run request behavior, seven named Axe surfaces, and stylesheet contrast/focus/reduced-motion rules. Axe reports zero critical/serious violations; the color-contrast Axe rule is disabled under jsdom and measured separately (minimum tested primary-text ratio 5.29:1).
- Updated Vite to 7.3.7 and Vitest to 4.1.12 after the new dev-test dependencies exposed advisories; `npm audit --audit-level=low` reports zero vulnerabilities.
- Full `make verify` passed, including Go suites, dashboard typecheck/tests/build, secret scan, and offline smoke. Latest SC-001: 19.005 ms. Browser E2E, real 200% zoom, rendered focus/reduced-motion checks, and screenshots remain NOT_RUN; no browser executable is installed, and the earlier localhost browser access gate denied the browser inspection. T031 remains unchecked; details are in [the accessibility audit](accessibility-audit.md).

### Phase checkpoint — verified pause boundaries and restart recovery

- Pause requests now take effect at the next stage boundary; in-flight stages finish and checkpoint before the run becomes paused. Both live-owner and expired-owner resume paths verify canonical workspace identity, exclusive lock ownership, immutable run configuration, checkpoint row/state/cursor identity, registered and canonical Spec-Kit artifact content, Git HEAD/index, managed-path hashes, and saved loop-budget counters.
- Expired-owner takeover is transactional and limited to a paused run whose workspace lock still belongs to it. A mismatch blocks the run with `WORKSPACE_DRIFT`, appends `run.interrupted`, releases abandoned ownership, and preserves checkpoints, artifacts, and workspace content. Recovery never resets, cleans, or overwrites user files. Resume a paused run before starting another run after a process restart because start-time reconciliation blocks expired owners.
- Added tests for valid restart continuation, tracked managed-file drift, ignored registered-artifact drift, Git HEAD drift, checkpoint identity/configuration mismatch, and unchanged recovery evidence. `go test ./internal/workflow ./internal/store -count=1` and full `make verify` passed. Latest SC-001: 29.799 ms; offline smoke remained `SIMULATED`.
- T034 is complete in the canonical task ledger. T031 browser checks, T036 cross-layer failure/secret-leak/exhaustion scenarios, T045 official Spec-Kit convergence, and T046 screenshots/release reporting remain open.

### Phase checkpoint — cross-layer recovery, redaction, and retry bounds

- Added an API recovery test that resumes a stale paused run without a verified checkpoint, confirms the API returns `WORKSPACE_DRIFT`, the run becomes `BLOCKED`, and the workspace file remains unchanged.
- Added an API secret-leak test that carries a synthetic `api_key` through run creation, persisted SQLite prompt, report response, and SSE replay; none expose the secret. The store now hashes the original request prompt for idempotency before redacting the prompt for persistence, and a test verifies distinct secrets do not alias under one idempotency key.
- Existing workflow acceptance tests verify exhaustion at exactly 3 analyze remediations, 4 test repairs, and 5 convergence rounds, plus no-progress termination; T034 restart tests verify continuation and checkpoint/workspace drift evidence preservation.
- Focused workflow/API/store acceptance tests and full `make verify` passed, including static analysis, Go suites, dashboard tests/build, secret scan, and offline smoke. Latest SC-001: 19.849 ms; smoke remained `SIMULATED`. T036 is complete. T031 browser acceptance, T045 official Spec-Kit convergence, and T046 screenshot/release evidence remain open.

### Phase checkpoint — first official Spec-Kit convergence

- Invoked `speckit.converge` through the installed OpenCode integration using Specify CLI 1.2.2.dev0 and `openai/gpt-6.1-sol`. No `.specify/extensions.yml` hooks were registered.
- The official assessment checked 30 functional requirements, 8 buildable success criteria, 14 acceptance scenarios, all 56 existing task entries, 9 plan areas, and 5 constitution principles. It reported 25 actionable findings: 9 CRITICAL, 14 HIGH, and 2 MEDIUM. Findings span transactional evidence/redaction, cross-database writer fencing, durable attempts/artifacts, strict parsers, initial artifact lifecycle, provider safety/streaming/capability verification, final gates, approvals/controls, API/dashboard truthfulness, simulated failure execution, browser coverage, release evidence, and Node 26 alignment.
- Spec-Kit appended T055–T079 under a new Phase 9. Its own byte-level assertion verified the original 13,716-byte `tasks.md` prefix and all prior task IDs were unchanged; only the master task ledger was modified. No app code was changed by the official command.
- The official run’s uncached `go test -count=1 ./...`, `go vet ./...`, and 26 dashboard tests passed. Outcome: `tasks_appended`, not converged. T045 remains open while the appended tasks are implemented; T031 and T046 browser/release evidence remain open.

### Phase checkpoint — persistence and presentation redaction (T055)

- Store writes now redact quality-gate evidence, approval actions, and capability version/effort/warning metadata. Secret-bearing immutable run configuration, profile identity, and routing identity references are rejected before persistence so provider launch identifiers remain accurate rather than being rewritten by display redaction.
- API JSON/error responses and CLI JSON/report output sanitize nested secret-key values and redact inline secret patterns. Synthetic-secret coverage now checks SQLite, reports, CLI output, API responses/errors, and the prompt-to-SSE path from T036.
- Focused `go test ./internal/store ./internal/api ./internal/cli -count=1` passed. Full `make verify` passed: Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke. Latest SC-001: 19.065 ms; smoke remained `SIMULATED`. T055 is complete. T056 is the next convergence task; T031, T045, and T046 remain open.

### Phase checkpoint — canonical workspace ownership fencing (T056)

- Added a per-user-cache kernel lock keyed by the canonical workspace. It supplements SQLite ownership so separate database files cannot concurrently authorize writes to the same workspace. Startup recovery leaves stale-heartbeat runs untouched while the kernel reports a live owner.
- Provider invocations inherit the lock descriptor. Closing SpecTide’s descriptor does not explicitly unlock it; the kernel releases ownership only after provider children close their inherited copies. Resume refuses to claim while another process still holds the lock. A failed owner heartbeat cancels the worker and process group.
- Tests cover lock exclusion across processes, child-held lifetime, stalled heartbeat recovery, owner identity loss, and two independent databases contending for one workspace. Focused `go test ./internal/workspace ./internal/runner ./internal/store ./internal/workflow -count=1` and full `make verify` passed, including Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke. Latest SC-001: 19.236 ms; smoke remained `SIMULATED`. T056 is complete; T057 is next. T031, T045, and T046 remain open.

### Phase checkpoint — transactional evidence events (T057)

- Stage creation and state transitions now emit `stage.created`/`stage.transition` in the same transaction as the row change. Routing decisions, quality-gate upserts, approval creation, and approval resolution likewise commit with their ordered events. Run state transitions were already transactional and remain covered by the append-only sequence.
- Workflow persistence callers check stage, routing, quality, and event writes; a failed quality-event write cannot produce a completed simulated run. Test verdict persistence is deterministic by check key and propagates errors through test and convergence loops.
- SQLite trigger-based fault injection verifies rollback for each target mutation when its event insert fails; replay tests verify contiguous sequence numbers. Focused store/workflow acceptance tests and full `make verify` passed, including Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke. Latest SC-001: 22.043 ms; smoke remained `SIMULATED`. T057 is complete; T058 is next. T031, T045, and T046 remain open.

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
