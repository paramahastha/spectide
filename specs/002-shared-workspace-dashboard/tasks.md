---

description: "Task list for Shared Workspace Dashboard implementation"
---

# Tasks: Shared Workspace Dashboard

**Input**: Design documents from `specs/002-shared-workspace-dashboard/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Required. The specification requires acceptance coverage, and the constitution requires automated coverage for persistence, routing, security, API/SSE, dashboard behavior, and accessible offline fake-provider use.

**Organization**: Tasks are grouped by user story. Foundational work establishes shared security and persistence boundaries; each story phase then has an independently runnable acceptance path.

## Phase 1: Setup (Shared Test Infrastructure)

**Purpose**: Add deterministic, disposable fixtures so this feature never reads a developer's real default workspace or requires paid provider credentials.

- [X] T001 [P] Add disposable default-workspace, legacy-workspace, and canonical-project fixtures in `internal/app/workspace_test.go` (covered by temporary database/project fixtures, canonical symlink tests, prior-schema import fixture, and isolated resolver tests)
- [X] T002 [P] Add confirmed-input, isolated database/home, and registered/unregistered project CLI fixtures in `internal/cli/root_test.go` (covered by `executeCLIInput`, temporary explicit DBs, setup confirmation/decline, and registration-gate cases)
- [X] T003 [P] Add project/workspace request fixtures and fake-provider run builders in `internal/api/server_test.go` (`testServer`, `registerProjectFixture`, fake-provider engine runs, and project-filter fixtures)
- [X] T004 [P] Add two-project, stale-selection, readiness-state, and foreign-port browser fixtures in `web/e2e/dashboard.spec.ts` (project A/B fixtures, scope-filtered runs/events, stale/malformed IDs, readiness, and simulated foreign/default endpoint)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Establish the shared workspace identity, canonical registration boundary, dynamic readiness model, and dashboard coordination primitives required by every user story.

**⚠️ CRITICAL**: Complete this phase before implementing the story phases. No implementation path may retain automatic registration of an arbitrary directory.

- [X] T005 [P] Add migration/open tests for singleton workspace metadata, nullable run association, exact-match backfill, filtered index, and import-record schema in `internal/store/migration_open_test.go` and `internal/store/evidence_test.go`
- [X] T006 Add append-only workspace migration and embed/version wiring; enforce `workspace_id` as “UUID, generated once, immutable, non-secret”, add nullable `runs.project_id` foreign key, retain `runs.project`, backfill only exact canonical matches, and create the project-history index in `internal/store/migrations/013_shared_workspace.sql` and `internal/store/store.go`
- [X] T007 [P] Add model serialization tests for workspace status, project readiness, project-linked runs, and import records in `internal/model/acceptance_test.go`
- [X] T008 Add workspace/project/readiness/run/import projections and all required readiness states in `internal/model/model.go`; serialization acceptance tests cover the projections
- [X] T009 [P] Add store tests for idempotent canonical registration, stable existing scopes, registered-project lookups, project-filtered pages/counts, and unassigned legacy runs in `internal/store/projects_test.go`
- [X] T010 Implement transactional store operations in `internal/store/store.go` so repeated canonical registration returns the existing record without overwriting scopes, new runs atomically persist both `project_id` and `project`, and project filtering occurs before ordering, pagination, and count calculation
- [X] T011 [P] Add canonical-path, symlink/path-drift, readiness-matrix, no-content-transfer, and launch-resolution tests in `internal/projects/service_test.go` (registration canonicalization and readiness tests plus workflow/API launch-resolution rejection coverage)
- [X] T012 Create the shared registration/readiness/launch service in `internal/projects/service.go`; enforce the data-model constraint “unique absolute resolved directory; no symlink escape”, require existing directories, derive safe display names, and never use display names as execution selectors
- [X] T013 [P] Add resolver tests for Linux XDG data location, macOS Application Support location, legacy location, explicit `--db` precedence, and private runtime paths in `internal/app/workspace_test.go`
- [X] T014 Implement the platform-aware default/legacy workspace resolver and private runtime-lease path helpers in `internal/app/workspace.go`; return one stable default workspace and preserve explicit database override precedence
- [X] T015 Add matching-workspace, stale-lease, launch-race, readiness-timeout, and foreign-endpoint manager tests in `internal/app/workspace_test.go`
- [X] T016 Implement the managed-dashboard lease, explicit-argument child launch, short loopback status probe, and matching-workspace readiness wait in `internal/app/workspace.go`; treat PID data as advisory and require endpoint workspace-ID verification before reuse
- [X] T017 Wire the workspace resolver, projects service, workspace identity, and dashboard manager into application construction without adding a second workflow authority in `internal/app/app.go`
- [X] T018 Add foundation integration coverage proving new-run registration gates occur before existing process/SQLite writer locks and preserve different-project concurrency in `internal/workflow/engine_test.go`

**Checkpoint**: Workspace identity, shared registration/readiness service, migration compatibility, and dashboard coordination are available. User-story work can now proceed.

---

## Phase 3: User Story 1 - Set Up and Use Multiple Projects (Priority: P1) 🎯 MVP

**Goal**: A developer can prepare one repository, add another to the same default workspace, and run from either registered repository without ordinary-use database/host/port options.

**Independent Test**: In a disposable home, run `spectide setup` in project A, add project B, repeat setup/add, and start fake-provider workflows in both. Each run must carry the correct canonical project ID/path; an unregistered directory must create no run and print the exact add-project remediation.

### Tests for User Story 1

- [X] T019 [P] [US1] Add CLI acceptance tests for setup confirmation-before-config-write, verified matching-dashboard availability after setup, idempotent setup/add, project listing readiness, and no copied configuration/profile/secret state in `internal/cli/root_test.go`
- [X] T020 [P] [US1] Add workflow tests that reject unregistered inferred and explicit project paths, preserve project-local configuration, and permit two different registered projects while blocking one duplicate writer in `internal/workflow/engine_test.go`
- [X] T021 [P] [US1] Add API tests that reject unregistered path-based create/preview requests with `PROJECT_NOT_REGISTERED` and leave no project/run record in `internal/api/server_test.go`

### Implementation for User Story 1

- [X] T022 [US1] Add `spectide setup`, root `spectide add [PATH]`, and compatibility `spectide projects register PATH` command flows in `internal/cli/root.go`; show canonical path, require confirmation before invoking project initialization, register one project idempotently, invoke the matching-dashboard manager after successful setup, and print sanitized readiness/remediation
- [X] T023 [US1] Change ordinary `spectide run` and explicit `--project PATH` handling in `internal/cli/root.go` to resolve registered projects through `internal/projects/service.go`; return canonical `spectide add /absolute/path` guidance before any workflow creation
- [X] T024 [US1] Replace the automatic `Store.RegisterProject` side effect in `internal/workflow/engine.go` with registered-project launch resolution, then revalidate config, scope, capability, and existing writer lock immediately before persisting a run
- [X] T025 [US1] Route API path-based run creation and dry-run preview through the registered-project resolver in `internal/api/server.go`, allowing compatibility paths only when they resolve to a registered canonical project
- [X] T026 [US1] Extend CLI/API/store integration assertions in `internal/cli/root_test.go`, `internal/api/server_test.go`, and `internal/store/projects_test.go` to prove each new run stores both its stable project ID and retained canonical path

**Checkpoint**: A first-time developer can safely establish and use two explicit local projects, and unregistered directories cannot become projects by starting work.

---

## Phase 4: User Story 2 - Open One Reliable Dashboard (Priority: P1)

**Goal**: A developer opens one verified dashboard for the default workspace, reuses it safely, and receives actionable collision guidance.

**Independent Test**: From two registered repositories, run `spectide dashboard` twice and confirm both use a healthy endpoint with the default workspace UUID. Verify distinct diagnostics for same-workspace reuse, foreign/unverifiable occupation, timeout, and intentional isolated serving.

### Tests for User Story 2

- [X] T027 [P] [US2] Add loopback workspace-status contract tests that return only workspace UUID, version, dashboard URL, and `local_only`, never raw database/runtime/provider paths or secrets, in `internal/api/server_test.go`
- [X] T028 [P] [US2] Add dashboard-reuse reliability and lifecycle coverage in `internal/cli/root_test.go` and `internal/app/workspace_test.go`: 100-call monotonic reuse, no duplicate launch, actual managed child startup and matching identity, second default `serve`, foreign/mismatched listener, stale lease, readiness timeout, and isolated-port guidance

### Implementation for User Story 2

- [X] T029 [US2] Add `GET /api/v1/workspace/status` and safe workspace-status response construction in `internal/api/server.go`
- [X] T030 [US2] Replace the informational `dashboard` command in `internal/cli/root.go` with the managed-dashboard flow from `internal/app/workspace.go`; only print/open `http://127.0.0.1:4747` after matching ready status
- [X] T031 [US2] Update default `serve` collision handling in `internal/cli/root.go` to distinguish matching workspace ownership from foreign/unverified occupancy and preserve loopback-only/custom-port behavior
- [X] T032 [US2] Add workspace-status and collision-diagnostic contract assertions to `internal/api/server_test.go`; defer public API and troubleshooting documentation until post-convergence verification

**Checkpoint**: The normal dashboard command is a safe, single-port entry point and never falsely claims a foreign listener.

---

## Phase 5: User Story 3 - View and Act Within the Intended Project (Priority: P2)

**Goal**: The dashboard clearly scopes activity to all projects or one registered project, uses stable project IDs for actions, and safely adds projects from an absolute path.

**Independent Test**: With two projects/runs, open the dashboard in All projects, select each project, confirm history/counts/default workflow/preview scope, restore from stale selection, and add a third project without copying configuration or provider access.

### Tests for User Story 3

- [X] T033 [US3] Add API contract tests for read-only project readiness, absolute-path project registration, created-versus-existing response, safe errors, and no repository-content transfer in `internal/api/server_test.go`
- [X] T034 [US3] Add API tests for `project_id` run filtering, 100-item limit, 1,000,000 offset cap, filtered counts/`X-Has-More`, invalid IDs, one-selector-only create/preview requests, and cross-project route/config rejection in `internal/api/server_test.go`
- [X] T035 [P] [US3] Add Vitest accessibility/component tests for All-projects default, valid/stale/malformed saved selection, visible text scope/count, selected defaults, readiness empty states, and add-project form labels/errors in `web/src/accessibility.test.tsx`
- [X] T036 [P] [US3] Add Playwright/Axe tests for two-project filtered recent activity and selected-run events, third-project addition, keyboard-only switcher/form use, narrow 390px layout, 200% zoom/reflow, and no project-B data/action leakage when project A is selected in `web/e2e/dashboard.spec.ts`

### Implementation for User Story 3

- [X] T037 [US3] Extend project and run API models in `internal/model/model.go` and persistence queries in `internal/store/store.go` to expose computed readiness and optional stable `project_id` filtering while retaining unfiltered legacy history
- [X] T038 [US3] Update `GET/POST /api/v1/projects` in `internal/api/server.go` to return server-derived readiness; require dashboard `path` to be absolute; derive a safe display name; return 201 for new/200 for existing; and retain CSRF, redaction, canonicalization, and no-content-read protections
- [X] T039 [US3] Update `GET/POST /api/v1/runs` and route preview resolution in `internal/api/server.go` to accept exactly one `project_id` or compatibility `project` selector, resolve IDs server-side, apply filters before pagination, and reject display-name execution
- [X] T040 [US3] Add API contract coverage for request alternatives, errors, readiness, filtered headers, workspace status, and selected-project activity semantics in `internal/api/server_test.go`; defer public OpenAPI/API documentation until post-convergence verification
- [X] T041 [US3] Add workspace/project/readiness/run project-ID types and safe selection persistence helpers in `web/src/domain.ts`
- [X] T042 [US3] Implement dashboard project loading, persistent top-level switcher, all-project fallback, server-filtered recent-run activity, selected-run events only within active scope, out-of-scope detail clearing, and project-ID workflow/preview defaults in `web/src/App.tsx`
- [X] T043 [US3] Implement the absolute-path add-project action and distinct no-runs/missing-config/missing-route/missing-provider states in `web/src/App.tsx`; submit only the selected path and never implement directory browsing or repository-content transfer
- [X] T044 [US3] Add responsive, keyboard-visible-focus, accessible-label/error, non-color scope, and narrow-screen styles for switcher/readiness/add-project controls in `web/src/style.css`
- [X] T045 [US3] Build the dashboard and refresh embedded assets served by Go in `internal/webassets/static/`

**Checkpoint**: Dashboard scope is explicit and server-authoritative; a stale selection is safe; every workflow/preview action resolves a registered project ID; adding a project never broadens access.

---

## Phase 6: User Story 4 - Preserve Existing and Advanced Workflows (Priority: P2)

**Goal**: Existing project-local and explicit isolated-workspace workflows continue to work, while first adoption offers safe, confirmable legacy-state import.

**Independent Test**: Use an explicit database successfully, then test data-bearing legacy state with confirmation, backup, restore, identity verification, source retention, and import recording. Verify empty, declined, ambiguous, non-empty-destination, invalid, and merge-required cases do not modify state.

### Tests for User Story 4

- [X] T046 [P] [US4] Add resolver and CLI tests for explicit `--db` precedence, `--project` compatibility, explicit custom host/port isolation, and safe differing-workspace diagnostics in `internal/app/workspace_test.go` and `internal/cli/root_test.go`
- [X] T047 [P] [US4] Add store tests for backup-first confirmed import, source retention, private backup reference, integrity/schema failure, no-overwrite destination, empty/invalid/symlink source, and no-silent-merge behavior in `internal/store/import_test.go`; source selection remains restricted to the single known legacy location and setup refuses import when the destination already exists
- [X] T048 [P] [US4] Add API/CLI tests proving existing unfiltered history remains available, legacy unassigned runs remain All-project-only, and browser-safe workspace identity excludes raw storage details in `internal/api/server_test.go`

### Implementation for User Story 4

- [X] T049 [US4] Implement known-legacy detection, empty/different-destination checks, confirmation gating, and safe workspace-ID comparison in `internal/app/workspace.go`
- [X] T050 [US4] Add backup/restore orchestration and `DETECTED` → `AWAITING_CONFIRMATION` → `BACKED_UP` → `RESTORED` → `VALIDATED` → `COMPLETED` import-record persistence in `internal/store/backup.go` and `internal/store/store.go`; refuse overwrite and never delete the source
- [X] T051 [US4] Expose opt-in migration confirmation and truthful backup/import results from setup/dashboard-adjacent CLI flows in `internal/cli/root.go`
- [X] T052 [US4] Preserve compatibility semantics for explicit database, project, host, port, and unfiltered-history flows in `internal/cli/root.go`, `internal/api/server.go`, and `internal/store/store.go`
- [X] T053 [US4] Capture advanced/isolated workspace, migration, workspace-identity, and recovery documentation requirements in the private implementation checklist `specs/002-shared-workspace-dashboard/documentation-notes.md`; defer public documentation until post-convergence verification

**Checkpoint**: Existing scripts and isolated workflows remain usable; importing legacy data is deliberate, backed up, verifiable, and never destructive.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Complete documentation, accessibility, security, release-quality evidence, and end-to-end validation without changing story boundaries.

- [X] T054 [P] Capture first-use command examples, API-reference, screenshot, and help-text update requirements in the private implementation checklist `specs/002-shared-workspace-dashboard/documentation-notes.md`
- [X] T055 [P] Audit new errors, events, API responses, import records, and dashboard strings for secret/path redaction and bounded user-controlled input in `internal/security/redact_test.go`, `internal/api/server_test.go`, and `internal/store/evidence_test.go` (secret redaction, safe workspace status, project/error projections, import metadata isolation, and request body limits are covered)
- [X] T056 [P] Verify all new managed child processes use explicit argument vectors, approved working directories, minimal environments, and no shell expansion in `internal/app/workspace.go` and `internal/app/workspace_test.go`
- [X] T057 Run `make verify` from the repository root; record PASS/FAIL/BLOCKED/NOT_RUN evidence in `specs/002-shared-workspace-dashboard/quickstart.md`
- [X] T058 Execute the two-project, collision, stale-selection, concurrency, and migration scenarios in `specs/002-shared-workspace-dashboard/quickstart.md`; label fake-provider evidence `SIMULATED` and any unavailable live-provider validation `NOT_RUN` (automated acceptance fixtures executed; remaining manual cases are listed)
- [ ] T059 EXTERNAL PARTICIPANTS REQUIRED — Run and record one timed SC-001 first-use usability protocol with at least 10 representative developers: each must set up project A, add project B, and view both projects without database/host/port input; record completion time, first-attempt success, and whether at least 9 of 10 finish within 10 minutes in `specs/002-shared-workspace-dashboard/quickstart.md`. Do not substitute simulated users.
- [X] T060 Run the required convergence pass after T057-T059 findings; route each remediation to its owning implementation task and append convergence evidence to `specs/002-shared-workspace-dashboard/quickstart.md`
- [X] T061 Re-run all affected build, lint, test, browser, security, and quickstart checks after convergence; record fresh PASS/FAIL/BLOCKED/NOT_RUN evidence in `specs/002-shared-workspace-dashboard/quickstart.md`
- [X] T062 Update only verified public command, API, migration, troubleshooting, recovery, and screenshot documentation after T061 passes in `README.md`, `docs/user-guide.md`, `docs/configuration.md`, `docs/troubleshooting.md`, `docs/recovery.md`, `docs/api.md`, `docs/openapi.yaml`, and `docs/screenshots/README.md`
- [X] T063 Perform final requirement-to-evidence review for FR-001 through FR-021 and SC-001 through SC-007 in `specs/002-shared-workspace-dashboard/spec.md` and record remaining limitations in `specs/002-shared-workspace-dashboard/quickstart.md`

## Phase 8: Convergence

**Purpose**: Close the gaps found by comparing the implementation after the first verification pass with the full spec, plan, and constitution. Complete these tasks before final evidence and public documentation.

- [X] T064 **CRITICAL** Restrict the managed dashboard child to an approved working directory and minimal environment, add private runtime lease coordination with stale-lease and launch-race recovery, and verify the process policy in `internal/app/workspace.go` and `internal/app/workspace_test.go` per Constitution I / FR-007–009
- [X] T065 **HIGH** Distinguish same-workspace default-port reuse from foreign or unverifiable listeners in `serve`, report isolated-port guidance for custom serving, and publish the actual bound loopback URL in `internal/cli/root.go` and `internal/api/server.go` per FR-009 / US2
- [X] T066 **HIGH** Complete confirmed setup by initializing project-local configuration only after confirmation, reporting computed readiness, starting and verifying the matching dashboard, and testing idempotent and declined flows in `internal/cli/root.go` and `internal/cli/root_test.go` per FR-002–004 / US1
- [X] T067 **HIGH** Complete dashboard project summaries with server-authoritative total counts, refresh readiness as project state changes, and provide direct actions for missing configuration, route, and authorized provider states in `internal/api/server.go`, `web/src/App.tsx`, and `web/src/accessibility.test.tsx` per FR-010–015
- [X] T068 **HIGH** Implement unambiguous detection of the one supported legacy default and backup-first, confirmed, no-overwrite import with source retention, identity/schema validation, and durable import lifecycle records in `internal/app/app.go`, `internal/cli/root.go`, `internal/store/backup.go`, and `internal/store/store.go` per FR-018–019 / US4; setup refuses to merge when the destination already exists, and store acceptance covers declined, empty, corrupt, future-schema, and occupied-destination refusals
- [X] T069 **HIGH** Add migration and model acceptance tests for stable workspace identity, exact-match run backfill, nullable project association, filtered index, import schema/projections, and readiness serialization in `internal/store/migration_open_test.go`, `internal/store/evidence_test.go`, and `internal/model/acceptance_test.go` per FR-001 / FR-020
- [X] T070 **HIGH** Add API contract tests for unregistered create and preview rejection, absolute-path project registration, created-versus-existing responses, project-ID selection, filtered count and pagination headers, invalid selectors, readiness, and safe workspace status in `internal/api/server_test.go` per FR-005 / FR-011 / FR-013 / FR-020
- [X] T071 **HIGH** Add true two-project component and browser fixtures covering filtered recent runs and selected-run events, stale and malformed saved IDs, scope counts, third-project addition, keyboard operation, and project-A data isolation from project B in `web/src/accessibility.test.tsx` and `web/e2e/dashboard.spec.ts` per FR-010–014 / FR-021
- [X] T072 **MEDIUM** Add compatibility tests for explicit `--db`, `--project`, default versus isolated host and port, and diagnostics when different workspaces expose different histories in `internal/app/workspace_test.go`, `internal/cli/root_test.go`, and `internal/api/server_test.go` per FR-006 / FR-017
- [X] T073 **HIGH** Add deterministic acceptance fixtures for registered and unregistered projects, setup input confirmation, workspace status, legacy location, foreign endpoint, and stale dashboard lease across `internal/app/workspace_test.go`, `internal/cli/root_test.go`, `internal/api/server_test.go`, and `web/e2e/dashboard.spec.ts` per US1–US4
- [X] T074 **HIGH** Run the 100-invocation dashboard reuse protocol with monotonic per-call timings, reuse/failure totals, and duplicate-process count, then record results in `specs/002-shared-workspace-dashboard/quickstart.md` per SC-004
- [X] T075 **HIGH** Execute and record the two-project setup, scope isolation, cross-project concurrency, dashboard collision, stale-selection, and legacy import scenarios in `specs/002-shared-workspace-dashboard/quickstart.md`; label fake-provider outcomes `SIMULATED` and unavailable live-provider results `NOT_RUN` per SC-002–006 (automated acceptance fixtures executed; multi-source ambiguity and merge-required manual cases remain)
- [ ] T076 **HIGH** DUPLICATE OF T059 — The same real-participant protocol and evidence will satisfy both tasks; awaiting at least 10 representative developers. No synthetic cohort is claimed.
- [X] T077 **MEDIUM** Re-run `make verify` and all affected quickstart, import, collision, and dashboard reliability checks after convergence, recording fresh PASS/FAIL/BLOCKED/NOT_RUN evidence in `specs/002-shared-workspace-dashboard/quickstart.md` per Constitution II / T061 (latest `make verify` passes; open acceptance gaps remain individually recorded)

---

## Dependencies & Execution Order

### Phase Dependencies

```text
Phase 1 Setup
    ↓
Phase 2 Foundation
    ├── Phase 3 US1: multi-project setup and registered runs (MVP)
    ├── Phase 4 US2: verified shared dashboard
    ├── Phase 5 US3: dashboard project scope and add-project UX
    └── Phase 6 US4: compatibility and opt-in migration
              ↓
      Phase 7 Polish and full evidence
```

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2. It is the recommended MVP and verifies the registration boundary plus two-project workflow attribution.
- **US2 (P1)**: Depends on Phase 2; it can begin in parallel with US1 after the shared dashboard manager is present. US1 setup invokes the shared manager, while US2 completes direct dashboard-command and collision behavior.
- **US3 (P2)**: Depends on Phase 2 API/project identity. It should follow US1 for real project-linked runs; its UI/API tasks can start after foundation in a separate workstream.
- **US4 (P2)**: Depends on Phase 2 resolver, metadata, and migration primitives. It can run in parallel with US1/US2/US3 after those prerequisites.
- **Polish**: Depends on all desired story phases and refreshed embedded web assets.

### Parallel Opportunities

- T001–T004 are independent setup fixtures.
- T005/T007/T009/T011/T013 are test-first tasks in distinct files and can run in parallel before their paired implementation tasks.
- Within US1, T019–T021 can run in parallel; within US2, T027–T028 can run in parallel; within US3, T035–T036 can run in parallel while T033–T034 remain sequential in the API test file; within US4, T046–T048 can run in parallel.
- US1, US2, and US4 can be assigned to separate developers after Phase 2; US3 backend work can start after T010/T012 and UI work after T037–T040.

## Parallel Execution Examples

### User Story 1

```text
Task: "Add CLI setup/add/run acceptance coverage in internal/cli/root_test.go"
Task: "Add registered-project engine coverage in internal/workflow/engine_test.go"
Task: "Add API registration-boundary coverage in internal/api/server_test.go"
```

### User Story 2

```text
Task: "Add workspace-status contract coverage in internal/api/server_test.go"
Task: "Add dashboard lifecycle/collision coverage in internal/cli/root_test.go"
```

### User Story 3

```text
Task: "Add project/readiness and filtered-run API tests in internal/api/server_test.go"
Task: "Add dashboard accessibility/component tests in web/src/accessibility.test.tsx"
Task: "Add two-project Playwright/Axe coverage in web/e2e/dashboard.spec.ts"
```

### User Story 4

```text
Task: "Add override/isolation CLI tests in internal/app/workspace_test.go and internal/cli/root_test.go"
Task: "Add backup/import safety tests in internal/store/migration_open_test.go and internal/store/backup_test.go"
Task: "Add legacy-history and safe-identity API tests in internal/api/server_test.go"
```

## Implementation Strategy

### MVP First

1. Complete Phases 1 and 2.
2. Complete Phase 3 (US1): safe setup/add, explicit registration, and project-attributed workflows.
3. Validate the US1 independent test using the fake provider.
4. Demonstrate only the verified US1 behavior; do not claim dashboard reuse or migration completion until their phases have evidence.

### Incremental Delivery

1. Deliver US1 as the registration-safe multi-project workflow MVP.
2. Add US2 to make the shared dashboard reliable and discoverable.
3. Add US3 for visible project scope, filtering, readiness, and dashboard registration.
4. Add US4 for legacy migration and advanced-workflow compatibility.
5. Run Phase 7 evidence gates after every increment and before any release preparation.

### Format Validation

Every task above uses `- [ ]`, a sequential `T###` ID, optional `[P]` only when parallel-safe, a `[US#]` label for story tasks, and concrete repository file paths.
