# Feature Specification: Shared Workspace Dashboard

**Feature Branch**: `002-shared-workspace-dashboard`

**Created**: 2026-10-24

**Status**: Draft

**Input**: User description: "based on docs/multi-project-ux-proposal.md this plan please generate a spec to improve spectide"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Set Up and Use Multiple Projects (Priority: P1)

A developer setting up SpecTide for the first time can prepare the current repository, then add another repository to the same local workspace. They can start workflows from either registered repository without choosing a state location, host, or port for normal use.

**Why this priority**: This establishes the core value: one local product for a developer's deliberately approved projects rather than disconnected repository-specific installations.

**Independent Test**: On a machine with no existing workspace, complete guided setup in one repository, add a second repository, and start one workflow from each. Both workflows are associated with their respective projects in the shared workspace.

**Acceptance Scenarios**:

1. **Given** a developer is in an unregistered repository with no local workspace, **When** they run guided setup and confirm the displayed repository path, **Then** the shared workspace and that project are prepared, project-specific configuration is created only after confirmation, and the dashboard is made available.
2. **Given** a developer is in a second unregistered repository, **When** they add the current directory, **Then** only that canonical directory is registered and the developer receives clear guidance for missing project configuration or provider authorization.
3. **Given** two registered repositories, **When** the developer starts a workflow from each, **Then** each workflow is attributed to the correct canonical project without ordinary-use connection or state-location options.
4. **Given** a registered repository, **When** the developer repeats setup or adds it again, **Then** the result reports that the project is already ready and does not create duplicate project records or configuration.

---

### User Story 2 - Open One Reliable Dashboard (Priority: P1)

A developer can open one local dashboard for all registered projects. If it is already running for their default workspace, the dashboard command reuses it; if not, it starts it and only opens the dashboard after it is reachable.

**Why this priority**: A reliable single entry point removes the most confusing current experience: conflicting dashboard instances and unexplained port failures.

**Independent Test**: Open the dashboard twice from different registered repositories and verify both commands lead to the same healthy default workspace at `http://127.0.0.1:4747`; separately verify the guidance when that address is unavailable.

**Acceptance Scenarios**:

1. **Given** the default dashboard is not running, **When** the developer runs the dashboard command, **Then** one dashboard for the default workspace becomes reachable at `http://127.0.0.1:4747` before the URL is opened or reported.
2. **Given** a healthy dashboard for the default workspace is already running, **When** the developer runs the dashboard command again, **Then** the existing dashboard is reused rather than a second instance being started.
3. **Given** another default dashboard is already running for the same workspace, **When** a developer tries to start one, **Then** they are shown the existing URL and directed to the dashboard command, with an option for intentionally isolated use.
4. **Given** the default dashboard address is occupied by another process or workspace, **When** a developer attempts to use it, **Then** the message states that ownership cannot be confirmed and explains how to use an isolated address without falsely claiming the running process.

---

### User Story 3 - View and Act Within the Intended Project (Priority: P2)

A developer can view all projects or select one registered project in the dashboard. The selected scope is conspicuous, filters project-specific activity, and supplies the default project for workflow and route actions. The developer can deliberately add a project from the dashboard without exposing repository contents.

**Why this priority**: Clear scope and safe switching prevent users from acting on or interpreting data from the wrong project.

**Independent Test**: With two registered projects and workflows in each, select all projects and then each individual project; verify scope, activity, and default actions change appropriately. Add a third project through the dashboard and verify it appears without transferring another project's configuration or permissions.

**Acceptance Scenarios**:

1. **Given** a workspace with runs from multiple projects, **When** the dashboard first opens after upgrade, **Then** it shows an all-projects view and identifies the count and scope of displayed activity.
2. **Given** a developer selects a registered project, **When** the selection is applied, **Then** run history, recent activity, workflow defaults, and route preview reflect only that project and the canonical path is available as secondary information.
3. **Given** a saved project selection is missing or no longer valid, **When** the dashboard opens, **Then** it safely falls back to the all-projects view.
4. **Given** a selected project has no workflows, configuration, route, or authorized provider, **When** its empty state is displayed, **Then** the dashboard explains the applicable condition and offers one direct next action without starting a workflow.
5. **Given** a developer enters an absolute local path in the dashboard's add-project action, **When** it is submitted, **Then** the project is registered only after path validation and no repository content is browsed or transmitted as part of adding it.

---

### User Story 4 - Preserve Existing and Advanced Workflows (Priority: P2)

An existing developer retains project-local configuration and intentional isolated-workspace workflows while being offered a safe, explicit path to adopt the new default workspace.

**Why this priority**: The feature must improve everyday use without breaking scripts, demos, testing environments, or existing local history.

**Independent Test**: Use an existing explicit workspace successfully after upgrade, choose a legacy-state import when offered, and verify the original state remains intact and the imported workspace can be identified.

**Acceptance Scenarios**:

1. **Given** a developer explicitly selects a workspace for a command, **When** the command runs, **Then** that explicit selection takes precedence over the default workspace.
2. **Given** unambiguous legacy default state containing data, **When** the developer is offered migration, **Then** the source and destination are shown and no data is copied unless the developer confirms.
3. **Given** migration is confirmed, **When** it completes, **Then** a backup exists, the legacy state is not deleted, and the developer can verify the identity of the resulting workspace.
4. **Given** two state stores are not the same, **When** a developer encounters different histories, **Then** the product explains that the histories differ because the selected workspaces differ and identifies the relevant workspaces without exposing sensitive storage details.

### Edge Cases

- A developer starts a workflow outside a registered project: show the canonical directory and the exact add-project remediation; do not start a workflow.
- The selected project lacks a route or provider authorization: block workflow creation, name the missing prerequisite and project boundary, and offer the least-privilege remediation.
- A selected path is a file, inaccessible directory, symlink escape, or duplicate canonical directory: reject registration with an actionable explanation and do not create a duplicate or broaden access.
- Two workflows begin in the same canonical project: reject the competing writer while allowing workflows in different registered projects to proceed concurrently.
- A dashboard session has a stale remembered project selection: restore the all-projects view rather than presenting incorrect data or actions.
- A legacy state location is ambiguous or two stores would need merging: do not migrate automatically; require an explicit import process that preserves backups and validates results.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide one stable, user-level default local workspace shared by all explicitly registered projects on the same machine.
- **FR-002**: Guided setup MUST prepare or open the default workspace, display the canonical current repository path, require confirmation before creating project-local configuration, and guide the developer to a project-scoped authorized provider profile.
- **FR-003**: Adding a project MUST register only a validated canonical directory in the default workspace and MUST not copy routes, permissions, profiles, or secrets from another project.
- **FR-004**: Setup and add-project operations MUST be idempotent for an already registered canonical project and MUST report its ready or incomplete state without duplicating records or configuration.
- **FR-005**: A workflow started from a registered project directory MUST infer that project; a workflow started elsewhere MUST not start and MUST provide the canonical path and an exact add-project remediation.
- **FR-006**: The system MUST retain an explicit project-selection option for scripted and cross-directory use.
- **FR-007**: The normal dashboard command MUST make one dashboard for the default workspace available at `http://127.0.0.1:4747`, reusing a reachable dashboard only after confirming it belongs to that workspace.
- **FR-008**: The dashboard command MUST not report or open the dashboard until it is reachable.
- **FR-009**: When the default dashboard address is occupied, the system MUST distinguish a confirmed dashboard for the same workspace from an unverified process or different workspace and provide corresponding actionable guidance.
- **FR-010**: The dashboard MUST offer an all-projects view and an individually selectable view for every registered project, with the active scope visibly identified.
- **FR-011**: Selecting a project in the dashboard MUST limit displayed workflow history and recent activity to that project, where recent activity means the filtered recent-run list and events for a selected run in that scope; it MUST set that project as the default for dashboard-initiated workflow and route actions.
- **FR-012**: The dashboard MUST preserve a valid project selection between visits and MUST fall back to all projects when the saved selection is malformed, unavailable, or unauthorized.
- **FR-013**: The dashboard MUST provide an add-project action that accepts an absolute local path, validates it under the same project-registration protections as the command line, and does not browse or transmit repository contents.
- **FR-014**: The dashboard MUST provide distinct, actionable empty states for a project with no workflows, missing configuration, missing route, or missing provider authorization.
- **FR-015**: Before creating a workflow, the system MUST resolve the selected project to its registered canonical path and revalidate that project's configuration, authorization, capabilities, and single-writer protection.
- **FR-016**: The system MUST allow workflows in different registered projects to proceed concurrently while preventing competing workflow writers within the same canonical project.
- **FR-017**: The system MUST preserve unfiltered historical views and intentional explicit-workspace, custom-address, and project-selection workflows for existing scripts, demos, tests, and isolated development.
- **FR-018**: On first adoption, the system MUST detect legacy default state only when its location is unambiguous; it MUST never silently merge state stores.
- **FR-019**: Any migration that copies legacy state containing data MUST show source and destination, require confirmation, create a backup, retain the source, and leave a verifiable workspace identity.
- **FR-020**: Project selection, paths, readiness summaries, and workflow filtering MUST enforce project boundaries and bounded result sets; no action or response may use one project's route or configuration for another project.
- **FR-021**: The project switcher and add-project flow MUST support keyboard use, accessible labels, and narrow-screen operation.

### Key Entities

- **Local workspace**: A user-level, machine-local collection of registered projects, workflow history, workspace identity, and dashboard state. It is distinct from each repository's project configuration.
- **Registered project**: An explicitly approved canonical repository directory within a local workspace, identified by display information, readiness state, project-local configuration, and provider authorization boundary.
- **Project readiness summary**: The current indication of whether a registered project has the required configuration, route, and authorized provider to start a workflow.
- **Workflow run**: A recorded unit of work associated with exactly one registered project's canonical path and governed by that project's validation and writer protection.
- **Dashboard instance**: The local dashboard associated with one identifiable workspace and its availability status; its identity prevents one workspace from being mistaken for another.
- **Migration record**: The user-confirmed transfer outcome for legacy state, including source, destination, backup, validation result, and resulting workspace identity.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a first-use usability test, at least 90% of developers can set up one repository, add a second repository, and view both in one dashboard within 10 minutes without entering a state location, host, or port.
- **SC-002**: In acceptance testing, 100% of workflows launched from each of two registered repositories are attributed to the correct canonical project, and 100% of selected-project views exclude workflows from the other project.
- **SC-003**: In acceptance testing, two workflows in different registered projects can start concurrently in 100% of attempts, while a concurrent second workflow in the same project is rejected with actionable guidance in 100% of attempts.
- **SC-004**: For a healthy existing default dashboard, at least 99% of dashboard-command attempts reuse the identified dashboard and present its URL within 5 seconds; no attempt starts a duplicate default dashboard.
- **SC-005**: In keyboard-only and narrow-screen acceptance testing, 100% of tested users can select all projects or an individual project and initiate the add-project flow, with the active scope understandable without relying on color alone.
- **SC-006**: In migration acceptance testing, 100% of confirmed legacy imports retain the source state and produce a recoverable backup; 0 unconfirmed, ambiguous, or merge-required migrations modify existing state.
- **SC-007**: In a post-release developer survey of users managing two or more local projects, at least 85% report that it is clear which project scope the dashboard is showing.

## Assumptions

- SpecTide remains a single-user, local-first product; remote hosting, multi-user accounts, shared-machine synchronization, and team permissions are outside this feature.
- Projects are deliberately added by the developer; the product does not automatically scan a machine for repositories or expand provider access.
- Existing project-local configuration remains the source of project-specific route configuration and contains no stored authentication secrets.
- The established provider authorization and project validation model remains authoritative; this feature improves discoverability and makes its boundaries visible rather than broadening them.
- Advanced workflows continue to support explicitly selected workspaces, addresses, and projects; these controls are documented outside the everyday first-use path.
- Legacy-state migration is opt-in and applies only where the existing default location can be identified unambiguously; importing or merging arbitrary state stores is outside the automated migration flow.
- Documentation, command help, troubleshooting guidance, contract references, and screenshots will be updated with delivered behavior so first-use materials lead with setup, add-project, and dashboard actions.

## Post-convergence evidence review

The implementation evidence matrix and current verification results are recorded in [quickstart.md](quickstart.md). The review found automated evidence for most functional requirements; FR-006, FR-008, FR-018, FR-019, and FR-021 retain acceptance gaps, and SC-001, SC-005, SC-006, and SC-007 remain open or partial as described there. These gaps are not represented as passed.
