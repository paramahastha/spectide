# Tasks: Build SpecTide v1

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), and [contracts/](contracts/)

**Tests**: Required by FR-028 and FR-029. Every story has automated acceptance coverage.

## Phase 0: Specification Analysis Gate

- [x] T044 Run Spec-Kit analyze before implementation, resolve blocking findings in their owning artifacts, and record evidence in `specs/001-build-spectide/analysis.md`

## Phase 1: Setup

- [x] T001 Create the Go module and base repository structure in `go.mod`, `cmd/`, `internal/`, `docs/`, `examples/`, and `testdata/`
- [x] T002 [P] Create the React/TypeScript dashboard project in `web/package.json`, `web/vite.config.ts`, and `web/src/`
- [ ] T003 [P] Create formatting, lint, build, test, demo, and smoke targets in `Makefile` and `scripts/`
- [x] T004 [P] Create public repository metadata and CI workflows in `README.md`, `LICENSE`, `.github/workflows/`, and root policy files

## Phase 2: Foundational

- [x] T005 Define canonical run, stage, event, and quality types in `internal/model/model.go`
- [x] T006 Implement transactional SQLite persistence, WAL configuration, event sequencing, and report queries in `internal/store/store.go`
- [x] T007 Create versioned migration files and migration tests in `internal/store/migrations/` and `internal/store/`
- [x] T047 Create checkpoint, artifact, approval, routing-decision, and workspace-lock persistence repositories with transaction tests in `internal/store/`
- [ ] T008 [P] Implement configuration precedence, profile registry, and validation in `internal/config/`
- [ ] T009 [P] Implement workspace canonicalization, allowed roots, and writer locks in `internal/workspace/`
- [ ] T010 [P] Implement redaction, process policy, and approval models in `internal/security/` and `internal/events/`
- [ ] T011 Implement strict workflow transition validation and recovery checkpoints in `internal/workflow/` (depends on T047 persistence repositories)

## Phase 3: User Story 1 — Complete development workflow (P1)

**Goal**: Submit a task and receive a durable full simulated lifecycle and evidence report.

**Independent Test**: A fake-provider run reaches `COMPLETED` with all canonical stages and required simulated gates persisted.

- [x] T012 [P] [US1] Add deterministic lifecycle acceptance test in `internal/workflow/engine_test.go`
- [x] T013 [US1] Implement deterministic fake stage execution and quality evidence in `internal/workflow/engine.go`
- [x] T014 [US1] Implement actual Spec-Kit discovery, stage mapping, artifact inspection, analyze and converge parsing in `internal/speckit/`
- [x] T015 [US1] Implement required test registry and evidence collection in `internal/quality/`
- [ ] T016 [US1] Implement bounded analyze remediation and converge no-progress loops in `internal/workflow/`
- [ ] T017 [US1] Implement CLI run, status, list, logs, report, and exit-code behavior in `internal/cli/`

## Phase 4: User Story 2 — Authorized profile routing (P1)

**Goal**: Resolve and audit safe manual, hybrid, and automatic stage routes.

**Independent Test**: Valid configured profiles resolve per stage; invalid capabilities and forbidden profile boundaries cannot start.

- [ ] T018 [P] [US2] Create adapter interface, typed errors, and invocation policy tests in `internal/adapters/`
- [ ] T019 [P] [US2] Implement fake adapter scripted failures, cancellation, and rate limits in `internal/adapters/fake/`
- [x] T020 [P] [US2] Implement Claude fixture adapter and capability probe in `internal/adapters/claude/`
- [x] T021 [P] [US2] Implement Codex fixture adapter and capability probe in `internal/adapters/codex/`
- [x] T022 [P] [US2] Implement OpenCode fixture adapter and capability probe in `internal/adapters/opencode/`
- [x] T023 [US2] Implement safe subprocess runner with explicit arguments, restricted environment, and cancellation in `internal/runner/`
- [ ] T048 [US2] Add runner timeout, bounded-output, and process-group cancellation cleanup tests in `internal/runner/`
- [ ] T024 [US2] Implement routing modes, capability validation, fallback authorization, and decision audit trail in `internal/routing/`
- [ ] T025 [US2] Add profile/model commands and offline probe output in `internal/cli/`

## Phase 5: User Story 3 — Live local observation and control (P1)

**Goal**: Observe real persisted state and safely control a run through CLI and local dashboard.

**Independent Test**: Two clients receive/replay the same ordered events; cancellation and approval actions persist correctly.

- [x] T026 [P] [US3] Implement baseline loopback API, CSRF checks, run routes, report routes, and SSE replay in `internal/api/server.go`
- [x] T027 [P] [US3] Implement an embedded local dashboard shell in `internal/webassets/`
- [x] T028 [US3] Add API request validation, idempotency keys, project/profile/config/approval endpoints, and structured errors in `internal/api/`
- [x] T029 [US3] Add SSE high-water replay, heartbeats, slow-client backpressure, and integration tests in `internal/api/`
- [x] T029a [US3] Add Host/Origin/CSRF, path-escape, request-size, and idempotency security tests in `internal/api/`
- [x] T030 [US3] Implement React dashboard API client, run list/detail, live events, controls, profiles, routing, and quality views in `web/src/`
- [ ] T031 [US3] Add dashboard component and browser end-to-end tests in `web/tests/` and `web/e2e/`

## Phase 6: User Story 4 — Safe failure recovery (P1)

**Goal**: Recover correctly from process failure, convergence work, cancellation, and restart without false success.

**Independent Test**: Failure-injection scenarios preserve attempts/events, obey bounds, and finish blocked/cancelled or recovered correctly.

- [x] T032 [P] [US4] Add state-machine transition and workspace lock contention tests in `internal/workflow/` and `internal/workspace/`
- [x] T033 [P] [US4] Add malformed stream, timeout, cancellation, and redaction adapter fixture tests in `internal/adapters/`, `internal/runner/`, and `internal/security/`
- [ ] T034 [US4] Implement pause/resume/cancel, process reconciliation, safe checkpoint resume, and drift detection in `internal/workflow/`
- [ ] T035 [US4] Implement append-only convergence task validation and final-review return loop in `internal/speckit/` and `internal/workflow/`
- [ ] T036 [US4] Add recovery, retry, no-progress, and secret-leak end-to-end scenarios in `internal/workflow/` and `internal/api/`
- [ ] T049 [US4] Add a 20-run deterministic simulated lifecycle acceptance test proving SC-002 in `internal/workflow/`

## Phase 7: User Story 5 — Local installation and validation (P2)

**Goal**: A contributor can build, test, run the demo, and understand limitations without credentials.

**Independent Test**: A clean offline build runs the documented demo and its report through the local dashboard.

- [ ] T037 [P] [US5] Implement Cobra root command, doctor, init, serve, dashboard, demo, and configuration command tree in `cmd/spectide/` and `internal/cli/`
- [ ] T038 [P] [US5] Create fake-provider demo workspace and deterministic fixtures in `examples/fake-provider-demo/` and `testdata/`
- [ ] T039 [US5] Add offline smoke validation in `scripts/smoke.sh`, `scripts/demo.sh`, and `Makefile`
- [ ] T040 [US5] Write verified user, architecture, configuration, provider, security, recovery, troubleshooting, and OpenAPI documentation in `docs/`
- [ ] T041 [US5] Add release-ready repository policy, contribution, security, changelog, CI, and no-secret checks in root files and `.github/`

## Phase 8: Cross-cutting verification and convergence

- [ ] T042 [P] Link FR-001 through FR-030 to automated evidence in `docs/acceptance-ledger.md`
- [ ] T043 [P] Run format, static analysis, unit, integration, fake E2E, security, dashboard, and smoke checks through `Makefile` and CI
- [ ] T043a [P] Measure local SSE visibility p95 and slow-client behavior with the 10,000-event fixture in `internal/api/`
- [ ] T045 Run implementation verification and official converge; implement appended work and repeat until converged or a documented blocker in `docs/implementation-progress.md`
- [ ] T046 Capture actual dashboard screenshots and publish final validation/release-readiness report in `docs/screenshots/` and `docs/final-verification.md`

## Dependencies & Execution Order

`spec.md + plan.md + tasks.md → T044 → T001–T004 → T005–T011 → US1/US2/US3 → US4 → US5 → T042–T043a → T045–T046`. T044 is the read-only analysis gate and runs after task generation but before implementation; blocking findings must be resolved in their owning work before implementation proceeds. US2 adapter work and the first embedded dashboard can proceed in parallel after foundational types; shared workflow contracts must not be parallel-written. The minimum demonstrable slice is US1 with T012–T017, then it must be analyzed before extension.

## Implementation Strategy

Complete the deterministic, evidence-backed lifecycle first; add real adapters only with fixture contracts and fail-closed probes; then expose durable state through API/SSE and the dashboard. No task may be checked complete without its stated automated evidence.
