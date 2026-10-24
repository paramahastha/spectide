# Research: Shared Workspace Dashboard

## Decision: Introduce one platform-aware default-workspace resolver

**Decision**: Replace direct callers of `app.DefaultDBPath()` with a resolver that returns a stable default workspace directory, state-store path, runtime-lease path, and a safe display identity. On Linux, use `XDG_DATA_HOME` (or `~/.local/share`) and on macOS use the user Application Support location. Keep `--db` as an explicit override that bypasses this resolver.

**Rationale**: The current default at `os.UserConfigDir()/spectide/spectide.db` is already user-level but is not modeled as a complete workspace and does not provide a runtime coordination location. Centralizing resolution prevents CLI commands, server startup, and migration logic from selecting different stores. It also gives migration one precise old-location candidate.

**Alternatives considered**:
- Continue resolving the database in each command: rejected because it preserves divergent default-state behavior.
- Use repository-local state as the normal default: rejected because it contradicts the shared-workspace experience.
- Remove explicit `--db`: rejected because demos, tests, scripts, and isolation depend on it.

## Decision: Make explicit registration an invariant before route or workflow evaluation

**Decision**: Add one shared project service that canonicalizes a path, verifies it is a directory within its approved boundary, registers idempotently, resolves stored projects by ID/path, and computes readiness. The engine and API must resolve a stored project before loading configuration, route-previewing, or creating a run. Remove the engine's current automatic registration side effect.

**Rationale**: Current engine startup auto-registers any existing directory, allowing an arbitrary path to become a project merely by starting a run. A single service gives CLI and dashboard registration identical symlink, canonicalization, scope, and redaction protections.

**Alternatives considered**:
- Retain auto-registration and surface it in the UI: rejected because it expands provider scope without an explicit user decision.
- Duplicate validation in CLI, API, and engine: rejected because differing path rules create boundary bypasses.
- Use project display name to resolve execution: rejected because display names are mutable and not an authorization boundary.

## Decision: Persist a non-secret workspace identity and stable run-to-project association

**Decision**: Add an append-only migration that creates a singleton workspace metadata record with a generated UUID, adds nullable `project_id` to runs, indexes filtered run history, and records confirmed legacy imports. New runs store both `project_id` and the existing canonical project path; historical runs retain their path and receive an ID only when backfill is unambiguous.

**Rationale**: The identity lets a dashboard command prove it reached the intended workspace without disclosing database paths. A stable relationship makes project filtering correct even if display names change and retains old run/report compatibility.

**Alternatives considered**:
- Identify a workspace by database path over browser APIs: rejected because paths are sensitive host details.
- Replace `runs.project` outright: rejected because it breaks existing reports and historical data.
- Infer run scope from route configuration at read time: rejected because configuration can change after a run starts.

## Decision: Compute project readiness; do not persist or infer it from run history

**Decision**: Return a read-only readiness summary with each registered project. It is calculated from canonical-path validity, project configuration, route presence, profile existence, scope authorization, and capability evidence. It contains a stable reason/action code plus sanitized user guidance.

**Rationale**: Readiness depends on externally changeable configuration and provider evidence. Computing it prevents stale readiness records and allows the dashboard to distinguish no configuration, no route, no authorized profile, and not-yet-verified capability.

**Alternatives considered**:
- Infer readiness from the most recent run: rejected because an old run is not proof of present readiness.
- Probe providers when listing projects: rejected because list views must be safe, fast, and non-executing.
- Persist readiness and update it on every possible external change: rejected as incomplete and error-prone.

## Decision: Coordinate the normal dashboard with a runtime lease plus endpoint identity verification

**Decision**: The dashboard manager will use a separate private runtime lease/PID record for launch-race coordination, probe `http://127.0.0.1:4747`, and reuse an endpoint only when a workspace-status response returns the expected workspace UUID. It will start a managed loopback child process with explicit arguments, wait for a matching ready response, and report foreign or unverifiable listeners without claiming ownership.

**Rationale**: The existing project writer lock cannot represent dashboard ownership. A process record alone can be stale; endpoint identity is the final ownership proof. This preserves loopback-only operation and turns raw bind failures into safe guidance.

**Alternatives considered**:
- Treat any successful health response as the default dashboard: rejected because another workspace can use the same address.
- Trust a PID file alone: rejected because PIDs can be stale or recycled.
- Allocate a dashboard port per project: rejected because it fragments the shared workspace and recreates collisions.

## Decision: Evolve existing loopback contracts compatibly

**Decision**: Preserve unfiltered run history and existing project/config endpoints. Add optional `project_id` filtering to the bounded run list, workspace status, server-derived project readiness, absolute-path project registration, and project-ID workflow/preview requests. Dashboard actions send stable IDs, while advanced explicit-path calls remain accepted only after resolution to a registered project.

**Rationale**: This supports the project switcher without client-side filtering, avoids route/config cross-over, and protects scripts that use existing list and explicit database workflows.

**Alternatives considered**:
- Filter loaded all-project history in the browser: rejected because it exposes data outside the selected scope and scales poorly.
- Add a separate API version: rejected because optional fields and filters are backward compatible.
- Allow unregistered path requests only from the CLI: rejected because the registration boundary must be consistent.

## Decision: Use explicit, backup-first legacy import

**Decision**: Detect only the known legacy default location when it is unambiguous. If it contains data and the new default destination is empty, show source and destination, require confirmation, create a private backup, restore into a new destination, verify integrity/schema and workspace identity, write an import record, and retain the source. Refuse automatic merge, ambiguous discovery, and overwrite.

**Rationale**: The store already provides no-overwrite, integrity-checked backup and restore primitives. The flow protects audit history and prevents accidental state combination.

**Alternatives considered**:
- Silently copy or merge all discovered stores: rejected because IDs, retention rules, profiles, and audit histories can conflict.
- Move the legacy database: rejected because it is destructive and unrecoverable.
- Require every user to migrate: rejected because explicit database workflows remain supported.

## Decision: Keep implementation dependency-neutral and validate across Go and browser layers

**Decision**: Use existing Go standard library, Cobra, UUID, SQLite, React, TypeScript, Vitest, and Playwright tooling. Add no dependency unless later evidence identifies a concrete P0 need. Regenerate embedded web assets after dashboard work.

**Rationale**: Existing tooling already supports secure local process control, storage migration, component tests, browser accessibility tests, and deterministic fake-provider scenarios.

**Alternatives considered**:
- Add a daemon/process supervisor dependency: rejected because a small local manager and explicit subprocess arguments satisfy the requirement.
- Add a client-side state framework: rejected because the dashboard's current state and stale-response guards are sufficient.
