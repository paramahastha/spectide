# Tasks: Build SpecTide v1

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), and [contracts/](contracts/)

**Tests**: Required by FR-028 and FR-029. Every story has automated acceptance coverage.

## Phase 0: Specification Analysis Gate

- [x] T044 Run Spec-Kit analyze as a strictly read-only command before implementation; route findings to owning work only after the command exits

## Phase 1: Setup

- [x] T001 [US5] Create the Go module and base repository structure in `go.mod`, `cmd/`, `internal/`, `docs/`, `examples/`, and `testdata/`
- [x] T002 [P] Create the React/TypeScript dashboard project in `web/package.json`, `web/vite.config.ts`, and `web/src/`
- [x] T003 [P] Create formatting, lint, build, test, demo, and smoke targets in `Makefile` and `scripts/`
- [x] T004 [P] Add the initial public repository metadata (`README.md`, `LICENSE`) and minimal CI workflow scaffold in `.github/workflows/`; T041 owns substantive contribution/security policy and final CI hardening

## Phase 2: Foundational

- [x] T005 Define canonical run, stage, event, and quality types in `internal/model/model.go`
- [x] T006 Implement transactional SQLite persistence, WAL configuration, event sequencing, and report queries in `internal/store/store.go`
- [x] T007 Create versioned migration files and migration tests in `internal/store/migrations/` and `internal/store/`
- [x] T050 Persist immutable effective run configuration and append-only stage-attempt history, with mutation/retry tests for FR-003, in `internal/store/` and `internal/workflow/`
- [x] T051 Implement backup/restore with schema compatibility checks; expire individual event details 90 days after their timestamps; prune details from the oldest terminal runs first at the 1,000-detailed-history cap; preserve summaries, outcomes, routes, verdicts, approvals, and sanitized report metadata; reject creation if all detailed-history slots are active; add recovery tests for FR-027 in `internal/store/`
- [x] T047 Create checkpoint, artifact, approval, routing-decision, and workspace-lock persistence repositories with transaction tests in `internal/store/`
- [x] T008 [P] Implement configuration precedence, profile registry, and validation in `internal/config/`
- [x] T009 [P] Implement workspace canonicalization, allowed roots, and writer locks in `internal/workspace/`
- [x] T010 [P] Implement redaction, process policy, and approval models in `internal/security/` and `internal/events/`
- [x] T011 Implement strict workflow transition validation and recovery checkpoints in `internal/workflow/` (depends on T047 persistence repositories)

## Phase 3: User Story 1 — Complete development workflow (P1)

**Goal**: Submit a task and receive a durable full simulated lifecycle and evidence report.

**Independent Test**: A fake-provider run reaches `COMPLETED` with all canonical stages and required simulated gates persisted.

- [x] T012 [P] [US1] Add deterministic lifecycle acceptance tests that verify simulated stage results are labeled and never claim official Spec-Kit invocation success in `internal/workflow/engine_test.go`
- [x] T013 [US1] Implement deterministic fake stage execution and quality evidence, keeping simulated outcomes visibly distinct from official Spec-Kit command results, in `internal/workflow/engine.go`
- [x] T014 [US1] Implement Spec-Kit discovery, canonical-stage invocation only through verified installed commands, blocked outcomes for missing or unusable integrations, artifact inspection, and analyze/converge parsing in `internal/speckit/`
- [x] T015 [US1] Implement required test registry and evidence collection in `internal/quality/`
- [x] T016 [US1] Implement the FR-009 defaults of at most 3 analyze remediation rounds (excluding the initial baseline; only critical/high findings and medium findings that leave security, acceptance, or prerequisites unsatisfied block; a round is one owning-artifact change followed by one read-only re-analysis), 4 test repair attempts, and 5 convergence rounds per run; add tests for each configured default, each exhaustion boundary, and the no-progress stop condition in `internal/workflow/`
- [x] T017 [US1] Implement CLI run, status, list, logs, report, and exit-code behavior in `internal/cli/`

## Phase 4: User Story 2 — Authorized profile routing (P1)

**Goal**: Resolve and audit safe manual, hybrid, and automatic stage routes.

**Independent Test**: Valid configured profiles resolve per stage; invalid capabilities and forbidden profile boundaries cannot start.

- [x] T018 [P] [US2] Add typed adapter errors and invocation-policy contract tests around the existing shared `Adapter` interface in `internal/adapters/`
- [x] T019 [P] [US2] Implement fake adapter scripted failures, cancellation, and rate limits against the shared `Adapter` interface in `internal/adapters/fake/`
- [x] T020 [P] [US2] Implement Claude fixture adapter and capability probe against the shared `Adapter` interface in `internal/adapters/claude/`
- [x] T021 [P] [US2] Implement Codex fixture adapter and capability probe against the shared `Adapter` interface in `internal/adapters/codex/`
- [x] T022 [P] [US2] Implement OpenCode fixture adapter and capability probe against the shared `Adapter` interface in `internal/adapters/opencode/`
- [x] T023 [US2] Implement safe subprocess runner with explicit arguments, restricted environment, cancellation, and a retained-output cap of 1 MiB each for stdout and stderr in `internal/runner/`
- [x] T053 [US2] Implement provider stage timeout defaults, configurable duration bounds, and graceful termination before forced process-group cleanup in `internal/runner/`
- [x] T048 [US2] Add runner tests for the 1 MiB output caps, configurable 1-second–4-hour stage timeout (30-minute default), and 2-second graceful process-group cancellation cleanup in `internal/runner/`
- [x] T024 [US2] Implement routing modes, capability validation, fallback authorization, decision audit trail, and dry-run intent resolution; test that previews create no run and that unauthorized direct/fallback routes are denied before any repository content crosses the project/profile boundary in `internal/routing/`
- [x] T025 [US2] Add profile/model commands and offline probe output in `internal/cli/`

## Phase 5: User Story 3 — Live local observation and control (P1)

**Goal**: Observe real persisted state and safely control a run through CLI and local dashboard.

**Independent Test**: Two clients receive/replay the same ordered events; cancellation and approval actions persist correctly.

- [x] T026 [P] [US3] Implement baseline loopback API, CSRF checks, run routes, report routes, and SSE replay in `internal/api/server.go`
- [x] T027 [P] [US3] Implement an embedded local dashboard shell in `internal/webassets/`
- [x] T028 [US3] Add API request validation, idempotency keys, project/profile/config/approval endpoints, and structured errors in `internal/api/`
- [x] T029 [US3] Add SSE high-water replay, heartbeats, slow-client backpressure, and integration tests in `internal/api/`
- [x] T029a [US3] Add Host/Origin/CSRF, path-escape, request-size, and idempotency security tests in `internal/api/`
- [x] T030 [US3] Implement React dashboard API client, run list/detail, live events, controls, profiles, routing and dry-run preview views, and quality views in `web/src/`
- [ ] T031 [US3] Add dashboard component and browser end-to-end tests for every page/state pair defined by FR-026 in `spec.md`; run the automated accessibility scan on at least one rendered state of each of the seven named surfaces; include disconnected, blocked, skipped, failed, not-run, and simulated text/symbol status labels; verify keyboard/focus, 4.5:1 text contrast, 200% zoom, reduced motion, and zero critical/serious scan violations; record the page/state matrix and scan output in `docs/accessibility-audit.md`

## Phase 6: User Story 4 — Safe failure recovery (P1)

**Goal**: Recover correctly from process failure, convergence work, cancellation, and restart without false success.

**Independent Test**: Failure-injection scenarios preserve attempts/events, obey bounds, and finish blocked/cancelled or recovered correctly.

- [x] T032 [P] [US4] Add state-machine transition and workspace lock contention tests in `internal/workflow/` and `internal/workspace/`
- [x] T033 [P] [US4] Add malformed stream, timeout, cancellation, and redaction adapter fixture tests in `internal/adapters/`, `internal/runner/`, and `internal/security/`
- [x] T034 [US4] Implement pause/resume/cancel at safe stage boundaries and checkpoint recovery; test resume only after canonical workspace identity and exclusive lock checks, immutable configuration hash match, checkpoint identity match, checkpointed artifact path/content hashes match, and Git HEAD plus managed-path hashes match; on mismatch preserve evidence, block with `WORKSPACE_DRIFT`, and require explicit verified recovery without overwriting in `internal/workflow/`
- [x] T035 [US4] Implement append-only convergence task validation and final-review return loop, with tests proving remediation/retries preserve the same product specification path and never create a replacement spec, in `internal/speckit/` and `internal/workflow/`
- [x] T036 [US4] Add recovery and secret-leak end-to-end scenarios; verify exhaustion at 3 analyze remediation rounds, 4 test repair attempts, and 5 convergence rounds, plus no-progress termination, in `internal/workflow/` and `internal/api/`
- [x] T049 [US4] Add a 20-run deterministic simulated lifecycle acceptance test proving SC-002 in `internal/workflow/`

## Phase 7: User Story 5 — Local installation and validation (P2)

**Goal**: A contributor can build, test, run the demo, and understand limitations without credentials.

**Independent Test**: A clean offline build runs the documented demo and its report through the local dashboard.

- [x] T037 [P] [US5] Implement Cobra root command and the `doctor`, `init`, `serve`, `dashboard`, `demo`, configuration, project, profile/model, approval, and run pause/resume/cancel control commands in `cmd/spectide/` and `internal/cli/`; T017 owns run/status/list/logs/report and exit-code commands
- [x] T038 [P] [US5] Create fake-provider demo workspace and deterministic fixtures in `examples/fake-provider-demo/` and `testdata/`
- [x] T039 [US5] Add offline smoke validation and assert `/healthz` responds within the SC-001 2-second startup threshold; capture exact OS, CPU model, installed RAM, build command, and monotonic process-start-to-first-HTTP-200 elapsed time for the verification report in `scripts/smoke.sh`, `scripts/demo.sh`, and `Makefile`
- [x] T040 [US5] Write verified user, architecture, configuration, provider, security, recovery, troubleshooting, and OpenAPI documentation in `docs/`
- [x] T041 [US5] Complete the contribution/security policy, changelog, CI verification steps, and no-secret checks in root policy files and `.github/workflows/`, extending T004’s initial scaffold

## Phase 8: Cross-cutting verification and convergence

- [x] T042 [P] Link FR-001 through FR-030 to automated tests or documented inspected evidence in `docs/acceptance-ledger.md`
- [x] T044a Rerun read-only Spec-Kit analyze at the post-implementation extension quality gate before proceeding to convergence
- [x] T054 After each read-only analyze command exits, append its exact report, timestamp, and any follow-up resolution references to `specs/001-build-spectide/analysis.md`; this documentation task is outside the read-only analyze stage
- [x] T043 [P] Run format, static analysis, unit, integration, fake E2E, security, dashboard, and smoke checks through `Makefile` and CI
- [x] T043a [P] Measure SC-006 using one local server, 10 concurrent SSE clients, 100 events/second for 100 seconds, and a 10,000-event persisted backlog; timestamp each event at persistence and each frame at client receipt, calculate p95 over delivered non-heartbeat frames after excluding connection setup, and fail if any client misses a persisted event or p95 exceeds 1 second in `internal/api/`
- [ ] T045 Run implementation verification and official converge; implement appended work and repeat until converged or a documented blocker in `docs/implementation-progress.md`
- [ ] T046 Capture actual dashboard screenshots and publish final validation/release-readiness report listing executed commands, evidence locations, PASS/FAIL/BLOCKED/NOT_RUN checks, limitations, and security/provider constraints; write the screenshot evidence index to `docs/screenshots/README.md` and link `docs/accessibility-audit.md` from that index and `docs/final-verification.md`

## Dependencies & Execution Order

`spec.md + plan.md + tasks.md → T044 → T001–T004 → T005–T011/T047/T050/T051 → US1–US5 → T042–T044a → T054 → T043/T043a → T045–T046`. T044 is the read-only analysis gate and runs after task generation but before implementation; findings are resolved in owning work after that command exits. T044a repeats the read-only gate after the current implementation extension and before convergence. T054 records its completed output only after the command exits. Adapter implementations proceed against the shared interface; T018's typed error and invocation-policy contract tests are complete. T053 implements the numerical process limits before T048 verifies them.

## Implementation Strategy

Complete the deterministic, evidence-backed lifecycle first; add real adapters only with fixture contracts and fail-closed probes; then expose durable state through API/SSE and the dashboard. No task may be checked complete without its stated automated evidence.

## Phase 9: Convergence

- [x] T055 CRITICAL — Close unredacted persistence and response paths in `internal/store/store.go`, `internal/store/evidence.go`, and `internal/api/server.go`: sanitize gate evidence, approval actions, capability/version metadata, configuration-derived strings, and errors before persistence/export/display; preserve safe execution identities separately from presentation redaction; add synthetic-secret round-trip tests through SQLite, reports, CLI, and API per Constitution I and security constraints, FR-023, and SC-005
- [x] T056 CRITICAL — Fence canonical workspace ownership in `internal/store/owners.go`, `internal/workflow/engine.go`, and `internal/workspace/`: do not release/reassign a writer solely because its heartbeat expired while its provider may still be running; stop or fence workers on ownership loss, verify process identity before recovery, and prevent conflicting writers using different SQLite databases; add live-child, stalled-heartbeat, ownership-loss, and cross-database contention tests per Constitution IV, FR-004, and FR-019
- [x] T057 CRITICAL — Commit stage transitions, approval creation/resolution, routing and quality mutations with their associated ordered events in the same transaction in `internal/store/`; update workflow callers to use these operations and propagate persistence failures; add rollback/fault-injection and replay tests proving no observable state change lacks its event per Constitution IV, FR-018, US3/AC3, and plan: transactional event model
- [x] T058 CRITICAL — Record a separate durable attempt for every provider invocation and remediation/review/convergence retry in `internal/workflow/`, and persist append-only test execution and quality evidence in `internal/store/` and migrations; retain route, iteration, timing, process identity, result, and evidence links without overwriting prior results; verify reports expose complete histories per Constitution IV, FR-002, FR-003, T050, and plan: data model
- [x] T059 CRITICAL — Require stage-specific semantic outcomes and expected artifact evidence before live stage success in `internal/workflow/live_stage.go` and `internal/workflow/engine.go`; safely inspect and register generated artifacts through `SaveArtifact`, link hashes and results to attempts/reports, and block no-op or missing-output stages even on exit zero; add artifact-missing, symlink-escape, and evidence-reporting acceptance tests per Constitution II, FR-006, FR-010, and US1/AC2–3
- [x] T060 CRITICAL — Harden analysis/convergence parsing and append validation in `internal/speckit/parse.go`: reject heading-only, truncated, malformed, unknown/contradictory reports and quoted success markers; require an unambiguous complete outcome and valid traceable unchecked task additions in one next-numbered Convergence phase with consecutive unused IDs; test realistic command outputs and report/ledger mismatches per Constitution II and III, FR-008, FR-009, and T035
- [x] T061 CRITICAL — Enforce read-only analysis and owner-only remediation across the affected workspace in `internal/workflow/analyze_loop.go` and `internal/workflow/live_stage.go`, not only hashes of the three canonical artifacts; support explicitly referenced plan design contracts as owners, prevent unrelated source/design writes, and avoid wrapping an artifact-only repair in instructions that authorize general implementation; add non-owner source/contract mutation tests per Constitution III, FR-008, and T016
- [x] T062 CRITICAL — Bind provider launch to approved canonical executable and configuration references in `internal/security/process.go`, `internal/runner/`, `internal/adapters/`, and profile persistence; validate symlink targets, executable permissions and identity, configuration boundaries, and working-directory identity immediately before launch; invalidate capability evidence on reference drift and test executable/config symlink escapes and replacement after probing per Constitution security constraints, FR-013, FR-017, and spec: symlink edge case
- [x] T063 CRITICAL — Implement the initial live artifact lifecycle in `internal/workflow/engine.go` and `internal/speckit/`: allow an initialized repository without feature artifacts to run constitution only when absent, specify exactly once, then create plan/tasks in their owning stages; retain and checkpoint the chosen product specification across retries/resume rather than requiring all three artifacts before specify or recreating an existing scope; add fresh-workspace and existing-constitution/spec acceptance tests per FR-005, FR-007, and US1/AC1
- [x] T064 Freeze the complete effective run execution configuration in `internal/config/`, `internal/store/`, and `internal/workflow/`, including approved profile references, required-check definitions, loop limits, and stage timeouts; wire configured bounds through adapter requests into every runner invocation and restore the same immutable settings after restart; test the 3/4/5 defaults, timeout default/range, overrides, and configuration/profile drift per FR-003, FR-009, FR-017, T048, and T053
- [x] T065 Implement supported credential-safe provider capability discovery in `internal/adapters/` and `internal/cli/` that can establish authenticated readiness and verified installed command/safety/model/effort capabilities without reading or copying credentials; validate selected model and effort values rather than only override support, persist verified observations, and keep unsupported/unavailable probes explicitly blocked; add anonymized success/failure/version fixtures and an offline configured-launch acceptance path per FR-012, FR-013, FR-014, T020–T022, and T025 (partial)
- [x] T066 Replace unconditional live-run blocking in `internal/workflow/engine.go` with explicit evidence-driven final acceptance and report generation; distinguish simulation-only checks from required live/local checks, complete only when all applicable required gates have inspectable passing evidence, and keep unavailable gates blocked/not-run without fabricating results; add complete non-simulated fixture and mixed-route acceptance tests per FR-005, FR-010, US1, and SC-003 (partial)
- [x] T067 Stream bounded normalized provider events while processes execute in `internal/runner/`, `internal/adapters/`, and `internal/speckit/invoke.go`; implement provider-specific supported JSONL framing and terminal-result extraction so analyze/converge parse extracted report text rather than raw JSON envelopes; reject malformed/unsupported versions and typed provider failures without exposing payload bodies or hidden reasoning; add realistic multi-line, partial-frame, error, and oversized-stream fixtures per FR-012, FR-021, FR-023, and T033 (partial)
- [x] T068 Configure and execute project-appropriate required quality checks in `internal/quality/` and `internal/workflow/quality_loop.go` instead of assuming every target has SpecTide's `cmd/spectide` Go layout; retain safe explicit argument vectors and the browser execution boundary, persist actual command/timing/result evidence, and test Go, non-Go, missing-command, and unsupported-project outcomes per FR-010, T015, and US1/AC3 (partial)
- [x] T069 Connect pending sensitive-action approvals to fail-closed workflow waiting and authorized continuation in `internal/workflow/`, `internal/store/`, `internal/api/`, and `web/src/App.tsx`; provide keyboard-operable approve/deny controls, bind each decision to its run/action and pending state, reject resolving another run's approval through a mismatched URL, and test durable decisions, denial, restart, and duplicate resolution per FR-019, FR-024, US3/AC3, and T010 (partial)
- [x] T070 Honor durable pause and cancellation requests between individual adapter invocations inside analyze/test-repair/convergence/final-review loops in `internal/workflow/`; persist result/evidence/checkpoint before pausing and poll cross-process cancellation during registered local checks so cleanup is bounded; test pausing after the first inner invocation and cancelling a long-running local check from a second controller per FR-017, FR-019, and T034 (partial)
- [x] T071 Keep restart-resumed CLI work alive in `internal/cli/root.go` by waiting for the resumed worker or handing ownership to a running local daemon before closing the store/exiting; add a subprocess-level restart/resume test proving continued execution and durable terminal evidence without an immediate closed database or orphaned worker per FR-001, FR-019, and T034 (partial)
- [x] T072 Initialize and preserve an explicit required-gate inventory for every run and terminal outcome in `internal/workflow/` and `internal/store/`; report PASS, FAIL, BLOCKED, and NOT_RUN distinctly for missing integrations, early provider failures, cancellation, interruption, and successful simulation/live fixtures rather than omitting unfinished gates; assert the full matrix in terminal reports per FR-010, SC-003, US1/AC3, and T015 (partial)
- [x] T073 Repair dashboard event and connection handling in `web/src/App.tsx`: consume all canonical event types including provider activity, failures, approvals, and quality updates; refresh canonical snapshots from received events, handle EventSource open/error/reconnect state, retain cursor-aware deduplication, and clear disconnected state on recovery; add two-viewer reconnect tests proving ordered missed events and identical canonical state per FR-021, FR-024, US3/AC1–2, and SC-004 (component coverage complete; browser E2E remains T077)
- [ ] T074 Derive CLI/dashboard execution labels from immutable persisted routing and stage evidence in `internal/cli/root.go`, `web/src/App.tsx`, and dashboard domain types; remove unconditional simulated/live-disabled claims for configurable project runs, label simulated history entries independently of selection, display effort alongside provider/profile/model/attempt, and show disconnected activity truthfully; test fake, live, mixed, blocked, and disconnected cases per FR-024, FR-025, US2/AC1, and US3/AC1 (contradicts)
- [ ] T075 Complete the specified local HTTP interface in `internal/api/` and its contract tests: expose capability discovery results and configuration validation, provide bounded/paginated run history with the dashboard's planned 100-most-recent default, make stage/artifact/report projections explicit, validate exact resource paths/methods, and use the standardized contract error envelope with request identity and sanitized details; keep `docs/openapi.yaml` consistent per FR-020, FR-022, T028, plan: scale/scope, and contracts/http-api.md (partial)
- [ ] T076 Wire deterministic simulated failure scenarios into complete engine runs in `internal/workflow/` rather than leaving them only in `internal/adapters/fake/` fixtures; exercise provider failure/rate limit, failed tests and repair, appended convergence tasks, bounded exhaustion/no-progress, interruption, and workspace contention while preserving visibly SIMULATED evidence; add end-to-end lifecycle assertions per FR-011, FR-029, US4/AC1–3, T019, and T036 (missing)
- [ ] T077 Complete browser component/E2E acceptance for every FR-026 page/state pair in `web/`, including actual app interaction, two clients, empty/error/disconnected behavior, keyboard/focus, rendered 4.5:1 text contrast, actual 200% zoom, reduced motion, and zero critical/serious accessibility scan violations; record the rendered state matrix and scan output in `docs/accessibility-audit.md`, using an authorized browser environment and retaining explicit blockers when unavailable per FR-026, FR-029, and T031 (partial)
- [ ] T078 Complete the remaining verification and release-readiness evidence after the convergence implementation pass: update `docs/acceptance-ledger.md`, record this convergence outcome and follow-up verification in `docs/implementation-progress.md`, capture actual authorized-browser screenshots with `docs/screenshots/README.md`, and finalize `docs/final-verification.md` with executed commands, evidence links, all verdicts, limitations, and accessibility links; rerun required changed-area checks and official convergence, document unavailable live-provider checks as NOT_RUN rather than requiring credentials, and prepare any missing release artifacts/workflows without publishing per FR-028, FR-030, SC-008, T045, and T046 (partial)
- [ ] T079 Align the dashboard verification runtime in `.github/workflows/ci.yml` and contributor/build documentation with the planned Node 26 toolchain; run frontend typecheck, tests, and production build on that runtime and retain Go 1.27 verification per plan: technical stack, FR-028, and T041 (contradicts)
