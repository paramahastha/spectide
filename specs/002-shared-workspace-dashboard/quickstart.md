# Quickstart Validation: Shared Workspace Dashboard

Use a disposable local home/workspace for validation. Do not point these checks at a developer's real default workspace or a live provider account. The fake-provider fixture is sufficient for automated workflow coverage.

## Prerequisites

- Go 1.27.2 and Node/npm dependencies installed.
- A built `spectide` binary (`make build`).
- Two disposable existing repository directories with project-local fake-provider configurations, referred to below as `PROJECT_A` and `PROJECT_B`.
- A disposable home/data environment injected by the test harness so the default-workspace resolver cannot touch user state.

## 1. Static, unit, and UI checks

```sh
make format-check
make lint
make test
make web-test
make web-build
```

**Expected outcome**: all commands succeed; migration, resolver, CLI, API, workflow, and dashboard component tests report pass status. Rebuild/refresh embedded web assets before Go API/e2e checks that serve the dashboard.

## 2. First project and idempotent setup

```sh
cd "$PROJECT_A"
spectide setup
spectide setup
spectide projects
```

**Expected outcome**:

- Setup displays the canonical `PROJECT_A` path before configuration creation and requests confirmation.
- The first call creates/opens exactly one default workspace and registers one project.
- The second call reports an existing project/workspace without duplicating records or changing project configuration.
- Project listing shows a safe readiness summary and no credentials.

## 3. Add a second project and enforce registration

```sh
cd "$PROJECT_B"
spectide add .
spectide projects

mkdir -p "$UNREGISTERED_PROJECT"
cd "$UNREGISTERED_PROJECT"
spectide run "Verify registration boundary"
```

**Expected outcome**:

- Both `PROJECT_A` and `PROJECT_B` appear once, with distinct canonical paths and independent readiness.
- Neither project inherits the other's route, profile permissions, scopes, or configuration.
- The unregistered run fails before creating a run and prints its canonical path plus `spectide add /absolute/path` remediation.

## 4. Shared dashboard lifecycle and identity

```sh
cd "$PROJECT_A"
spectide dashboard
cd "$PROJECT_B"
spectide dashboard
```

**Expected outcome**:

- The first command starts/waits for a loopback dashboard at `http://127.0.0.1:4747`.
- The second command reuses the same endpoint only after its workspace ID matches the selected default workspace.
- A second default `serve` reports the existing URL and dashboard command rather than a raw bind error.
- A fixture occupying the port with a foreign/mismatched service receives an ownership-not-confirmed message and an isolated-port remedy; it is never called the SpecTide dashboard.
- In the integration harness, invoke `spectide dashboard` 100 times against one healthy matching workspace; record every invocation time, require at least 99 to reuse the existing URL within 5 seconds, and require zero duplicate default dashboard processes.

## 5. Project scope and workflow boundary

1. Start a fake-provider workflow from each registered project.
2. Open the dashboard and verify the initial **All projects** view contains both runs.
3. Select `PROJECT_A`, then `PROJECT_B`.
4. For each selected project, start a route preview and workflow from the dashboard.

**Expected outcome**:

- Each run records its stable project ID and canonical path.
- Selected-project history, counts, recent activity, route preview, and workflow start defaults contain only the selected project.
- A malformed or stale saved project selection returns to **All projects**.
- The dashboard never sends a display name or arbitrary path as a workflow authority.
- Missing configuration, route, profile authorization, and no-runs conditions show distinct direct actions without starting a workflow.

## 6. Concurrency and accessibility

```sh
make web-e2e
```

**Expected outcome**:

- Two fake workflows in different registered projects can run together.
- A competing workflow for the same canonical project is blocked with the existing workspace-lock guidance.
- Browser tests cover keyboard selection, accessible labels and errors, focus behavior, 390px/narrow-screen layout, and 200% zoom/reflow for project switching and project addition.

## 7. Legacy import safety

In the integration harness, place valid legacy state in the known old default location and leave the new default destination empty. Repeat with an empty source, ambiguous source, non-empty destination, and declined confirmation.

**Expected outcome**:

- Data-bearing legacy state is never copied until source/destination and confirmation are shown/accepted.
- A confirmed import creates a private backup, retains the source, validates the restored database, records/display the destination workspace ID, and preserves historical all-project run visibility.
- Empty, ambiguous, declined, corrupt, schema-incompatible, merge-required, or destination-conflicting cases leave destination and source state unchanged.

## Final evidence

Record commands and results as `PASS`, `FAIL`, `BLOCKED`, or `NOT_RUN`. Fake-provider results must be labeled simulated; live-provider validation is `NOT_RUN` unless an authorized profile is explicitly used.

### 2026-10-24 verification run (fresh post-convergence)

- `make verify` — **PASS**, exit code 0. Includes Go format check, `go vet ./...`, `go test ./...` (including prior-schema import, workflow-boundary, setup, serve collision, readiness-timeout, managed child startup, and 100-call reuse tests), frontend typecheck and 48 unit/component tests, 7 Playwright/Axe tests (including two-project filtering, selected-run event isolation, third-project addition, and 26 page/state Axe pairs), production web build, Go build, secret scan, and smoke workflow.
- Smoke workflow — **PASS (SIMULATED)**. Registered `examples/fake-provider-demo`, started the configured fake-provider run, and observed fixture completion. Fresh health endpoint responded in 32.196 ms; dashboard assets were served.
- Managed dashboard reuse protocol — **PASS** (`TestDashboardReusesVerifiedWorkspaceForOneHundredInvocations`, 2026-10-24). 100/100 invocations reused the same verified endpoint; 100/100 completed under 5 seconds (min 0.418 ms, mean 0.489 ms, max 0.901 ms); failures 0; duplicate managed launches 0. Timings use Go's monotonic component.
- Managed dashboard first launch — **PASS** (`TestEnsureDashboardStartsAndVerifiesManagedChild`). The test builds the actual CLI, launches it on a disposable loopback port, and verifies the child reports the expected workspace UUID before readiness is returned. Confirmed again in the fresh full Go suite.
- Compatibility and refusal coverage — **PASS**. Resolver tests cover Linux XDG, macOS Application Support, fallback platform paths, legacy path, and explicit DB precedence. CLI/API/store coverage includes `--project`, custom/isolated ports, foreign workspace identity, source retention, empty/corrupt/future-schema sources, declined confirmation, and refusal to merge into an existing destination.
- Security boundary audit — **PASS**. Existing tests verify secret redaction in CLI/API outputs and persisted evidence, safe workspace/project projections, bounded JSON request bodies, minimized dashboard child environment, and private import backup permissions/references.
- Live-provider validation — **NOT_RUN**.
- Store legacy-import regression — **PASS**. A prior-schema database with historical run data was snapshotted, restored, and validated; the source remained present, the backup had mode `0600`, imported history remained readable, the full lifecycle event sequence was recorded, and a second import refused the occupied destination. Unconfirmed, corrupt, and future-schema imports leave the destination absent.
- CLI declined-import prompt — **PASS**. The prompt shows source and destination; declining left the source bytes unchanged and destination absent.
- Manual two-project concurrency/collision/stale-selection and ambiguous/merge-required import scenarios — **NOT_RUN**. `setup` detects only the single former config-directory database and refuses import when the destination already exists. Automated tests cover decline, empty/corrupt/future-schema input, occupied destination, source retention, backup permissions, and lifecycle events. Multi-source discovery is intentionally unsupported; hands-on confirmation of migration prompts remains outstanding.
- Ten-person first-use protocol — **NOT_RUN; requires 10 representative developers**. Use fresh disposable project A/B directories and an isolated home for each participant. Start a monotonic timer when the participant receives the task; stop when both projects appear in the dashboard's project switcher. Do not coach unless needed for safety; record first-attempt success and any database/host/port input. SC-001 passes only if at least 9 of 10 complete within 10 minutes.

| Participant | Completed (Y/N) | First attempt (Y/N) | Time (mm:ss) | Entered DB/host/port? | Notes |
|---|---|---|---|---|---|
| P01 |  |  |  |  |  |
| P02 |  |  |  |  |  |
| P03 |  |  |  |  |  |
| P04 |  |  |  |  |  |
| P05 |  |  |  |  |  |
| P06 |  |  |  |  |  |
| P07 |  |  |  |  |  |
| P08 |  |  |  |  |  |
| P09 |  |  |  |  |  |
| P10 |  |  |  |  |  |

### Automated multi-project acceptance evidence

- Two-project setup and readiness — **PASS** through confirmed/idempotent `setup` tests and project-registration API tests.
- Scope isolation and selected-run events — **PASS (SIMULATED)** in Playwright with separate project A/B runs; the selected A view showed A history/event evidence and no B run or event.
- Cross-project concurrency and same-project writer guard — **PASS (SIMULATED)** in Go workflow tests.
- Dashboard ownership/collision — **PASS** in matching/foreign workspace probe tests, second-default-`serve` guidance, readiness-timeout cleanup, 100-call reuse, real managed child startup and identity verification, actual bound-URL serving, and occupied custom-port diagnostics.
- Stale selection and keyboard add-project — **PASS** in Vitest and Playwright; scope switching via keyboard and platform-native browser selection still needs hands-on confirmation.
- Legacy import — **PASS** for prior-schema backup, restore, identity, source retention, lifecycle events, declined confirmation, corrupt/future-schema rejection, and existing-destination refusal. Multi-source ambiguity and merge-required manual cases — **NOT_RUN**.

### Requirement review

| Requirement | Evidence | Status |
|---|---|---|
| FR-001 | Migration 013/014, workspace identity and import-schema tests | PASS |
| FR-002 | `setup` confirmation, declined-flow, idempotency, readiness, and verified dashboard tests | PASS |
| FR-003 | Canonical project service and absolute-path API/CLI registration | PASS |
| FR-004 | Store idempotency and repeated setup tests | PASS |
| FR-005 | Workflow registration gate and API rejection tests | PASS |
| FR-006 | Explicit `--db`, `--project`, host/port isolation, and differing workspace diagnostics tests | PASS |
| FR-007 | Workspace-identity probe and 100-call existing-dashboard reuse test | PASS |
| FR-008 | Managed child subprocess starts and status endpoint identity is verified; readiness-timeout cleanup is tested | PASS |
| FR-009 | Foreign identity probe, custom-port collision guidance, and actual bound-URL tests | PASS |
| FR-010 | All-project default, project switcher, visible scope and counts | PASS |
| FR-011 | Server-filtered runs, selected-run event isolation, project-aware actions | PASS |
| FR-012 | Persisted valid selection and malformed/stale selection fallback tests | PASS |
| FR-013 | Absolute-path registration and add-project form/error tests | PASS |
| FR-014 | Distinct configuration, route, profile, authorization, and empty-state readiness actions | PASS |
| FR-015 | Workflow and preview resolve registered project IDs and revalidate project configuration | PASS |
| FR-016 | Same-project writer lock and different-project concurrency tests | PASS |
| FR-017 | Explicit database/project and unfiltered-history compatibility retained | PASS |
| FR-018 | Import detects only the single known former config-directory database and refuses destination collisions; multi-source selection is unsupported | PASS |
| FR-019 | Confirmed backup-first import, source retention, schema validation, identity and lifecycle records; automated refusal subset passes, hands-on prompt review remains NOT_RUN | PARTIAL |
| FR-020 | Bounded pages/counts, stable project IDs, path validation and project-scoped API tests | PASS |
| FR-021 | Accessible labels, keyboard selection/add form, narrow layout, and 200% reflow browser tests | PASS |
| SC-001 | Ten-person first-use usability study | NOT_RUN |
| SC-002–003 | Two-project filter and workflow concurrency automated fixtures | PASS (SIMULATED) |
| SC-004 | 100-invocation timing/reuse protocol | PASS |
| SC-005 | Keyboard and narrow-screen browser evidence | PASS |
| SC-006 | Import backup/source-retention regression and refusal subset; full manual matrix remains open | PARTIAL |
| SC-007 | Post-release survey | NOT_RUN |
