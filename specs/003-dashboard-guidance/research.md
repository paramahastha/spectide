# Research: Dashboard Guidance and Route Recovery

## Decision: Use the existing project ID as the only ordinary dashboard execution selector

**Rationale**: `activeProjectId` already selects the registered project sent to both preview and start requests. The editable project-path state is redundant and is not used as the request selector. Removing it prevents a developer from seeing or changing a path that has no effect while retaining server-side canonical-path resolution.

**Alternatives considered**:

- Keep project-path inputs and synchronize them with the selected project: rejected because it creates conflicting authority signals and repeats input.
- Send browser-entered paths for preview/start: rejected because it weakens the registered-project boundary and contradicts the existing ID-scoped API pattern.

## Decision: Keep one in-memory task draft shared by route preview and workflow start

**Rationale**: The current prompt state is already shared by requests but displayed twice. One editor prevents redundant entry and enables retry after a corrective action. It remains in page memory rather than persistent browser storage because task text may contain sensitive repository context.

**Alternatives considered**:

- Maintain separate preview and workflow prompts: rejected because their divergence makes a preview unreliable as a preflight for the run.
- Persist drafts in local storage: rejected because it introduces an unnecessary local retention path for potentially sensitive text.

## Decision: Add a bounded server-generated blocked-preview projection

**Rationale**: The API currently returns a redacted error envelope with `NOT_RUN` evidence, but the browser throws and renders the whole JSON body. A stable category, safe message, remediation target, and optional stage/profile/provider fields let the dashboard guide recovery without parsing errors or exposing sensitive diagnostic detail.

**Alternatives considered**:

- Parse error strings in the browser: rejected because wording is unstable, error detail is not a user interface contract, and it could reveal unintended information.
- Keep only a generic blocked message: rejected because it cannot explain the screenshot's missing-profile situation or direct the user to the appropriate remedy.
- Return full route, scope, or provider diagnostic data: rejected because least-privilege and redaction require a minimal recovery projection.

## Decision: Use the existing readiness status ordering for the workflow's single next action

**Rationale**: The project service already computes configuration, route, profile, authorization, capability, and ready states in dependency order. A pure UI mapping can turn those safe statuses into one concise card without adding persistence or duplicate server logic.

**Alternatives considered**:

- Return every possible blocker in readiness: rejected because later checks cannot be truthfully evaluated until earlier configuration is valid and the user requested one clear next action.
- Add a separate dashboard-specific readiness store: rejected because it would duplicate an existing server projection and risk stale authority.

## Decision: Preserve the existing dry-run endpoint and evidence model

**Rationale**: `POST /api/v1/runs` with `dry_run: true` already validates registered-project path, safe configuration access, route/profile scope, capability evidence, and decisions without creating a run. It is shared with command-line validation and has no-run API tests.

**Alternatives considered**:

- Create a dashboard-only preview endpoint: rejected because it would duplicate security-sensitive routing validation.
- Treat a blocked preview as a persisted run: rejected because it would falsely imply execution and violate explicit evidence-state semantics.

## Decision: Apply the selected app-screen direction to hierarchy, not a visual rewrite

**Rationale**: The `true-black-flat` developer-tool direction preserves the existing dark brand while making readiness the dominant answer, reducing decorative/duplicate panels, and limiting color to status/data. The product-specific readiness path gives the screen a recognizable SpecTide purpose without inventing metrics or fake progress.

**Alternatives considered**:

- Use a generic KPI/dashboard layout: rejected because it buries the action users came to perform and would repeat the current cognitive overload.
- Use `data-terminal`: rejected for this flow because its monitoring/long-session selection language competes with the calmer one-next-action setup experience.
- Rebrand the dashboard with a new palette/type system: rejected because the feature is a usability/recovery improvement and should preserve established product identity.

## Decision: Extend existing test layers and include rendered visual inspection

**Rationale**: Existing Go tests prove no-run and route-security behavior; Vitest covers component states/accessibility; Playwright with Axe covers rendered browser flows. The design skill adds a screenshot-based look-and-fix loop so composition, density, focus, and narrow-screen behavior are checked rather than assumed.

**Alternatives considered**:

- Component tests only: rejected because they cannot prove server redaction, no-run behavior, or rendered hierarchy.
- API tests only: rejected because the raw JSON defect exists in browser error handling and needs rendered accessibility coverage.
