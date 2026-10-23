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

### Phase checkpoint — durable invocation and quality histories (T058)

- Added schema 9 for provider execution attempts and append-only quality execution/check results. Each live invocation has a start record, then one terminal update and completion event in a shared transaction. Records distinguish canonical stage from loop phase and iteration, retain provider/profile/model/effort, process ID, timestamps, duration, exit/result, safe error, redacted evidence JSON, stdout/stderr hashes, and reserved artifact links. Raw provider output is not persisted.
- The live quality loop records each check pass with check key, sanitized command, subprocess PID, exit code, duration, verdict, and redacted summary. Test repairs, convergence tests, and final-review tests get separate phase histories. Reports expose both provider attempts and quality histories. Retention/restore schema support was updated to version 9.
- Tests cover attempt iteration, completion-event rollback, immutable/delete guards, quality result redaction and history, event-sequence links, and report exposure for a fixture-backed live workflow. Focused store/workflow/runner/API tests and full `make verify` passed, including static analysis, all Go suites, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke. Latest SC-001: 21.473 ms; smoke remained `SIMULATED`. T058 is complete; T059 is next. T031, T045, and T046 remain open.

### T059 — Semantic stage outcomes and artifact evidence — 2026-10-22

- Live analyze and convergence output is parsed and validated before the provider attempt can be finalized as successful. Convergence task claims must exactly match the valid append in `tasks.md`; read-only analysis must leave canonical Spec-Kit artifacts unchanged.
- Generation stages require a changed owning artifact (constitution, spec/clarifications, plan, checklist, tasks); implementation/remediation requires a changed workspace file. Artifact inspection rejects path escapes and non-regular expected outputs. Verified changed files are registered through `SaveArtifact` with SHA-256 metadata and linked into the attempt and report.
- No-op provider stages and missing generated files fail even if the subprocess exits zero. Tests exercise semantic failure, no-op rejection, symlink escape rejection, report metadata, and artifact-to-attempt links.
- Focused workflow/store acceptance tests and full `make verify` passed. Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and smoke all passed. Latest SC-001: 17.638 ms; offline smoke remains `SIMULATED`. T059 is complete; T060 is next. T031, T045, and T046 remain open.

### T060 — Strict Spec-Kit report parsing and convergence append validation — 2026-10-22

- Analysis output requires a single exact report heading, recognized six-column findings table, complete finding fields and unique IDs, a valid coverage metric, and an explicit empty-findings representation. Coverage percentages and optional numerator/denominator summaries must agree. Truncated fences, quoted/fenced reports, unknown severity/table formats, malformed rows, and contradictory outcomes are rejected.
- Convergence output accepts only a standalone unquoted success marker or a structured findings report with one supported outcome. Task additions must be traceable, unchecked, unique, and consecutive; append validation requires one next-numbered Convergence phase, consecutive unused IDs, and exact reported/persisted task agreement.
- Tests exercise realistic report summaries, malformed/unknown/contradictory output, quoted/fenced success markers, duplicate and skipped IDs, checked/untraceable tasks, multiple phases, and ledger/report mismatches. Focused Spec-Kit/workflow tests, full `make verify`, `go vet ./...`, and `git diff --check` passed. Latest SC-001: 18.385 ms; offline smoke remains `SIMULATED`. T060 is complete; T061 is next. T031, T045, and T046 remain open.

### T061 — Workspace-wide analysis integrity and owner-only remediation — 2026-10-22

- Live analyze hashes regular project files across the workspace before and after the provider call, so changes to source code, contracts, configuration, or any other included workspace file fail read-only analysis.
- Remediation snapshots the workspace and requires exactly one changed file: the resolved owner. Spec, plan, and task findings map to their owning canonical artifacts. A design contract can be an owner only when its relative path appears in `plan.md`, it is a regular file, and symlink resolution remains inside the feature directory.
- Owner-only remediation uses the verified Spec-Kit `clarify`, `plan`, or `tasks` prompt template on the implementation route; the prompt restricts changes to the one owner and explicitly forbids product implementation. Attempt completion also rejects files outside that owner.
- Tests cover source and contract changes during read-only analysis, source/contract changes or deletion during remediation, new non-owner files, referenced/unreferenced contracts, symlink escape, and owner-specific prompt selection. Full `make verify` passed; Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke all passed. Latest SC-001: 21.574 ms; smoke remains `SIMULATED`. T061 is complete; T062 is next. T031, T045, and T046 remain open.

### T062 — Bind process launches to approved profile references — 2026-10-22

- Capability probes resolve the provider executable through symlinks, require a regular executable with safe permissions, and persist its canonical path and SHA-256 together with the canonical provider configuration directory. Store persistence compares those references to the current profile before accepting the capability.
- Live stage requests carry the approved identity into the runner. Immediately before launch, the runner revalidates the executable path/content and permissions, confirms the config environment points to the approved directory, and verifies the working directory still resolves to the canonical project root; execution uses the canonical executable and directory.
- Executable and configuration symlink retargets, binary replacement/content changes, unsafe file permissions, and working-directory changes fail closed. Profile reference drift removes the capability snapshot and records an ordered invalidation event in one transaction.
- Focused security/runner/adapter/store/workflow tests and full `make verify` passed. Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline smoke all passed. Latest SC-001: 18.126 ms; smoke remains `SIMULATED`. T062 is complete; T063 is next. T031, T045, and T046 remain open.

### T063 — Initialize and preserve live Spec-Kit artifact lifecycle — 2026-10-22

- Live preflight selects the sole feature directory containing any canonical artifact, supports a single incomplete/empty feature directory, and derives a stable `specs/001-<prompt-slug>` path for a fresh workspace. The selected directory is created only after route validation and workspace ownership succeed.
- Existing constitution, specification, plan, and task owner artifacts are recorded as skipped/preserved; missing artifacts remain owned by their respective stages. This keeps specify create-once and prevents plan/tasks from recreating existing scope.
- Cursor checkpoints persist the selected feature directory relative to the workspace. Restart uses that persisted path and validates containment rather than selecting again from whatever artifacts happen to exist later.
- Acceptance tests cover fresh and partial directory selection, path escape rejection, preserving an existing constitution/specification, creating a missing plan, and checkpoint persistence. Full `make verify` passed: Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline `SIMULATED` smoke. Latest SC-001: 21.319 ms. T063 is complete; T064 is next. T031, T045, and T046 remain open.

### T064 — Freeze effective run execution settings — 2026-10-22

- Added optional global/project execution configuration for stage timeout defaults and per-stage overrides, plus bounded analyze/test/convergence loop settings. Defaults resolve to 1,800 seconds and 3/4/5; provider-stage timeout values accept 1–14,400 seconds, and loop overrides may lower but not exceed the specified maxima.
- Each run stores a versioned immutable snapshot of all effective stage routes, approved profile executable/configuration/scope and capability references, required-check definitions, loop limits, and fully resolved per-stage timeouts. Configuration hash and checkpoint verification preserve that snapshot across restart.
- Live provider calls receive the frozen timeout through `adapters.Request` into every production adapter's `runner.Invocation`. Local quality checks reconstruct from frozen definitions, and analyze/test-repair/convergence/final-review loops run with the frozen limits. Provider launch rejects profile or capability-reference drift.
- Tests cover 3/4/5 defaults, global/project override merging, timeout defaults and minimum/maximum, invalid values, profile drift, frozen checks, and adapter timeout propagation. Full `make verify` passed: Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline `SIMULATED` smoke. Latest SC-001: 22.153 ms. T064 is complete; T065 is next. T031, T045, and T046 remain open.

### T065 — Credential-safe provider capability discovery — 2026-10-23

- Added per-provider status/capability probes using CLI `auth status`, `login status`, `auth list`, `models`, and help commands. Provider stdout/stderr is parsed in memory; the CLI returns only capability facts and the store validates and persists bounded identifiers, never account labels or credential contents. Probe failures remain `UNKNOWN`/`UNAVAILABLE` and are not treated as readiness.
- Claude and Codex report authenticated readiness only from their documented status output. OpenCode reports provider readiness only for providers with nonempty configured auth entries. Model catalogs are verified only for OpenCode when safe provider/model identifiers are present; Claude/Codex explicit models remain blocked because their CLIs do not expose a verified model catalog. Claude effort options and Claude/Codex execution modes are checked against installed help output; OpenCode stage execution remains blocked because its adapter has no verified read-only/workspace-write controls.
- Routing validates exact model and effort values, and the immutable run configuration freezes catalog status and observed values. Tests cover anonymized auth success/failure, catalog and effort parsing, secret omission, exact-value rejection, capability drift, and a fixture-backed offline launch carrying a configured model/effort.
- Focused adapter/routing/store/CLI/workflow tests and `make verify` passed. The full verification included Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and offline simulated smoke. Latest SC-001: 35.811 ms. T065 is complete; T066 is next. T031, T045, and T046 remain open; live account verification remains `NOT_RUN`.

### T066 — Evidence-based final acceptance and report — 2026-10-23

- Replaced the terminal unconditional live block with explicit gate aggregation. Live/mixed execution requires inspectable `PASS` evidence for `analyze_clean`, `required_tests`, `converged`, and `final_review`. Missing evidence becomes `BLOCKED`; failures remain `FAIL`. The simulation-only `simulated_e2e` gate is `NOT_RUN` for live/mixed runs and remains required for a simulated-only completion.
- Added a report acceptance projection and ordered `run.acceptance`/terminal events. The summary labels execution `LIVE`, `MIXED`, or `SIMULATED`, preserves each gate verdict, and reports `PASS`, `FAIL`, `BLOCKED`, or `SIMULATED` without treating deterministic fake output as live evidence.
- The fixture-backed mixed workflow now completes after all applicable live/local gates pass. Model tests cover successful mixed acceptance, fake evidence failing a live gate, a failing gate, and missing simulated evidence. Focused model/store/workflow/API/CLI tests and full `make verify` passed. Latest SC-001: 22.210 ms; offline smoke remains `SIMULATED`. T066 is complete; T067 is next. T031, T045, and T046 remain open.

### T067 — Bounded provider stream and terminal result extraction — 2026-10-23

- Runner stdout now tees retained bytes into the selected adapter while the child is running. The stream parser bounds total bytes, individual frames, and normalized event count; output truncation, malformed JSON, unsupported event versions, and typed provider failures return stable errors. Payload and reasoning bodies are not copied to normalized event records.
- Provider parsers extract Claude result/assistant text, Codex completed agent messages, and OpenCode text parts. `speckit.ExecuteStage` exposes terminal text separately from captured raw stdout, and the live workflow feeds that extracted text to analyze/converge parsers while retaining hashes of raw output for attempt evidence.
- Tests exercise partial JSONL frames, final result selection, error/unsupported-version records, oversized frames, live runner callbacks, and the integrated Spec-Kit invocation path. Full `make verify` passed. Latest SC-001: 21.637 ms; offline smoke remains `SIMULATED`. T067 is complete; T068 is next. T031, T045, and T046 remain open.

### T068 — Project-aware required quality checks — 2026-10-23

- Workflow run creation now detects Go modules, Node manifests/scripts/package manager lockfiles, and Python pytest markers. It freezes exact required command arguments, timeout, empty-output policy, and unavailable reason into the immutable run configuration. Go checks use `gofmt -l .`, `go vet ./...`, `go test ./...`, and `go build ./...`; Node checks invoke only the detected package manager's test script and optional build script; Python uses `python3 -m pytest -q`.
- Unsupported project layouts, invalid/missing Node test scripts, unavailable commands, and conventional shell exit 126/127 produce BLOCKED evidence. Missing checks do not trigger repair loops. The registry keeps commands as argument vectors and does not launch browser executables; every invoked check retains the actual process ID, command, timing, exit code, and safe summary.
- Tests cover Go and Node discovery, missing scripts and commands, unsupported projects, frozen configuration restore, and blocked-check repair suppression. Full `make verify` passed. Latest SC-001: 19.104 ms; offline smoke remains `SIMULATED`. T068 is complete; T069 is next. T031, T045, and T046 remain open.

### T069 — Durable sensitive-action approval and continuation — 2026-10-23

- Live implementation now creates a run-bound pending approval and pauses atomically before writing workspace files. Approval can continue an active worker or restart from the verified checkpoint; denial skips implementation and records a blocked authorization gate.
- The API binds approval resolution to the URL run ID and pending approval ID. Duplicate decisions and cross-run IDs are rejected. The dashboard presents accessible native approve/deny buttons only for pending approvals on paused runs and refreshes after resolution.
- Store/API tests cover mismatch, single resolution, persistence across reopen, and approved restart replay; workflow tests cover denial and fixture-backed approval before implementation. `make verify` passed: `go vet ./...`, `go test ./...`, dashboard typecheck, 27 frontend tests, production build, secret scan, and simulated smoke. SC-001: 19.621 ms.
- T069 is complete; T070 is next. T031, T045, and T046 remain open. Live provider execution remains `NOT_RUN`.

### T070 — Durable inner-loop pause and cancellation — 2026-10-23

- Analyze, test-repair, convergence, and final-review loops now honor shared pause/cancel controls between provider invocations. After each completed invocation, result and evidence are persisted, then a checkpoint records the active stage cursor and loop counters before the run enters PAUSED. Resume continues in-process; process restart replays from the verified active-stage cursor.
- Registered local quality checks persist each check result as it completes and poll durable control state while the subprocess is running. A second controller can cancel the child; runner process-group termination and cleanup remain bounded. The tests pause after an analyze invocation, inspect the durable checkpoint, resume, and cancel a long-running registered check from a second SQLite connection.
- Focused quality/workflow/store/API tests and full `make verify` passed: `go vet ./...`, `go test ./...`, dashboard typecheck, 27 frontend tests, production build, secret scan, and simulated smoke. SC-001: 21.163 ms.
- T070 is complete; T071 is next. T031, T045, and T046 remain open. Live provider execution remains `NOT_RUN`.

### T071 — Foreground CLI restart resume lifetime — 2026-10-23

- `spectide resume RUN_ID` now keeps the app/store open until the resumed worker or existing owner completes. It then returns the stable terminal-state exit code and reports durable completion; it no longer exits immediately after starting restart recovery.
- A subprocess test creates a stale-owner paused run with a verified checkpoint, builds the CLI, invokes `resume`, and confirms the subprocess remains alive through the recovery-claimed event, completed run state, and persisted terminal acceptance report.
- `go test ./internal/workflow -run TestCLISubprocessResumeWaitsForDurableTerminalEvidence -count=1` passed. Full `make verify` will be recorded after the remaining tasks at the next verification gate.
- T071 is complete; T072 is next. T031, T045, and T046 remain open. Authenticated provider execution remains `NOT_RUN`.

### T072 — Durable terminal gate inventory — 2026-10-23

- Every run now persists six gate rows from creation: `analyze_clean`, `required_tests`, `converged`, `final_review`, `simulated_e2e`, and `workflow_execution`, all initially `NOT_RUN`. Migration 10 backfills existing run history and records inventory initialization.
- Terminal state writes update `workflow_execution` to PASS, FAIL, or BLOCKED and append the acceptance summary in the same transaction. Abandoned and drift-blocked recovery also records terminal acceptance. Reports classify execution from the immutable route snapshot when stage routing decisions are incomplete, while preserving unfinished required gates as `NOT_RUN`.
- The store terminal matrix covers missing integration, early provider failure, cancellation, interruption, simulated success, and live success. Full `make verify` passed: Go vet/tests, dashboard typecheck, 27 frontend tests, production build, secret scan, and simulated smoke. SC-001: 23.425 ms.
- T072 is complete; T073 is next. T031, T045, and T046 remain open. Live account checks remain `NOT_RUN`.

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
### T073 — Dashboard event replay and connection recovery — 2026-10-23

- The dashboard now subscribes to persisted run lifecycle, stage, provider, routing, failure, approval, attempt, and quality events. Each event refreshes canonical run/report data; stream errors show a disconnected/reconnecting state, reconnects request events after the last seen sequence, and recovery clears the stale state. Event rendering sorts by durable sequence and deduplicates by run/sequence.
- A two-viewer React integration test injects a connection loss after sequence 1, changes the shared persisted report, then replays missed approval, quality, and failure events. Both viewers show the same refreshed report and ordered unique event list after reconnect. Frontend tests pass (28), typecheck and production build pass.
- Full `make verify` passed: Go vet/tests, dashboard typecheck/tests/build, binary build, secret scan, and simulated smoke. Latest SC-001: 17.906 ms. Browser E2E and rendered accessibility checks remain T077/T031/T046. T073 is complete; T074 is next.
### T074 — Truthful route and execution labels — 2026-10-23

- Run projections now expose execution class derived from the immutable route snapshot, with persisted route/stage evidence as a fallback for legacy runs. Reports and CLI status include the same execution class plus the evidence-based acceptance verdict. CLI run output labels terminal results from the persisted report; doctor reports live workflow availability as conditional on configured and authorized routes.
- Dashboard history labels each row whether or not it is selected, using SIMULATED/LIVE/MIXED/UNKNOWN. Selected-run detail shows execution class, acceptance verdict, and provider/profile/model/effort/attempt. The stream label is idle without a selected run and shows reconnecting on disconnect; stale global claims that every run is simulated or live-disabled were removed.
- Store, CLI, API, and component tests pass; dashboard suite has 29 passing tests with typecheck/build. Full `make verify` passed (Go vet/tests, dashboard, binary, secret scan, simulated smoke). Latest SC-001: 21.562 ms. T074 is complete; T075 is next. Live account checks remain `NOT_RUN`.
### T075 — Complete local HTTP contract — 2026-10-23

- Added a read-only capabilities endpoint exposing persisted, non-secret profile observations and a configuration-validation endpoint that reuses route-preview checks without creating a run. Run history is newest-first, bounded to 100, accepts `limit`/`offset`, and returns total/page/has-more headers. Stage and artifact metadata have dedicated endpoints, while reports serialize every evidence collection as an explicit array projection.
- All API errors use the documented nested envelope with sanitized message/details, a request ID in the body, and the matching `X-Request-ID` response header. Unknown API paths, extra path segments, and unsupported methods fail closed. OpenAPI and the user/master API guides describe the exact shapes and controls.
- Focused API/store tests passed, including two-page history, secret-free capability output, validation without run creation, request identity, strict routes, and projections. Full `make verify` passed (Go vet/tests, 29 dashboard tests, typecheck/build, binary, secret scan, simulated smoke); OpenAPI YAML parses. Latest SC-001: 22.359 ms. T075 is complete; T076 is next.

### T076 — Simulated failure scenarios through complete engine runs — 2026-10-23

- Simulated workflow stages now invoke the deterministic fake adapter and persist its normalized provider events. Scripted adapter errors stop the run at the affected stage, preserve typed sanitized failure metadata, mark the stage failed, retain all six terminal gates, and label the event/gate evidence `SIMULATED`.
- Full engine tests load the checked-in success, failure, and rate-limit fixtures; success completes all 12 lifecycle stages, while failure scenarios assert the prior stage succeeds and the scripted stage fails. Existing workflow tests cover failed-check repair, append-only convergence, loop exhaustion/no-progress, pause/cancel interruption, and workspace contention. Focused `go test ./internal/workflow ./internal/adapters/fake -count=1` passed.
- Full `make verify` passed: `go vet ./...`, `go test ./...`, dashboard typecheck, 29 frontend tests, production build, binary build, secret scan, and simulated smoke. SC-001: 20.936 ms.
- T076 is complete; T077 is next. Browser page/state acceptance and authenticated live-provider checks remain `NOT_RUN`.

### T077 — Browser acceptance attempt — 2026-10-23

- Component tests remain green (29 tests, seven named Axe surfaces with zero critical/serious violations; stylesheet contrast and reduced-motion rules are separately checked). The rendered acceptance matrix is documented in `docs/accessibility-audit.md` and `docs/screenshots/README.md`.
- Chrome is installed and available, but its saved origin approval policy rejected the local dashboard origin `http://127.0.0.1:4747` and explicitly prohibited alternate browser surfaces or indirect access. I stopped the attempt and requested that the user enable this specific local origin. Browser E2E, rendered keyboard/focus/contrast/reduced-motion, 200% zoom, and screenshots remain `NOT_RUN` pending authorization.
- T077 remains open; this is a concrete browser policy blocker, not a failed app test.

### T079 — Node 26 CI and contributor toolchain — 2026-10-23

- CI now selects Node 26, and README/user-guide prerequisites document Node.js 26 with npm and Go 1.27. Local Node is v26.12.0.
- `make verify` passed on Node 26: Go 1.27 vet/tests, dashboard typecheck, 29 tests, production build, binary build, secret scan, and simulated smoke. SC-001: 18.782 ms.
- T079 is complete.

### Official convergence — 2026-10-23

- After T055–T076 and T079, `/speckit.converge` returned `tasks_appended`, not converged. It appended T080–T106 to the existing Phase 9: 10 critical, 15 high, and 2 medium findings. The official pass verified the preceding 26,772 bytes of `tasks.md` and the product spec, plan, and constitution hashes remained unchanged.
- Findings include persistence redaction and config symlink containment, runner/process lock lifetime, read-only/owner-only workspace mutation checks, checkpoint/restart cursor semantics, repair-path approvals, final-gate aggregation, local stage routing, selected feature scope, semantic no-op evidence, parser compatibility, live event delivery, stream validation, quality-check discovery, migration compatibility, retention, dashboard evidence projections, CLI/API preview parity, release documentation, and packaging.
- Fresh review checks: `go vet ./...`, `npm run typecheck`, `npm test -- --run` (29 tests), and targeted race tests passed. `go test -count=1 ./...` failed `TestHeartbeatOwnershipLossCancelsActiveWorker` (observed state `FAILED`); the focused `-count=5` retry failed 2/5. `make load-test-sse` failed in fixture setup with `UNIQUE constraint failed: events.run_id, events.seq` before any latency measurement. See T101/T102; do not replace those outcomes with earlier passes.
- Official outcome: `tasks_appended`; T045 remains open. T077 remains blocked because the browser-origin policy denies local dashboard access. T080–T086 are complete; T087 is next.

### T080 — Secret-safe project, artifact, and config persistence — 2026-10-23

- Project display names and artifact kind metadata are redacted before persistence. Canonical project/run paths, allowed scopes, artifact IDs/stages/paths, route/profile/model/effort references, and config save paths reject values containing supported secret patterns rather than rewriting identity values.
- Synthetic-secret tests cover config JSON save/load, SQLite project/artifact values, backup bytes and restore, report projection, CLI project listing, and API project/report responses.
- `go test ./internal/config ./internal/store ./internal/cli ./internal/api -count=1` passed. T080 is complete; T081 is next.

### T081 — Canonical project-config containment on actual start — 2026-10-23

- Added shared `config.SafeProjectPath` resolution that canonicalizes the project root and verifies the `.spectide` metadata directory and `config.json` target remain within the project before settings are read. The independently authorized global config remains separately selected.
- Engine start and API preview/config reads use the shared resolver. Engine-start tests cover a config-file symlink and a parent `.spectide` directory symlink escaping to an external config; both are rejected before any run is created. Existing canonical workspace and API config symlink-swap tests remain green.
- Targeted config/workflow/API tests passed. T081 is complete; T082 is next.

### T082 — Process-group cleanup after early leader exit — 2026-10-23

- Runner cancellation no longer closes the escalation window when `cmd.Run` returns. It retains SIGKILL escalation for the full two-second grace period and reasserts process-group termination after a canceled or `exec.ErrWaitDelay` result, including when the direct child exited before a TERM-resistant descendant holding inherited pipes.
- Added a regression fixture with an early-exiting shell parent and a TERM-resistant child that continuously writes a marker. The runner waits for bounded cleanup and no later writes occur. The test passed three consecutive runs; existing process-group cleanup and graceful-TERM tests passed. `go vet ./internal/runner` passed.
- T082 is complete; T083 is next.

### T083 — Workspace lock inheritance for local quality checks — 2026-10-23

- `quality.Registry.RunControlled` accepts inherited descriptors and passes them into every registered check invocation. Workflow quality execution retrieves the active process-lock file under the engine mutex and supplies it to the registry.
- Live shell check tests close the controller descriptor and exit a separate lock-owner helper while the child remains active. In both cases the kernel lock stays held, another process cannot acquire it, and the lease releases after the check exits.
- `go test ./internal/quality ./internal/workflow -count=1`, the owner-process-exit focused test, `go vet ./internal/quality ./internal/workflow`, and `git diff --check` passed. T083 is complete; T084 is next.

### T084 — Read-only analyze mutation enforcement — 2026-10-23

- Analyze fingerprints now run after failed and cancelled provider invocations; a detected write takes precedence as `ANALYZE_MUTATED_ARTIFACTS` so failure cannot hide a boundary violation.
- Workspace snapshots include generated `vendor`, `dist`, and `build` files, directory and symlink entries, and Git HEAD/status. Provider adapters retain verified read-only modes or fail closed when unsupported.
- Tests cover mutation during a failed analyze, excluded/generated directories, symlink retargeting, and Git-only state changes.
- `go test ./internal/workflow -count=1`, focused mutation tests, `go vet ./internal/workflow`, and `git diff --check` passed. T084 is complete; T085 is next.

### T085 — Tasks-only convergence workspace enforcement — 2026-10-23

- Convergence snapshots the governed workspace before each provider invocation and compares it afterward, including on provider failure or cancellation. Only the selected existing `tasks.md` path is excluded from that comparison.
- Git status filtering permits that ledger path while preserving HEAD and all other status changes. Existing append-only validation and exact report/ledger matching remain required; alternate-spec creation and changes to specs, plan, constitution, or source are rejected.
- Targeted fingerprint and live mixed-engine convergence tests, `go test ./internal/workflow -count=1`, `go vet ./internal/workflow`, and `git diff --check` passed. T085 is complete; T086 is next.

### T086 — Current checkpoint manifest over immutable artifact history — 2026-10-23

- Checkpoint creation now selects the newest artifact observation for each normalized workspace path before validating current file bytes. It records one current hash per path without modifying or pruning any historical artifact rows.
- The restart regression test records two versions each of spec, tasks, and source, confirms all six observations remain in the reportable store, saves and verifies a checkpoint against the latest versions, then confirms an external post-checkpoint source edit still produces `WORKSPACE_DRIFT`.
- Targeted restart/recovery tests, `go test ./internal/workflow -count=1`, `go vet ./internal/workflow`, and `git diff --check` passed. T086 is complete; T087 is next.

### T087 — Restart continuation cursor and attempt recovery — partial, 2026-10-23

- Checkpoints can record pending phase, operation, task list/position, quality check, and spent loop budgets. Analyze checkpoints are now written after remediation budget accounting; convergence checkpoints follow report validation and round accounting.
- On an expired-owner restart, active/ambiguous provider attempts are finalized as `FAILED` with an explicit interruption reason. A still-active outer stage is finalized as failed, then the resumed stage is appended with the next attempt number rather than reusing attempt 1.
- The restart fixture verifies attempt finalization and numbering, and unit tests verify persisted cursor fields and that analyze/convergence boundaries observe spent counters.
- `go test ./internal/store ./internal/workflow -count=1`, `go vet ./internal/store ./internal/workflow`, and `git diff --check` passed. T087 remains open: Resume currently replays the whole active outer stage instead of dispatching the stored inner cursor. Process-exit/resume tests for approval, analyze remediation, local checks, and appended-task convergence are still required before marking complete.

### T088 — Invocation-bound workspace write authorization — partial, 2026-10-23

- Removed the single outer `implement` approval check. Every live `implement` invocation now checks a durable approval immediately before an execution attempt can be created, including analyze remediation, test repair, convergence implementation, and final-review fixes.
- Approval actions bind the run, phase, workspace identity, provider, and profile. Only the same stored action can reuse an approved decision. Denials persist a blocked authorization gate and return before provider invocation; tests verify no attempt is created and cross-phase/workspace decisions are rejected.
- The mixed live-engine fixture and approval phase tests pass with targeted checks; full store/workflow suites, vet, and diff checks passed.
- T088 remains open pending checkpointed recovery of a pending phase-specific approval and process-exit tests across repair paths; this depends on the unfinished executable inner-loop cursor in T087.

### T089 — Terminal acceptance and required gate aggregation — 2026-10-23

- Added schema version 11 with persisted `required` and `applicable` gate policy. Gate reports and events expose the policy; live execution marks the simulation-only gate inapplicable.
- Acceptance now includes `workflow_execution` and every applicable required gate. Failed/cancelled/blocked terminal workflow outcomes cannot report PASS, and `SetRunState(COMPLETED)` rejects any unsatisfied required gate. The engine evaluates a candidate successful terminal verdict before completing either live/mixed or simulated runs.
- Tests cover terminal failure/cancellation, extra required blocked gates, optional and inapplicable gates, and live/mixed/simulated report matrices. `go test ./... -count=1`, `go vet ./...`, and `git diff --check` passed. T089 is complete; T087 remains the current partial item.

### T090 — Engine-owned local tests and report stages — 2026-10-23

- Preflight now excludes engine-owned `tests`/`report` from Spec-Kit provider discovery and unsupported-command checks. Live/mixed runs persist these stages with the local engine route; local tests still use the quality runner and local report generation persists a projection hash/count event without inventing a provider attempt.
- Acceptance classification, configuration-derived execution labels, and simulated-gate applicability omit these engine-owned stages so a non-fake default does not misclassify them as provider work or trigger a false simulation-only gate.
- The verified mixed-run fixture now uses a non-fake default for tests/report and confirms local stage labels, no provider attempts for either stage, local report-generation evidence, and final acceptance PASS.
- The focused mixed-engine fixture, `go test ./internal/workflow ./internal/store ./internal/model -count=1`, `go vet ./internal/workflow ./internal/store ./internal/model`, and `git diff --check` passed. T090 is complete; T091 is next. T087/T088 remain partial.

### T091 — Bind provider prompts to the selected feature scope — 2026-10-23

- Added the selected feature directory to provider requests and centrally append a prompt directive after verifying it resolves to exactly `specs/<one-directory>` under the canonical project root. Specify is directed to create only the selected `spec.md`; every feature stage, retry, and remediation receives the same scope.
- After specify, the workflow verifies the selected directory is the only artifact-bearing feature scope, rejecting alternate specification creation. Existing artifact-owner checks still preserve an already-created selected spec.
- Tests cover canonical scope instructions, rejection of outside/nested scopes, rejection of a second artifact-bearing spec directory, and scope presence in live provider prompts. `go test ./internal/speckit ./internal/adapters ./internal/workflow -count=1`, package vet, and diff checks passed. T091 is complete; T092 is next. T087/T088 remain partial.

### T092 — Semantic evidence and explicit no-op outcomes — 2026-10-23

- Clarify and implement prompts now define standalone `SPECTIDE_NO_CLARIFICATION:` and `SPECTIDE_NO_WORK:` markers for genuine no-change outcomes. The parser requires one unquoted, unfenced marker with a specific reason; missing, duplicate, or ambiguous claims fail closed.
- A clarify no-op requires an inspectable selected specification with substantive sections and no changed artifact. An implementation no-op requires no changed files plus existing source evidence; subsequent required local quality checks still determine acceptance. A marker paired with a change is rejected.
- Changed checklists must contain actual checkbox requirements; changed task ledgers must contain stable task IDs, alongside existing scoped owner-path enforcement. Tests cover valid/invalid markers, missing artifacts, no-op evidence, malformed owner artifacts, and the mixed live workflow. `go test ./internal/workflow ./internal/speckit -count=1`, package vet, and diff checks passed. T092 is complete; T093 is next. T087/T088 remain partial.

### T093 — Master-ledger parser and convergence traceability — 2026-10-23

- The strict task parser now accepts lowercase letter suffixes already present in the master ledger and computes the next task number from the numeric portion without rewriting historical IDs. The actual `specs/001-build-spectide/tasks.md` is used as a regression fixture.
- Convergence parsing and append validation recognize the installed command's `plan: ...` trace form. Concrete source-file references are accepted only when named in the canonical plan; workflow convergence snapshots and live provider validation now pass that plan evidence through parsing and exact report/ledger comparison.
- Clean and findings convergence formats and path traceability have fixtures. The next-numbered-phase rule remains enforced; same-phase runtime append behavior was not added under the explicit scope clarification.
- `go test ./internal/speckit ./internal/workflow -count=1`, package vet, and `git diff --check` passed. T093 is complete; T094 is next. T087/T088 remain partial.

### T094 — Publish provider events during execution — 2026-10-23

- The incremental provider JSONL parser now calls a normalized event callback while frames are consumed. Live workflow attempts send those events through a bounded 128-item queue that never blocks the stdout reader on persistence or viewer speed.
- Persisted event payloads include safe provider event type, attempt ID, and order index, with generic redacted summaries and no raw bodies. Queue saturation is summarized as a dropped-event count; queued events drain before the attempt is finalized, and post-return events are not duplicated.
- The API/SSE test runs a delayed provider subprocess and asserts the client receives its first provider event before the process exits. Adapter callback and workflow publisher tests cover incremental delivery and attempt linkage. `go test ./internal/adapters ./internal/speckit ./internal/workflow ./internal/api -count=1`, package vet, and diff checks passed. T094 is complete; T095 is next. T087/T088 remain partial.

### T095 — Validate provider stream schemas and terminal outcomes — 2026-10-23

- Provider event parsing now enforces provider-specific event allowlists and validates every present schema version as the supported string value; numeric/object versions and unknown types fail closed. Required terminal events and terminal payload shapes are checked, and stdout truncation before a terminal result produces a distinct typed error.
- Text deltas are concatenated without adding separators between fragments. Runner callback failures wrap the original provider parser error, and Spec-Kit invocation returns typed stream errors rather than replacing them with generic stream/process errors. Provider failure and rate-limit event types retain their codes; raw provider bodies remain discarded.
- Tests cover terminal/missing/partial results, unknown event/version shapes, rate limiting, unsuccessful turns, text framing, redacted payloads, and typed parser-error propagation. `go test ./internal/adapters ./internal/runner ./internal/speckit ./internal/workflow ./internal/api -count=1`, package vet, focused terminal-payload tests, and diff checks passed. T095 is complete; T096 is next. T087/T088 remain partial.

### T096 — Defer quality discovery for newly created projects — 2026-10-23

- The immutable run configuration now records a check-selection policy: freeze checks detected at run start, or defer discovery until after implementation when no supported manifest exists. Newly derived Go/Node/Python checks are stored separately in an append-only `quality.checks.derived` event with provenance and a hash of all supported project manifests; the original configuration JSON is unchanged.
- Resume reuses the derived check list only when its manifest hash still matches. Unsupported projects remain explicitly BLOCKED. CLI `verify` now discovers checks for its target project rather than running SpecTide's own default Go checks.
- Tests cover newly created Go and Node projects, derived check restore and manifest drift, unsupported layout blocking, immutable configuration evidence, and target-specific CLI verification. `go test ./internal/quality ./internal/workflow ./internal/cli -count=1`, package vet, and `git diff --check` passed. T096 is complete; T097 is next. T087/T088 remain partial.

### T097 — Validate schema compatibility during database open — 2026-10-23

- Normal open now checks that migration metadata is contiguous and no version exceeds the supported schema before any new migration is applied. A gap or forward-incompatible database is rejected with an actionable schema code.
- Migration failures close the database connection and return recovery guidance. Schema changes and history updates run in one SQLite transaction, preserving the original schema/history if a migration fails.
- Tests verify future-version rejection without mutation, inconsistent-history rejection, rollback and connection cleanup for a deliberately failing upgrade, and the existing valid upgrade paths. `go test ./internal/store -count=1`, `go vet ./internal/store`, and `git diff --check` passed. T097 is complete; T098 is next. T087/T088 remain partial.

### T098 — Retain bounded detail while preserving execution summaries — 2026-10-23

- Retention now redacts old event/checkpoint payloads, provider attempt error/evidence JSON, and quality command/output summaries. The 90-day comparison expires details strictly before the cutoff; compact provider results, quality verdicts, gates, routes, approvals, artifact links, and sanitized report metadata remain.
- Migration v12 preserves append-only history while allowing only the targeted terminal-detail scrub updates. The 1,000-run cap uses the same pruning path for the oldest terminal run; active runs are never pruned. `spectide serve` performs maintenance at startup and every hour while idle.
- Tests cover just-before/exactly-at-90-day boundaries, active-run preservation, idle-daemon maintenance, and all 1,000 history slots with preserved attempt/quality outcomes. `go test ./internal/store ./internal/cli -count=1` passed. T098 is complete; T099 is next. T087/T088 remain partial.

### T099 — Render persisted execution evidence and recover dashboard state — 2026-10-23

- Dashboard report types and panels now expose provider invocation records and local quality executions, including actual phase iteration, active invocation status, route/model/effort, quality verdicts, and available safe summaries. Stage rows use the latest invocation data.
- SSE listeners now handle generic `provider.event`, provider start/result/failure records, and interruption records; visible activity rows show safe summary fields and never render raw event bodies. Successful run-list/profile refresh clears stale disconnected state even with no selected stream. Project or prompt edits invalidate route preview results, whose rows now expose model and effort.
- Component coverage verifies invocation and quality projections, safe activity summaries, provider event replay, connection recovery, and preview invalidation. `npm run typecheck`, `npm test -- --run` (33 tests), and `npm run build` passed. Browser-rendered validation remains blocked under T077. T099 is complete; T100 is next. T087/T088 remain partial.

### T100 — Share launch authorization across preview and validation — 2026-10-23

- Added one routing-level launch validator used by API dry-run/validation and CLI `config preview`/`config validate`. It canonicalizes the workspace and checks project scope, configured direct/fallback profile registration and scope, persisted capability evidence, and stage route resolution without starting a workflow or probing a provider.
- CLI blocked responses include an explicit `NOT_RUN` preview and blocked reasons; API preview and validation errors include the same intent and reason data in the standard error envelope. Tests cover unauthorized direct/fallback routes, missing profiles, unprobed capability, project boundary, and zero run creation.
- `go test ./internal/routing ./internal/api ./internal/cli -count=1`, package vet, and `git diff --check` passed. T100 is complete; T101 is next. T087/T088 remain partial.

### T101 — Finalize heartbeat ownership loss safely — 2026-10-23

- Worker contexts now retain cancellation causes. Heartbeat failure is classified as `WORKSPACE_OWNERSHIP_LOST`, and persistence failures caused by canceled worker contexts finalize as `CANCELLED` instead of being misreported as `FAILED`.
- Cleanup releases the workspace database lease only when the same owner ID still holds it. A replacement owner remains fenced until stale-terminal recovery confirms the lease expired and the process lock is no longer held. The regression test repeats eight scenarios and checks terminal state, ownership cause, process-lock release, retained replacement-owner fence, safe stale recovery, and a stable event count after cancellation.
- `go test -race ./internal/workflow -run TestHeartbeatOwnershipLossCancelsActiveWorker -count=2`, `go test ./internal/workflow ./internal/store -count=1`, focused repeat after the final process-lock assertion, `go vet ./internal/workflow ./internal/store`, and `git diff --check` passed. T101 is complete; T102 is next. T087/T088 remain partial.

### T102 — Re-run the SC-006 SSE load acceptance profile — 2026-10-23

- The backlog fixture now starts at `EventHighWater+1`, retains its batch-persisted UTC timestamp, and computes buffer/expected counts from the actual persisted high-water mark plus live frames. The reader still rejects missing/duplicate sequences and counts every non-heartbeat frame.
- Fresh `make load-test-sse` passed: 10 clients; 100 events/s for 100 seconds; 10,000 backlog rows; 200,020 contiguous non-heartbeat frames; all-frame p95 269.75 ms; live-frame p95 285.043 ms. SC-006 is again current PASS evidence, replacing the prior setup failure and older pass.
- `make load-test-sse`, `go test ./internal/api -count=1`, `go vet ./internal/api`, and `git diff --check` are the phase validation. T102 is complete; T103 is next. T087/T088 remain partial.

### T105 — Reconcile current documentation — 2026-10-23

- Updated the README and provider guide to describe fixture-backed adapters, the supported provider-specific profile probes, `doctor`'s executable-presence scope, fail-closed unavailable capabilities, and the distinction between required offline evidence and optional real-account smoke acceptance.
- Corrected the acceptance ledger and final-verification report: live/mixed runs do not require the simulation-only E2E gate; T098 retention and sanitized JSON `spectide report RUN_ID` output are documented; both official convergence passes and the current T087/T088/T103–T106 states are visible. No additional export format is implied.
- Preserved older progress entries as dated historical checkpoints and added this correction as a new dated entry. A repo-wide `rg` scan of current docs found no remaining stale current-tense claims; `git diff --check` passed. T105 is complete; T106 release preparation and T104 final verification remain. T103 remains blocked by Chrome's saved-origin policy; T087/T088 remain partial.

### T106 — Prepare local release artifacts — 2026-10-23

- Added `make release-prep VERSION=...`, which rebuilds the embedded dashboard, cross-compiles four static targets (macOS arm64/amd64 and Linux arm64/amd64), injects the requested version, creates normalized deterministic archives, and writes build metadata plus SHA256 checksums. A manual `workflow_dispatch` workflow prepares and retains artifacts without creating a release or publishing packages. Release instructions document toolchain, matrix, verification limits, and owner authorization requirements.
- `make release-prep VERSION=1.0.0-rc.1` passed on macOS 15.1/Apple M4 with Go 1.27.2 and Node 26.12.0. All four archives built; checksum verification and binary format inspection passed. A final clean-tree build recorded its exact commit and `working_tree_clean=true`; host arm64 version, doctor, `/healthz`, embedded dashboard HTML/JS, and offline demo smoke passed within the two-second health limit. Script/YAML validation, CLI tests/vet, and diff checks passed. Artifacts were prepared locally only, not published.
- T106 is complete. T104 remains for full final verification; T103 remains blocked by the Chrome origin policy; T087/T088 remain partial. See [release preparation](release-preparation.md) and [final verification](final-verification.md).
