# Proposal: one local workspace for many projects

**Status:** proposed
**Problem observed:** a user who starts `spectide --db .spectide/state.db serve` in one repository reasonably expects the dashboard to represent all of their work. Instead, the database location, whether commands share it, and port collisions are exposed as implementation details. This makes the existing multi-project capability difficult to discover and easy to use incorrectly.

## Outcome

SpecTide should feel like one local application that can manage many explicitly approved repositories:

- Users start one dashboard server on one local port and use it for all their projects.
- Normal commands do not require `--db`, `--host`, or `--port`.
- A project is added deliberately, has its own local route configuration, and remains subject to its own provider-scope checks.
- The dashboard makes the active project obvious and supports viewing either one project or all projects.
- Two simultaneous `serve` commands give a useful answer, rather than an unexplained `address already in use` error.

The current SQLite store, registered-project model, per-run canonical `project` path, and `/api/v1/projects` endpoint already establish the core data boundary. This proposal is primarily a usability and product-completeness change, not a relaxation of those boundaries.

## Single-port dashboard decision

The normal installation has exactly one dashboard endpoint: `http://127.0.0.1:4747`. That server opens the default shared workspace database and monitors every project registered in it. Projects are separated in the data model, route configuration, provider authorization, and workspace locks—not by dashboard ports.

```text
Project A ─┐
Project B ─┼─> shared local workspace database ─> one SpecTide server :4747 ─> dashboard
Project C ─┘                                          └─ All projects / selected project views
```

Therefore, a user does **not** run one `serve` command per project. They run or open `spectide dashboard` once, then add projects and start workflows from each repository. The dashboard subscribes to the same persisted run/event history and can show all projects together or one selected project.

Additional ports are an advanced isolation mechanism only—for example, automated tests, a temporary demo database, or intentional parallel development against a different database. They are not part of normal multi-project tracking.

## Design principles

1. **One default workspace.** The default state database is a stable user-level location, not a path relative to the repository from which a command happens to be run.
2. **Explicit projects, isolated execution.** A project must be registered and configured separately. A run lock remains per canonical workspace, so unrelated projects may run at the same time but the same project cannot have competing writers.
3. **Safe by default.** Project registration must canonicalize the path, validate it is a directory, and never expand provider scopes automatically beyond the project selected by the user.
4. **Progressive disclosure.** Advanced users may still select a database or port, but these choices are absent from the everyday workflow and help text examples.
5. **Truthful status.** A dashboard opens only when its server is actually reachable and it must clearly show which local workspace/database it is displaying.

## Proposed everyday experience

### First project

From a repository, a new user runs:

```sh
spectide setup
```

`setup` should:

1. Create or open the default local workspace at an OS-appropriate user data path (for example `~/.local/share/spectide/state.db` on Linux and the macOS Application Support equivalent).
2. Register the current repository after displaying its canonical path.
3. Create the project-local `.spectide/config.json` only after the user confirms.
4. Guide the user to add or select a verified provider profile and its exact project scope.
5. Start or open the existing local dashboard and print one short next-step command.

The command must be idempotent: rerunning it reports that the workspace and project are already ready rather than duplicating records or configuration.

### Subsequent projects

From another repository:

```sh
spectide add .
```

This registers the canonical current directory in the same default workspace, initializes or validates its own project configuration, and explains any missing route or provider authorization. It must not copy the first project’s route, profile permissions, or secrets into the new project.

### Daily use

```sh
spectide dashboard
spectide run "Add an audit log to exports"
```

When invoked inside a registered project, `run` infers the project from the current directory. A user outside a registered project receives a clear remediation, for example:

```
This directory is not registered with your local SpecTide workspace.
Run: spectide add /absolute/path/to/project
```

`--project PATH` remains available for scripts and cross-directory use. `--db PATH`, `serve --port`, and `serve --host` remain advanced options with documentation, but are not needed for this flow.

## Command surface

| User goal | New/default command | Existing/advanced equivalent |
| --- | --- | --- |
| Guided first-use setup | `spectide setup` | manual `projects register`, `init`, profile setup |
| Add the current repository | `spectide add .` | `spectide projects register PATH` plus `init` |
| See projects | `spectide projects` | `spectide projects list` |
| Set the current project | `spectide use PROJECT` | pass `--project PATH` |
| Open the shared dashboard | `spectide dashboard` | `spectide serve` and manually open the URL |
| Start a run here | `spectide run "…"` | `spectide run --project PATH "…"` |
| Use isolated state for testing/automation | `spectide --db PATH …` | unchanged |

`dashboard` should first probe the default server’s health endpoint at port `4747`. If it is healthy and belongs to the default workspace, it opens or prints the existing URL. If no server is active, it starts one in a managed background process, writes a local PID/lock record, waits for `/healthz`, then opens or prints the URL. A second `serve` should say:

```
The SpecTide dashboard is already running at http://127.0.0.1:4747.
Use `spectide dashboard` to open it, or use `serve --port PORT` for an isolated instance.
```

It must verify that the running server uses the requested workspace before making that claim.

## Dashboard changes

Add a persistent project switcher at the top of the dashboard:

- **All projects** is the initial view after upgrade, preserving today’s all-run history behavior.
- Each registered project appears by display name with its canonical-path tooltip or secondary label.
- Selecting a project filters run history, recent activity, and default project path for “Start a workflow” and “Route preview.”
- The page clearly states the active scope: `All projects · 12 runs` or `spectide · /Users/…/Projects/spectide`.
- An empty project state explains whether the project has no runs, lacks configuration, or lacks an authorized provider, with one direct action.
- A visible “Add project” action accepts a local absolute path and uses the existing protected project-registration API. It must never browse or transmit repository content.

The dashboard should retain the chosen project in local browser storage, but the API remains authoritative and a malformed/stale saved selection falls back to **All projects**.

## API and persistence changes

Keep the existing `projects` table and canonical-path uniqueness. Add only what the UI needs:

- Extend `GET /api/v1/runs` with optional `project_id` (preferred) and/or canonical `project` filtering, with the same bounded pagination rules as the current list endpoint.
- Return a project’s configuration/readiness summary in `GET /api/v1/projects` or a dedicated read-only endpoint, so the dashboard does not infer readiness from runs.
- Add a local workspace status endpoint containing server version, workspace identity (a non-secret generated ID), dashboard URL, and default-server state. Do not expose the database path to browser clients unless it is explicitly safe to display.
- Preserve all existing endpoints and unfiltered history for compatibility.
- Store the default workspace location in one platform-aware resolver. Commands, managed dashboard startup, and migration code must use the same resolver.

No run may be created solely from a display name. The server must resolve the selected project to its stored canonical path and reapply configuration, scope, capability, and workspace-lock validation before creating the run.

## Migration and compatibility

1. Existing users keep working: an explicit `--db` always wins over the default resolver.
2. On the first version with this feature, detect the legacy default database only when its location is unambiguous. Offer a non-destructive import/copy into the new default workspace; show source and destination paths and require confirmation if the source has data.
3. Do not silently merge two databases. Duplicate run IDs, profile records, retention rules, and audit histories require an explicit import command with backup and validation.
4. Preserve repository-local `.spectide/config.json` files. They remain project configuration, not shared workspace configuration.
5. `serve --db PATH --port PORT` continues to support isolated demos, tests, and parallel development environments.

## Error messages that teach the model

| Situation | Required user-facing guidance |
| --- | --- |
| Dashboard port is occupied by the same workspace | State the existing URL and suggest `spectide dashboard`. |
| Dashboard port is occupied by another process/workspace | Identify that the port cannot be claimed and suggest `serve --port PORT`; do not claim ownership. |
| Current directory is unregistered | State the canonical directory and show `spectide add PATH`. |
| Project has no route | Offer `spectide configure` or point to the route screen; do not start a run. |
| Profile does not authorize the project | Name the profile and project boundary, then show the exact least-privilege remediation. |
| Different databases are in use | State that dashboard and command histories differ because their workspace stores differ, and show the selected workspace identities—not raw assumptions. |

## Acceptance criteria

1. A fresh user can add two repositories, start one dashboard at `127.0.0.1:4747`, and view runs from both without ever typing `--db`, `--host`, or `--port`.
2. A run launched from each registered repository is attributed to the correct canonical project; the dashboard can show each project alone and all projects together.
3. Two runs in different projects can proceed concurrently, while a second run in the same canonical project remains rejected by the existing workspace lock.
4. `spectide dashboard` detects and reuses a healthy server for the same workspace. A second `serve` produces the actionable message above.
5. All project/run API filters are authorized, bounded, covered by tests, and cannot cause one project’s route/configuration to be used for another.
6. Explicit `--db` workflows, existing scripts, offline demo isolation, and project-local configuration continue to work unchanged.
7. Migration creates a backup, never deletes legacy state, and leaves the user with one verifiable workspace identity.
8. Dashboard keyboard navigation, accessible labels, empty states, and narrow-screen behavior are covered for the project switcher and add-project flow.

## Delivery slices

1. **Foundation:** platform-aware default workspace resolver, workspace identity, clear `serve` collision detection, and revised documentation/help text.
2. **CLI onboarding:** `setup`, `add`, current-directory inference, idempotency, and scripted/interactive tests.
3. **Project-aware data:** run filtering and project readiness summaries in the API, with access and pagination tests.
4. **Dashboard:** project switcher, filtered views, add-project experience, and accessibility/browser tests.
5. **Migration:** opt-in legacy-state detection/import, backup/restore verification, release notes, and troubleshooting guidance.

## Non-goals

- Remote hosting, multi-user accounts, team permissions, or synchronizing project state between machines.
- Automatically discovering every directory on a machine or automatically granting a provider access to it.
- Removing `--db`, `--port`, or `--project`; advanced and automated workflows need those explicit controls.
- Combining project-local SpecTide route configuration into a single global configuration.

## Documentation changes required with delivery

Update the README’s five-minute tutorial and everyday workflow, the local user guide, CLI help examples, troubleshooting table, API/OpenAPI contract, and screenshots. The first commands shown to a new user should be `spectide setup`, `spectide add .`, and `spectide dashboard`; database and port overrides should appear only in an “Advanced / isolated workspace” section.
