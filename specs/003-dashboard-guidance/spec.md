# Feature Specification: Dashboard Guidance and Route Recovery

**Feature Branch**: `003-dashboard-guidance`

**Created**: 2026-10-24

**Status**: Draft

**Input**: User description: "Improve the SpecTide dashboard so it is easier to understand and less redundant, and fix the confusing errors shown after selecting Preview routes."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Understand What to Do Next (Priority: P1)

A developer opening a selected project's dashboard can immediately understand whether the project is ready to run and, if it is not, see the single next setup action in the correct order. They do not need to infer a workflow from repeated forms, internal status labels, or the navigation structure.

**Why this priority**: A clear next step is the minimum value of the dashboard. Without it, users can attempt previews or runs that the product already knows cannot succeed.

**Independent Test**: With projects in each supported readiness state, open the dashboard, select each project, and verify that its plain-language state, ordered prerequisite, and next action are visible before attempting a preview or workflow.

**Acceptance Scenarios**:

1. **Given** a selected project has no usable route, **When** the developer opens Workflow control, **Then** the dashboard identifies route setup as the next step, explains why preview and live work are unavailable, and offers one direct route-setup action.
2. **Given** a selected project has a route that references an unavailable, unauthorized, or unverified provider profile, **When** the developer opens Workflow control, **Then** the dashboard identifies provider setup as the next step, distinguishes its condition in plain language, and offers one direct profile-readiness action.
3. **Given** a selected project is ready, **When** the developer opens Workflow control, **Then** the dashboard presents the primary workflow action and makes the selected project unmistakable without asking the developer to re-enter its path.
4. **Given** no individual project is selected, **When** the developer views the dashboard, **Then** the dashboard makes the all-project scope clear and asks the developer to choose a project before offering project-specific preview or run actions.

---

### User Story 2 - Recover From a Blocked Route Preview (Priority: P1)

A developer who previews routes receives a human-readable outcome that says what was checked, why it is blocked, what did not run, and the best next action. They can recover without decoding raw error data or guessing which screen to visit.

**Why this priority**: Route preview is intended to build confidence before starting work. A raw blocked response currently does the opposite and creates a support burden.

**Independent Test**: Attempt a preview for a project with a missing route, an absent named profile, a profile outside the permitted project boundary, unverified capability evidence, and a valid configuration. Verify that every outcome is understandable and offers the correct recovery without a workflow being created.

**Acceptance Scenarios**:

1. **Given** route preview is blocked by a missing provider profile referenced by a stage, **When** the result is displayed, **Then** it names the missing profile and affected stage in user-friendly language, confirms that no workflow ran, and provides the appropriate profile setup action.
2. **Given** a preview is blocked by a missing route, profile authorization boundary, or unverified provider capability, **When** the result is displayed, **Then** it explains that specific prerequisite and provides the matching corrective action without exposing a raw serialized error response.
3. **Given** the developer follows the corrective action and returns to Route preview, **When** the prerequisite is resolved, **Then** they can retry the preview without re-entering the selected project or task information.
4. **Given** a preview succeeds, **When** results are displayed, **Then** the developer can see the resolved provider choice for each workflow stage and a clear statement that no workflow or repository transfer occurred.

---

### User Story 3 - Work Without Repeated Inputs (Priority: P2)

A developer can choose a project once, compose a task once, preview the routes for that task, and start the workflow from the same context. Optional project registration and historical inspection remain available without competing with the primary task.

**Why this priority**: Repeated project paths, duplicated task fields, and simultaneous setup, preview, history, and run controls make a simple workflow appear more complex than it is.

**Independent Test**: From a ready project, choose a project, enter a task, preview routes, navigate to the workflow action, and start it. Verify that the chosen project and task are retained and the developer never has to type the project path in normal dashboard use.

**Acceptance Scenarios**:

1. **Given** a project is selected, **When** the developer moves between Workflow control and Route preview, **Then** the same selected project and task draft are retained and shown as context rather than duplicate editable project-path fields.
2. **Given** a developer needs to add or switch projects, **When** they use the project controls, **Then** those secondary actions are available without obscuring the selected project's readiness and primary next step.
3. **Given** a project cannot start work, **When** the developer opens the dashboard, **Then** a start action cannot misleadingly appear ready; the dashboard instead explains the gating prerequisite.
4. **Given** a developer runs the offline demonstration, **When** it is offered, **Then** the dashboard clearly labels it as a separate simulated path and does not imply that it validates the selected project's live readiness.

### Edge Cases

- A project changes from ready to blocked after the dashboard loads: preview and start re-check readiness, preserve the task draft, and show the current actionable prerequisite rather than claiming a run started.
- Multiple prerequisites are missing: show the first required action and indicate that additional checks will follow; do not overwhelm the developer with a raw list of internal failures.
- A profile name is known but no profile record exists: identify the expected name and affected stage without exposing credentials, local account data, or unrelated profile details.
- The local dashboard cannot reach its service: distinguish connectivity from project readiness and do not give a misleading setup recommendation.
- The selected project is removed or becomes unavailable: clear the project-specific action safely, retain any unsent task draft, and ask the developer to select a valid project.
- The developer uses a keyboard or narrow screen: readiness, scope, corrective actions, and preview result remain reachable, ordered, and understandable without relying on color alone.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The dashboard MUST show the active project scope prominently on every project-specific workflow surface and identify all-project scope as non-actionable for preview and workflow start.
- **FR-002**: For a selected project, the dashboard MUST present a concise readiness summary containing the current condition, its user-facing consequence, and the single highest-priority next action.
- **FR-003**: The dashboard MUST order readiness prerequisites so configuration and route issues are resolved before provider-profile authorization and capability issues, and MUST describe additional checks as pending rather than presenting them as simultaneous failures.
- **FR-004**: The dashboard MUST make the primary workflow action available only when the selected project is ready; otherwise, it MUST present the corrective action in its place or immediately alongside it.
- **FR-005**: The dashboard MUST preserve one task draft while a developer moves between workflow preparation and route preview for the same selected project.
- **FR-006**: In ordinary dashboard use, the selected project's canonical path MUST be context information, not an editable field required by both route preview and workflow start.
- **FR-007**: The dashboard MUST retain a deliberate way to select, switch, and add approved projects without presenting project registration as an equal-priority task to running the selected project.
- **FR-008**: A route preview MUST state that it is a dry check and MUST clearly distinguish a ready preview, a blocked preview, an unavailable local connection, and a preview that has not been requested.
- **FR-009**: When a route preview is blocked, the dashboard MUST translate the reported condition into plain language, identify the relevant route prerequisite and affected workflow stage when available, confirm that no workflow was created, and offer a direct corrective action.
- **FR-010**: Blocked-preview presentation MUST NOT expose raw serialized error payloads, request identifiers, or unnecessary internal diagnostic structure as the primary user-facing message.
- **FR-011**: For a missing named provider profile, the dashboard MUST identify the expected profile name and provider context, guide the developer to the supported profile setup and readiness check, and protect credentials and unrelated profile details.
- **FR-012**: For a profile that is unauthorized for the project or lacks verified capability evidence, the dashboard MUST distinguish those conditions from an absent profile and direct the developer to the corresponding least-privilege remedy.
- **FR-013**: After a developer completes a corrective action, the dashboard MUST preserve the selected project and task draft and make retrying the route preview or starting work straightforward.
- **FR-014**: Readiness and route-preview actions MUST re-check the project at the time of the action and report current status honestly; no blocked check may be represented as a run or successful validation.
- **FR-015**: The offline demonstration MUST remain available as a visibly simulated, separate learning path and MUST NOT be presented as evidence that the selected project is configured for live work.
- **FR-016**: Project scope controls, readiness guidance, corrective actions, route outcomes, and primary workflow actions MUST support keyboard operation, readable labels, focus visibility, and narrow-screen layouts.

### Key Entities

- **Project readiness summary**: The selected project's current preparation state, its plain-language explanation, the next required action, and any later checks not yet evaluated.
- **Task draft**: A developer's unsent workflow request associated with one selected project and retained while the developer reviews routes or resolves prerequisites.
- **Route preview outcome**: The non-executing result of checking routes for a selected project and task, including its state, affected stages when known, user-facing explanation, and recovery action.
- **Corrective action**: A context-specific path that takes the developer to the existing route, profile, readiness, or project-selection capability needed to resolve the present blocker.
- **Workflow scope**: The explicitly selected project that bounds preview and start actions; all-project scope is for observation and selection only.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In first-use usability testing, at least 90% of developers can identify the selected project's next required action within 30 seconds without reading documentation or inspecting raw error text.
- **SC-002**: In acceptance testing of missing route, missing profile, unauthorized profile, and unverified capability conditions, 100% of blocked preview outcomes name a plain-language cause, state that no workflow ran, and provide the matching recovery action.
- **SC-003**: In a usability test with a ready project, at least 90% of developers can select a project, enter one task, preview its routes, and start the workflow in under 3 minutes without manually entering a project path.
- **SC-004**: In route-preview acceptance testing, 100% of blocked responses avoid presenting raw serialized error data as the primary user-facing content.
- **SC-005**: In keyboard-only and narrow-screen acceptance testing, 100% of tested users can identify project scope, reach the displayed corrective action, retry a preview, and distinguish simulated from live readiness without relying on color alone.
- **SC-006**: In acceptance testing where readiness changes after page load, 100% of preview and start attempts report the current blocked state and create no workflow when a prerequisite is unmet.
- **SC-007**: In a post-release survey of developers who used route preview, at least 85% report that they understood both the preview result and their next action without consulting troubleshooting documentation.

## Assumptions

- SpecTide remains a local-first, single-user developer tool; this feature improves existing guidance and recovery rather than adding remote accounts, team roles, or credential storage.
- Existing route configuration, profile registration, capability checking, and project-scope protections remain authoritative; the dashboard makes their prerequisites clear rather than bypassing or weakening them.
- The first deliverable focuses on the everyday desktop dashboard while retaining usable keyboard and narrow-screen behavior; a separate mobile-native redesign is out of scope.
- Adding a provider profile continues to use the product's supported local setup path; this feature may guide users there but does not assume the dashboard stores or handles provider credentials.
- Detailed diagnostics remain available through established troubleshooting and support workflows, but ordinary dashboard recovery leads with concise, actionable explanations.
- Existing run history, route details, and offline demonstration behavior remain available; their presentation may be reorganized to reduce cognitive load without misrepresenting persisted evidence.
