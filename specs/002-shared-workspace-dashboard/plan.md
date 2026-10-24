# Implementation Plan: Shared Workspace Dashboard

**Branch**: `002-shared-workspace-dashboard` | **Date**: 2026-10-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-shared-workspace-dashboard/spec.md`

## Summary

Make SpecTide operate as one local, loopback-only workspace for explicitly registered repositories. Everyday setup, adding projects, running workflows, and opening the dashboard will use a platform-aware default workspace without requiring database, host, or port options. The plan centralizes registration/readiness validation, identifies each workspace safely, associates new runs with stable project IDs, adds project-aware dashboard/API behavior, coordinates one verified default dashboard, and offers backup-first legacy import while preserving explicit isolated-workspace workflows.

## Technical Context

**Language/Version**: Go 1.27.2; TypeScript 5.9.3 for the dashboard.

**Primary Dependencies**: Cobra CLI; `modernc.org/sqlite`; Google UUID; Go standard library loopback HTTP/process/filesystem primitives; React 19.2.7; Vite; Vitest; Playwright; Axe.

**Storage**: Local SQLite with WAL, sequential embedded migrations, private file permissions, existing backup/restore primitives, project-local `.spectide/config.json`, and private local runtime lease metadata for the managed dashboard.

**Testing**: `go test ./...`; `go vet ./...`; Go CLI/API/store/workflow tests; Vitest and Testing Library; Playwright/Axe browser tests; existing fake provider for deterministic offline workflow coverage; `make verify` for the aggregate local quality flow.

**Target Platform**: Single-user local desktop development on macOS and Linux; loopback dashboard/API only. Existing non-Unix process-lock behavior remains fail-closed.

**Project Type**: Local Go CLI and orchestration service with an embedded React/TypeScript browser dashboard.

**Performance Goals**: A healthy matching dashboard is reused and its URL reported within 5 seconds in 99% of attempts; normal dashboard startup waits for a confirmed ready response; project/run listings remain bounded to 100 records per page; the two-project first-use journey completes within 10 minutes for 90% of participants.

**Constraints**: Default-deny project/profile authorization; canonical, symlink-safe paths; no secret, raw database, provider-config, runtime-lease, or repository-content exposure to browser clients; no arbitrary browser filesystem/shell access; explicit subprocess argument vectors; preserve project-local configuration, existing global history, loopback binding, `--db`, `--project`, and isolated `serve --host/--port` workflows.

**Scale/Scope**: One user and one local workspace managing many deliberately registered repositories; default acceptance validates two projects, a third dashboard-added project, project-scoped history, and migration from one unambiguous legacy default. Remote/team/machine-sync use is out of scope.

## Constitution Check

### Pre-design gate

| Principle / constraint | Planned compliance | Gate |
|---|---|---|
| Local-first, least privilege | Keep state and dashboard local/loopback; require registered canonical project IDs; no automatic discovery or scope expansion; browser receives only approved local operations. | PASS |
| Evidence before completion | Require automated CLI, API, migration, store, workflow, component, accessibility, and browser evidence; label fake-provider validation as simulated and live validation as `NOT_RUN` when unavailable. | PASS |
| One product scope | Extend the existing `002-shared-workspace-dashboard` specification and plan; do not create a replacement product scope. | PASS |
| Durable, observable, recoverable workflows | Keep the Go engine as workflow owner; preserve per-canonical-project writer fencing, durable run/event history, backup/restore validation, and explicit interruption/recovery semantics. | PASS |
| Provider-neutral, accessible, maintainable delivery | Reuse provider/profile validation and fake provider; add keyboard, accessible-label, narrow-screen, and browser tests; add no dependency without a demonstrated P0 need. | PASS |
| Security constraints | Canonicalize/revalidate paths, retain CSRF/Host/Origin/body limits/redaction, use explicit child-process arguments, prohibit browser DB-path/content access, and never silently merge state stores. | PASS |

**Pre-design result**: PASS. No constitutional exception or complexity justification is required.

## Project Structure

### Documentation (this feature)

```text
specs/002-shared-workspace-dashboard/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── documentation-notes.md    # private implementation checklist; not public documentation
├── contracts/
│   ├── cli.md
│   └── http-api.md
└── tasks.md                         # created later by /speckit.tasks
```

### Source Code (repository root)

```text
cmd/spectide/
└── main.go                          # CLI entry point

internal/
├── app/
│   ├── app.go                       # application composition; default workspace resolver integration
│   └── workspace.go                 # new: default/legacy workspace resolution and managed-dashboard lifecycle
├── api/
│   ├── server.go                    # loopback workspace, project, run, readiness contracts
│   └── server_test.go
├── cli/
│   ├── root.go                      # setup/add/dashboard/serve/run command behavior
│   └── root_test.go
├── config/                          # safe project-local configuration access
├── model/
│   └── model.go                     # workspace/readiness/project-linked run projections
├── projects/
│   ├── service.go                   # new: canonical registration, lookup, readiness, launch resolution
│   └── service_test.go
├── store/
│   ├── store.go                     # project/run query and persistence changes
│   ├── backup.go                    # backup-first import support
│   ├── migrations/
│   │   └── 013_shared_workspace.sql # append-only workspace/run/import migration
│   └── *_test.go
├── workflow/
│   ├── engine.go                    # registered-project-only launch, preserved writer locks
│   └── engine_test.go
├── workspace/                       # existing canonical-path and process writer locks
└── webassets/                       # embedded built dashboard assets

web/
├── src/
│   ├── App.tsx                      # project switcher, scoped actions, readiness/empty states, add project
│   ├── domain.ts                    # project, readiness, workspace, and project-linked run types
│   ├── style.css                    # responsive accessible switcher/add-project styling
│   └── accessibility.test.tsx
├── e2e/
│   └── dashboard.spec.ts
└── package.json

docs/
├── openapi.yaml
├── api.md
├── user-guide.md
├── configuration.md
├── troubleshooting.md
└── screenshots/
```

**Structure Decision**: Keep the established Go monolith plus embedded dashboard. Add a focused `internal/projects` boundary to eliminate duplicated registration/launch checks and a small app-level workspace/dashboard manager; do not add a remote service, microservice, or browser-side scheduler.

## Implementation Approach

### Phase A — Default workspace, identity, and safe project boundary

1. Introduce the platform-aware default-workspace resolver and use it wherever an omitted `--db` currently calls `app.DefaultDBPath()`; preserve explicit override precedence.
2. Add append-only migration `013_shared_workspace.sql` for workspace metadata, nullable run `project_id`, filtered-history index, and import records. Initialize a non-secret workspace UUID exactly once.
3. Add `internal/projects` service for path canonicalization, idempotent registration without scope overwrite, stored-project resolution, readiness evaluation, and a launch-time revalidation operation.
4. Update store/model projections to return a registered project by ID/canonical path, project-linked run pages/counts, and safe import audit data.
5. Backfill run IDs only for exact unambiguous canonical-path matches; retain unassigned legacy runs in all-project history.
6. Update engine startup to require a registered project and delete the automatic `RegisterProject` call. Keep existing canonical workspace process/SQLite locks after registration/readiness validation.

### Phase B — CLI onboarding and verified dashboard management

1. Add `setup`, root-level `add`, and streamlined project-list aliases using the shared project service.
2. Make setup show the canonical path and require explicit confirmation before project-local config initialization; report readiness and provider/profile remediation without storing secrets.
3. Change ordinary `run` to infer the current directory only when no explicit selection is supplied and return the canonical `spectide add PATH` remediation for an unregistered directory. Keep explicit project path/DB flows, but require registered project resolution.
4. Implement managed default-dashboard startup/reuse: a private runtime lease prevents launch races, a short loopback probe checks workspace status, a child server starts with explicit arguments, and the command waits for matching ID/readiness before reporting/opening the URL.
5. Improve `serve` collision handling: same-workspace endpoint directs users to `dashboard`; foreign/unverifiable occupancy receives isolated-port guidance with no ownership claim.
6. Add opt-in legacy detection/import around the new resolver. Reuse backup/restore validation, require confirmation for data-bearing source, reject merge/overwrite/ambiguous cases, preserve source, and record the verified outcome.

### Phase C — Project-aware API and contracts

1. Add safe `GET /api/v1/workspace/status` for workspace UUID, version, loopback URL, and local-only state.
2. Extend projects listing with computed readiness, and update registration to require an absolute dashboard path while retaining a compatibility-safe request shape for existing callers.
3. Extend bounded run listing with optional `project_id` filtering; validate the project before query, filter before count/pagination, and preserve unfiltered compatibility.
4. Update workflow/preview request contracts to use `project_id` for dashboard calls. Resolve a project server-side and reapply config, route, scope, capability, path-drift, and writer-lock validation immediately before acting.
5. Preserve endpoint security middleware and add contract tests for all changed/added responses, error codes, and pagination headers; defer OpenAPI/API documentation updates until post-convergence verification.

### Phase D — Dashboard project UX

1. Extend dashboard domain types and initial load to fetch workspace status and project/readiness data alongside current handshake state.
2. Add an always-visible, keyboard-accessible project switcher: **All projects** is the upgrade default; registered projects show safe name/path context; active scope and count are textual and prominent.
3. Store only a selected project UUID as a browser preference; validate it against fresh project data and reset malformed/stale/unauthorized values to all projects.
4. Treat the server-filtered recent-run list as project recent activity, retain events only for a selected run in the active scope, and use selected project IDs for route preview and workflow start. Clear selected run/details/events when a scope change makes them out of scope.
5. Add an accessible absolute-path add-project flow that submits only user intent/path, then refreshes readiness; never browse or transmit repository contents.
6. Render distinct no-runs, missing-configuration, missing-route, and missing/unauthorized-provider states with one direct action. Preserve existing connection, CSRF refresh, stale-response, and SSE safety behavior.
7. Regenerate embedded web assets. Defer public documentation and screenshots until post-convergence verification confirms the implemented behavior.

### Phase E — Verification and evidence

1. Add Go tests for resolver/override behavior, ID persistence, migrations/backfill, safe registration/idempotency/path drift, readiness matrix, run association/filter pagination, API contract errors, CLI confirmation/remediation, dashboard lifecycle/collisions, lock concurrency, and import recovery/refusal cases.
2. Add component and browser tests for scope filtering, stale saved selection, selected-action defaults, add-project errors/success, data isolation, keyboard/focus/labels, narrow layout, zoom, and Axe results.
3. Run the quickstart validation sequence and record each check as `PASS`, `FAIL`, `BLOCKED`, or `NOT_RUN`; include a 100-attempt matching-dashboard reuse run with per-attempt timing, reuse/failure totals, and duplicate-launch count; distinguish deterministic fake-provider evidence from authorized live-provider evidence.
4. Run a timed first-use usability validation with a defined representative participant cohort; record task success, completion time, and the SC-001 pass rate.
5. Run convergence for any blocking findings, route remediation to the owning implementation work, then repeat affected validation before final review.
6. Only after validation passes, update public documentation, API contracts, command examples, and screenshots with verified behavior.

## Constitution Check (Post-design)

| Re-check | Design evidence | Gate |
|---|---|---|
| Local-only, least privilege | Contracts expose workspace UUID/URL but not storage paths; project IDs resolve server-side; add-project is absolute-path validation only; no automatic discovery, path browsing, secret persistence, or scope copying. | PASS |
| Workflow authority and recovery | The Go engine remains the only workflow scheduler; project registration gates precede existing locks; workspace runtime lease is only dashboard coordination; legacy import is backup-first and non-destructive. | PASS |
| Security and provider controls | Existing path revalidation, safe config access, profile scope/capability checks, CSRF/Host/Origin controls, redaction, and explicit subprocess arguments are retained and applied to new routes. | PASS |
| Evidence and truthfulness | Data migration, dashboard ownership, readiness, concurrency, and accessibility all have explicit test/quickstart evidence. Fake-provider scenarios remain labeled simulated; absent live credentials remain `NOT_RUN`. | PASS |
| Maintainability and scope | Uses existing Go/React/SQLite architecture with one focused shared-project boundary; no new service or dependency is required. | PASS |

**Post-design result**: PASS. The design satisfies constitutional gates without exceptions.
