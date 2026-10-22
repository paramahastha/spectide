# Tasks: Build SpecTide v1

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), and [contracts/](contracts/)

**Tests**: Required by FR-028 and FR-029. Every story has automated acceptance coverage.

## Phase 0: Specification Analysis Gate

- [x] T044 Run Spec-Kit analyze as a strictly read-only command before implementation; route findings to owning work only after the command exits

## Phase 1: Setup

- [x] T001 [US5] Create the Go module and base repository structure in `go.mod`, `cmd/`, `internal/`, `docs/`, `examples/`, and `testdata/`
- [x] T002 [P] Create the React/TypeScript dashboard project in `web/package.json`, `web/vite.config.ts`, and `web/src/`
- [ ] T003 [P] Create formatting, lint, build, test, demo, and smoke targets in `Makefile` and `scripts/`
- [x] T004 [P] Add the initial public repository metadata (`README.md`, `LICENSE`) and minimal CI workflow scaffold in `.github/workflows/`; T041 owns substantive contribution/security policy and final CI hardening

## Phase 2: Foundational

- [x] T005 Define canonical run, stage, event, and quality types in `internal/model/model.go`
- [x] T006 Implement transactional SQLite persistence, WAL configuration, event sequencing, and report queries in `internal/store/store.go`
- [x] T007 Create versioned migration files and migration tests in `internal/store/migrations/` and `internal/store/`
- [x] T050 Persist immutable effective run configuration and append-only stage-attempt history, with mutation/retry tests for FR-003, in `internal/store/` and `internal/workflow/`
- [ ] T051 Implement backup/restore with schema compatibility checks; expire individual event details 90 days after their timestamps; prune details from the oldest terminal runs first at the 1,000-detailed-history cap; preserve summaries, outcomes, routes, verdicts, approvals, and sanitized report metadata; reject creation if all detailed-history slots are active; add recovery tests for FR-027 in `internal/store/`
- [x] T047 Create checkpoint, artifact, approval, routing-decision, and workspace-lock persistence repositories with transaction tests in `internal/store/`
- [ ] T008 [P] Implement configuration precedence, profile registry, and validation in `internal/config/`
- [ ] T009 [P] Implement workspace canonicalization, allowed roots, and writer locks in `internal/workspace/`
- [ ] T010 [P] Implement redaction, process policy, and approval models in `internal/security/` and `internal/events/`
- [ ] T011 Implement strict workflow transition validation and recovery checkpoints in `internal/workflow/` (depends on T047 persistence repositories)

## Phase 3: User Story 1 — Complete development workflow (P1)

**Goal**: Submit a task and receive a durable full simulated lifecycle and evidence report.

**Independent Test**: A fake-provider run reaches `COMPLETED` with all canonical stages and required simulated gates persisted.

- [x] T012 [P] [US1] Add deterministic lifecycle acceptance tests that verify simulated stage results are labeled and never claim official Spec-Kit invocation success in `internal/workflow/engine_test.go`
- [x] T013 [US1] Implement deterministic fake stage execution and quality evidence, keeping simulated outcomes visibly distinct from official Spec-Kit command results, in `internal/workflow/engine.go`
- [ ] T014 [US1] Implement Spec-Kit discovery, canonical-stage invocation only through verified installed commands, blocked outcomes for missing or unusable integrations, artifact inspection, and analyze/converge parsing in `internal/speckit/`
- [x] T015 [US1] Implement required test registry and evidence collection in `internal/quality/`
- [ ] T016 [US1] Implement the FR-009 defaults of at most 3 analyze remediation rounds (excluding the initial baseline; only critical/high findings and medium findings that leave security, acceptance, or prerequisites unsatisfied block; a round is one owning-artifact change followed by one read-only re-analysis), 4 test repair attempts, and 5 convergence rounds per run; add tests for each configured default, each exhaustion boundary, and the no-progress stop condition in `internal/workflow/`
- [ ] T017 [US1] Implement CLI run, status, list, logs, report, and exit-code behavior in `internal/cli/`

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
- [ ] T024 [US2] Implement routing modes, capability validation, fallback authorization, decision audit trail, and dry-run intent resolution; test that previews create no run and that unauthorized direct/fallback routes are denied before any repository content crosses the project/profile boundary in `internal/routing/`
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
- [ ] T034 [US4] Implement pause/resume/cancel at safe stage boundaries and checkpoint recovery; test resume only after canonical workspace identity and exclusive lock checks, immutable configuration hash match, checkpoint identity match, checkpointed artifact path/content hashes match, and Git HEAD plus managed-path hashes match; on mismatch preserve evidence, block with `WORKSPACE_DRIFT`, and require explicit verified recovery without overwriting in `internal/workflow/`
- [ ] T035 [US4] Implement append-only convergence task validation and final-review return loop, with tests proving remediation/retries preserve the same product specification path and never create a replacement spec, in `internal/speckit/` and `internal/workflow/`
- [ ] T036 [US4] Add recovery and secret-leak end-to-end scenarios; verify exhaustion at 3 analyze remediation rounds, 4 test repair attempts, and 5 convergence rounds, plus no-progress termination, in `internal/workflow/` and `internal/api/`
- [x] T049 [US4] Add a 20-run deterministic simulated lifecycle acceptance test proving SC-002 in `internal/workflow/`

## Phase 7: User Story 5 — Local installation and validation (P2)

**Goal**: A contributor can build, test, run the demo, and understand limitations without credentials.

**Independent Test**: A clean offline build runs the documented demo and its report through the local dashboard.

- [x] T037 [P] [US5] Implement Cobra root command and the `doctor`, `init`, `serve`, `dashboard`, `demo`, configuration, project, profile/model, approval, and run pause/resume/cancel control commands in `cmd/spectide/` and `internal/cli/`; T017 owns run/status/list/logs/report and exit-code commands
- [ ] T038 [P] [US5] Create fake-provider demo workspace and deterministic fixtures in `examples/fake-provider-demo/` and `testdata/`
- [x] T039 [US5] Add offline smoke validation and assert `/healthz` responds within the SC-001 2-second startup threshold; capture exact OS, CPU model, installed RAM, build command, and monotonic process-start-to-first-HTTP-200 elapsed time for the verification report in `scripts/smoke.sh`, `scripts/demo.sh`, and `Makefile`
- [ ] T040 [US5] Write verified user, architecture, configuration, provider, security, recovery, troubleshooting, and OpenAPI documentation in `docs/`
- [ ] T041 [US5] Complete the contribution/security policy, changelog, CI verification steps, and no-secret checks in root policy files and `.github/workflows/`, extending T004’s initial scaffold

## Phase 8: Cross-cutting verification and convergence

- [x] T042 [P] Link FR-001 through FR-030 to automated tests or documented inspected evidence in `docs/acceptance-ledger.md`
- [x] T044a Rerun read-only Spec-Kit analyze at the post-implementation extension quality gate before proceeding to convergence
- [x] T054 After each read-only analyze command exits, append its exact report, timestamp, and any follow-up resolution references to `specs/001-build-spectide/analysis.md`; this documentation task is outside the read-only analyze stage
- [ ] T043 [P] Run format, static analysis, unit, integration, fake E2E, security, dashboard, and smoke checks through `Makefile` and CI
- [ ] T043a [P] Measure SC-006 using one local server, 10 concurrent SSE clients, 100 events/second for 100 seconds, and a 10,000-event persisted backlog; timestamp each event at persistence and each frame at client receipt, calculate p95 over delivered non-heartbeat frames after excluding connection setup, and fail if any client misses a persisted event or p95 exceeds 1 second in `internal/api/`
- [ ] T045 Run implementation verification and official converge; implement appended work and repeat until converged or a documented blocker in `docs/implementation-progress.md`
- [ ] T046 Capture actual dashboard screenshots and publish final validation/release-readiness report listing executed commands, evidence locations, PASS/FAIL/BLOCKED/NOT_RUN checks, limitations, and security/provider constraints; write the screenshot evidence index to `docs/screenshots/README.md` and link `docs/accessibility-audit.md` from that index and `docs/final-verification.md`

## Dependencies & Execution Order

`spec.md + plan.md + tasks.md → T044 → T001–T004 → T005–T011/T047/T050/T051 → US1–US5 → T042–T044a → T054 → T043/T043a → T045–T046`. T044 is the read-only analysis gate and runs after task generation but before implementation; findings are resolved in owning work after that command exits. T044a repeats the read-only gate after the current implementation extension and before convergence. T054 records its completed output only after the command exits. Adapter implementations proceed against the shared interface; T018's typed error and invocation-policy contract tests are complete. T053 implements the numerical process limits before T048 verifies them.

## Implementation Strategy

Complete the deterministic, evidence-backed lifecycle first; add real adapters only with fixture contracts and fail-closed probes; then expose durable state through API/SSE and the dashboard. No task may be checked complete without its stated automated evidence.
