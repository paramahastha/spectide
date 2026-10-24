---

description: "Actionable implementation tasks for dashboard guidance and route recovery"
---

# Tasks: Dashboard Guidance and Route Recovery

**Input**: Design documents from `/specs/003-dashboard-guidance/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [HTTP contract](contracts/http-api.md), [dashboard UI contract](contracts/dashboard-ui.md), [quickstart.md](quickstart.md)

**Tests**: Required. The specification and constitution require evidence that no route preview creates a run, all blocked states remain truthful `NOT_RUN`, remediation is accessible, and the dashboard does not render unsafe/raw diagnostics.

**Organization**: Tasks are grouped by user story so each journey can be built and validated independently. The P1 stories share the workflow screen, so they are sequenced to avoid conflicting edits while preserving independently runnable checks.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel with other tasks in the same phase because it changes a different file and has no incomplete-task dependency.
- **[Story]**: Maps a task to the user story it delivers.

## Phase 1: Setup and Baseline

**Purpose**: Preserve current evidence before changing route-preview safety or the dashboard composition.

- [X] T001 [P] Capture the current 1440px, 768px, and 390px workflow/blocked-preview baseline using `web/e2e/dashboard.spec.ts` and store the inspection output under `.taste-skill/before/`.
- [X] T002 [P] Run and record the existing dry-run no-record and route-security baseline cases in `internal/api/server_test.go` and the existing dashboard accessibility baseline in `web/src/accessibility.test.tsx`.

---

## Phase 2: Foundational Typed Client Contract

**Purpose**: Establish the shared typed, safe browser representation before any dashboard surface consumes blocked-preview data.

- [X] T003 Define `PreviewBlocker`, `PreviewRemediation`, and decoded API-failure types in `web/src/domain.ts`; enforce the contract constraints verbatim: `category` is one of `MISSING_ROUTE`, `MISSING_PROFILE`, `PROFILE_NOT_AUTHORIZED`, `CAPABILITY_NOT_VERIFIED`, `PROJECT_UNAVAILABLE`, `CONFIGURATION_INVALID`, or `UNKNOWN`, and `remediation.target` is one of `routing`, `profiles`, `readiness`, or `project-selection`.

**Checkpoint**: The dashboard has a typed, bounded representation for safe recovery data; user-story work may proceed.

---

## Phase 3: User Story 1 - Understand What to Do Next (Priority: P1) 🎯 MVP

**Goal**: Make the selected project's scope, ordered readiness state, and one next action the dominant workflow experience.

**Independent Test**: With fixtures for all-projects, missing route/configuration, missing profile, unauthorized profile, unverified capability, and ready states, select each project and verify a plain-language readiness path shows exactly one highest-priority action before preview/start.

### Tests for User Story 1

- [X] T004 [P] [US1] Add table-driven readiness-guidance unit tests for every existing project readiness status and all-project scope in `web/src/dashboard-guidance.test.ts`.
- [X] T005 [P] [US1] Add component/accessibility assertions for selected scope, one actionable readiness step, blocked start affordance, keyboard focus, and color-independent labels in `web/src/accessibility.test.tsx`.

### Implementation for User Story 1

- [X] T006 [US1] Implement the pure readiness-to-guidance mapping in `web/src/dashboard-guidance.ts`, including route/configuration before profile/access/capability ordering and a safe unknown/path-changed fallback.
- [X] T007 [US1] Refactor the Workflow control composition in `web/src/App.tsx` to render selected project scope, the SpecTide readiness path, and the mapped single next action; make all-project scope observational and prevent a misleading preview/start action.
- [X] T008 [US1] Apply the `true-black-flat` readiness-first layout, focus-visible states, compact sentence-case labels, and narrow-screen action ordering in `web/src/style.css` without gradients, glows, colored navigation, tinted icon tiles, or fake progress.
- [X] T009 [US1] Add browser/Axe coverage for all-project selection, missing-route guidance, profile-related guidance, ready-project workflow affordance, keyboard operation, 200% zoom, and 390px layout in `web/e2e/dashboard.spec.ts`.

**Checkpoint**: A developer can identify the selected project’s next required action without decoding internal status labels or trying a known-blocked action.

---

## Phase 4: User Story 2 - Recover From a Blocked Route Preview (Priority: P1)

**Goal**: Replace raw blocked-preview JSON with a bounded, stage-specific recovery result while retaining server-side authorization and zero-run guarantees.

**Independent Test**: Preview routes for missing route, missing named profile, unauthorized profile, unverified capability, unknown internal failure, and ready route fixtures. Every blocked result states `NOT_RUN`, identifies a safe cause/action, creates no workflow record, and never uses raw serialized error data as primary content.

### Tests for User Story 2

- [X] T010 [P] [US2] Add table-driven API contract tests for one safe `preview_blockers` entry per dry-run failure category, optional stage/profile/provider fields, redaction, `execution: "NOT_RUN"`, and zero created runs; assert successful dry previews return resolved stage routes with `execution: "NOT_RUN"` and create zero runs, stages, events, provider calls, or repository transfers; include a project that changes from ready to blocked after page load and prove that both preview and start revalidate current readiness, return the current blocked condition, and create no workflow in `internal/api/server_test.go`.
- [X] T011 [P] [US2] Add component tests proving route-preview recovery shows safe missing-profile stage context, direct remediation, `NOT_RUN`, disconnected behavior, and no raw envelope/request ID in `web/src/accessibility.test.tsx`.

### Implementation for User Story 2

- [X] T012 [US2] Implement bounded dry-run failure classification in `internal/api/preview_blocker.go` and wire it through `internal/api/server.go`; emit exactly one redacted blocker whose `stage`, `profile`, and `provider` are included only when safely derived from the selected route, while preserving the existing error envelope and dry-run no-run semantics.
- [X] T013 [US2] Decode non-success preview responses into the typed contract from `web/src/domain.ts` within `web/src/App.tsx`; never parse arbitrary browser error strings for display decisions.
- [X] T014 [US2] Render the blocked-preview recovery card and remediation navigation in `web/src/App.tsx`, preserving the selected project/task draft and distinguishing blocked, idle, ready, and disconnected states.
- [X] T015 [US2] Add Playwright/Axe fixtures and assertions for missing-route, missing-profile, authorization, capability, and unknown blockers; matching remediation; retry navigation; safe text-only diagnostics; and a successful dry preview that lists resolved stage routes and visibly states that no workflow or repository transfer occurred in `web/e2e/dashboard.spec.ts`.

**Checkpoint**: A blocked preview is an actionable recovery experience that truthfully reports `NOT_RUN`; it neither leaks internal diagnostics nor changes the existing server-side safety boundary.

---

## Phase 5: User Story 3 - Work Without Repeated Inputs (Priority: P2)

**Goal**: Let a developer choose a project and write one task once, use that context for preview/start, and keep project management/history secondary.

**Independent Test**: From a ready project, enter one task, preview it, navigate to workflow control, and start it without editable project-path fields. Change project or draft and verify stale previews clear; run the offline demo and verify it stays visibly simulated.

### Tests for User Story 3

- [X] T016 [P] [US3] Add component tests for one in-memory task draft, removal of editable route/start project-path fields, stale-preview invalidation on project/draft change, valid-project retention through remediation, retention of an unsent draft with disabled project actions when the active project becomes unavailable, and simulated-demo labeling in `web/src/accessibility.test.tsx`.

### Implementation for User Story 3

- [X] T017 [US3] Consolidate workflow and preview state in `web/src/App.tsx` around `activeProjectId` plus one in-memory task draft; remove obsolete editable `project` path state; clear the prior draft/preview on an explicit project change; and, if the active project becomes invalid, retain its unsent draft in page memory while clearing preview state and disabling project-specific actions until a valid project is selected.
- [X] T018 [US3] Reorganize project switch/add controls, one task editor, preview/start actions, recent activity, and offline-demo separation in `web/src/App.tsx` and `web/src/style.css` so secondary work does not compete with the readiness path.
- [X] T019 [US3] Add browser/Axe coverage for select → draft → preview → start, remediation round trips, a project becoming blocked after page load before preview or start (current prerequisite shown, task retained, no false run-start state), project changes during pending preview, unavailable-project draft retention with disabled actions, add/switch-project discoverability, offline-demo labeling, and narrow-screen ordering in `web/e2e/dashboard.spec.ts`.

**Checkpoint**: The normal workflow has one project context and one task draft, while history, project registration, and simulated learning paths remain clear but secondary.

---

## Phase 6: Polish, Verification, and Evidence

**Purpose**: Complete cross-story documentation, visual review, and constitution-required evidence without claiming unrun checks.

- [X] T020 Update route-preview and readiness recovery guidance in `docs/troubleshooting.md` and `README.md` only with behavior verified by the completed API/UI tests; retain existing credential and project-scope safety guidance.
- [X] T021 Run the app-screen visual gate against the built dashboard with `web/src/style.css`, `web/e2e/dashboard.spec.ts`, and `.taste-skill/after/`; compare 1440px, 768px, and 390px screenshots against the baseline, fix documented hierarchy/density/focus/overflow findings, and record any unavailable inspector/browser as `BLOCKED` rather than passed.
- [X] T022 Run targeted API, component, and browser suites for `internal/api/server_test.go`, `web/src/dashboard-guidance.test.ts`, `web/src/accessibility.test.tsx`, and `web/e2e/dashboard.spec.ts`; record each result as `PASS`, `FAIL`, `BLOCKED`, or `NOT_RUN` in `docs/implementation-progress.md`.
- [X] T023 Run `make verify` from `Makefile` after rebuilding embedded web assets; record the exact command outcomes, simulated/live classification, and any remaining limitations in `docs/implementation-progress.md`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 — Setup**: Starts immediately; T001 and T002 may run in parallel.
- **Phase 2 — Foundational**: Depends on Phase 1 evidence; T003 defines the typed client contract used by blocked-preview recovery.
- **Phase 3 — US1**: Depends on T003. T004 and T005 may run in parallel; T006 precedes T007; T008 and T009 follow the workflow composition.
- **Phase 4 — US2**: Depends on T003 and the US1 readiness/action surface. T010 and T011 may run in parallel; T012 precedes T013/T014; T015 follows the completed response/UI flow.
- **Phase 5 — US3**: Depends on the US1 workflow surface and US2 preservation requirements. T016 precedes T017; T018 follows state consolidation; T019 follows the complete journey.
- **Phase 6 — Polish**: Depends on all desired user stories and their focused tests.

### User Story Dependencies

```text
Foundation (T003)
  └── US1: readiness-first workflow (T004–T009)  [MVP]
       └── US2: typed blocked-preview recovery (T010–T015)
            └── US3: one project + one task draft (T016–T019)
                 └── Polish and evidence (T020–T023)
```

US2 deliberately follows US1 because remediation actions land on the readiness-first workflow surface. US3 follows both P1 stories because it preserves their selected-project and task context while removing duplicate inputs.

### Parallel Opportunities

- **Setup**: T001 and T002.
- **US1**: T004 and T005 can be prepared in parallel before the guidance mapping; after T007, stylesheet work T008 can be assigned separately from browser coverage T009 when the markup contract is agreed.
- **US2**: T010 (Go contract tests) and T011 (React recovery tests) can be prepared in parallel; after the response schema is agreed, T012 and client decoder prep may proceed in separate files.
- **US3**: T016 component tests can be prepared while API work is finishing because it relies only on the documented client behavior.
- **Polish**: T020 documentation and T021 visual review can run in parallel after feature tests pass; T022 must precede T023.

## Parallel Example: User Story 2

```text
Task: "Add API preview-blocker contract tests in internal/api/server_test.go"
Task: "Add blocked-preview accessibility tests in web/src/accessibility.test.tsx"
```

Both tasks define the server and browser acceptance boundary before the implementation tasks change `internal/api/preview_blocker.go`, `internal/api/server.go`, and `web/src/App.tsx`.

## Implementation Strategy

### MVP First — User Story 1

1. Complete T001–T003 to preserve baseline evidence and establish typed client shapes.
2. Complete T004–T009 to deliver selected-project scope, readiness ordering, a single next action, and responsive accessible workflow guidance.
3. Run US1’s independent test scenario before accepting further scope.

### Incremental Delivery

1. **US1** makes readiness and the next step understandable before a request is attempted.
2. **US2** turns actual blocked dry runs into safe, precise recovery without changing no-run security semantics.
3. **US3** removes repeated path/task input and makes the complete preview-to-start flow coherent.
4. **Polish** confirms documentation, visual direction, and end-to-end evidence.

## Format Validation

All 27 tasks use the required `- [X] T### [P?] [US#?] Description with file path` checklist format. User-story tasks have a story label; setup, foundational, polish, and convergence tasks intentionally do not.

---

## Phase 7: Convergence

- [X] T024 Replace periodic dashboard polling in `web/src/App.tsx` with action- and SSE-driven refreshes in line with the plan's interaction constraint, retaining current-readiness rechecks, recovery behavior, and project/task context.

---

## Phase 8: Convergence

- [X] T025 Render an action-time route-preview transport failure as the disconnected/unavailable local-connection state, retaining the selected project and task draft and adding component/browser coverage in `web/src/App.tsx`, `web/src/accessibility.test.tsx`, and `web/e2e/dashboard.spec.ts` per FR-008
- [X] T026 Derive and return safe stage, expected fallback-profile, and provider context for `FALLBACK_PROFILE_NOT_FOUND` preview blockers, with API contract coverage proving no run is created in `internal/api/preview_blocker.go` and `internal/api/server_test.go` per FR-009, FR-011
- [X] T027 Classify unauthenticated or authentication-unverified provider evidence as `CAPABILITY_NOT_VERIFIED` with readiness remediation rather than project-boundary authorization, and cover the distinction in `internal/api/preview_blocker.go`, `internal/api/server_test.go`, and dashboard recovery tests per FR-012
