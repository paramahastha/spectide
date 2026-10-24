# Quickstart Validation: Dashboard Guidance and Route Recovery

Use disposable local projects and the fake-provider fixture for automated checks. Do not add provider credentials to configuration files or use an unapproved live provider account. Refer to [data-model.md](data-model.md), [HTTP contract](contracts/http-api.md), and [dashboard UI contract](contracts/dashboard-ui.md) for expected behavior.

## Prerequisites

- Go 1.27.2 and Node.js 26 with dashboard dependencies installed.
- A built local binary and current dashboard assets.
- Fixtures for a registered project in each state: missing route, missing profile, unauthorized profile, unverified capability, and ready fake-provider route.

## 1. Static and automated checks

```sh
make format-check
make lint
make test
make web-test
make web-build
make web-e2e
```

**Expected outcome**: Go and browser tests pass. The test suite proves that a successful and a blocked preview both record no workflow run, raw serialized error responses are not the primary dashboard message, keyboard actions are reachable, and the UI remains usable at narrow/zoomed layouts.

## 2. Missing route guidance

1. Register and select a project with no usable default route.
2. Open **Workflow control**.
3. Attempt the offered corrective action, then return to the workflow view.

**Expected outcome**:

- The project scope is clear.
- The readiness path says that routes need configuration and presents **Review route settings** as the sole next action.
- The dashboard does not make preview/start look ready and does not ask for an editable project path.
- No workflow exists as a consequence of viewing or following the guidance.

## 3. Missing-profile preview recovery

1. Configure a selected project with a route that names a profile that is not registered.
2. Enter a task once and select **Preview routes**.
3. Follow **Review profile setup**, then return to preview.

**Expected outcome**:

- The blocked card explains that the named profile is needed and identifies the affected stage when known.
- It visibly states `NOT_RUN`, contains no serialized error envelope or request identifier as the main message, and exposes no credential/scope detail.
- The selected project and task draft are still present on return.
- Server/API tests confirm zero new run records and no provider invocation.

## 4. Authorization and capability recovery

Repeat the preview with a registered profile that is outside the selected project's allowed boundary, then with a profile without verified capability evidence.

**Expected outcome**:

- The dashboard distinguishes profile access from capability readiness.
- Each response presents its relevant recovery action and does not disclose raw allowed scopes, internal validation text, or unrelated profiles.
- Retrying after resolving the prerequisite produces a ready dry preview only when server validation succeeds.

## 5. Ready project flow and redundant-input regression

1. Select a project with a verified fake-provider route.
2. Enter one task draft on Workflow control.
3. Preview its routes, return to Workflow control, and start the workflow.
4. Run the offline demonstration separately.

**Expected outcome**:

- No normal preview/start surface requires an editable project path.
- The same project ID and one task draft drive preview and start; changing either invalidates the former preview.
- A ready preview lists resolved routes and remains `NOT_RUN` until the distinct start action is used.
- The offline demo is visibly simulated and is not evidence of the selected project's live readiness.

## 6. Accessibility, visual, and stale-state validation

1. Complete the state matrix using only keyboard navigation.
2. Repeat at 200% zoom and a 390px-wide browser viewport.
3. Change a project's configuration/readiness after the dashboard has loaded, then preview/start.
4. Remove the selected project or disconnect the local API.
5. Capture the before/after workflow screen at 1440px, 768px, and 390px; inspect the rendered application using the app-screen visual rules.

**Expected outcome**:

- Scope, readiness path, recovery action, and preview result have readable labels and visible focus without relying on color.
- Required actions stay reachable without horizontal scrolling; the current readiness/path/action appears before secondary run history on narrow screens.
- A fresh preview/start reports the current state, preserves only valid in-memory draft context, and never creates a run for a blocked condition.
- Disconnection is clearly separate from a project setup recommendation.
- The dark dashboard uses flat lightness elevation with no gradients, glows, colored nav, tinted icon tiles, or generic KPI-card decoration.

## Evidence recording

Record each command and scenario as `PASS`, `FAIL`, `BLOCKED`, or `NOT_RUN`. Label fake-provider evidence as simulated. Live provider validation remains `NOT_RUN` unless an authorized account is explicitly approved for the test.
