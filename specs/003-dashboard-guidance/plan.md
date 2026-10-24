# Implementation Plan: Dashboard Guidance and Route Recovery

**Branch**: `003-dashboard-guidance` | **Date**: 2026-10-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/003-dashboard-guidance/spec.md`

## Summary

Make the dashboard guide a developer through one selected project's readiness instead of presenting repeated project-path and task inputs. Preserve the server-side preview safety check, but return a bounded, user-safe blocked-preview projection that the dashboard can render as a specific recovery action rather than raw JSON. Keep task drafts in browser memory, use the existing registered-project ID as the sole preview/start selector, and cover the guidance with API, component/accessibility, browser, and visual QA tests.

## Technical Context

**Language/Version**: Go 1.27.2; TypeScript 5.9.3

**Primary Dependencies**: Cobra CLI; SQLite driver; React 19.2; Vite; Vitest; Testing Library; Playwright with Axe

**Storage**: Existing local SQLite workspace store for projects, profiles, capability evidence, and workflow records. This feature adds no persisted database entity; the selected project remains an existing persisted preference and the task draft remains in browser memory only.

**Testing**: Go package/API tests; Vitest component and accessibility tests; Playwright with Axe; design-taste lint/inspection and screenshots; `make verify` for the full local quality flow.

**Target Platform**: Single-user local dashboard bound to loopback, in supported Go/Node development environments.

**Project Type**: Local Go orchestration service and CLI with a React dashboard.

**Interaction Constraints**: Do not add a background polling loop. Primary interactions must remain usable at narrow width and 200% zoom.

**Constraints**: Preserve registered-project boundaries, safe path canonicalization, CSRF/idempotency protections, redaction, non-secret profile observations, and truthful `NOT_RUN` dry-run evidence. Browser code must not parse raw internal error strings or persist task text outside the active page session.

**Scale/Scope**: One local developer selects one registered project at a time; existing all-project history remains observational. Change is limited to dashboard guidance, bounded preview-blocker data, and related tests/documentation—no provider execution, credential, route-editor, or workspace-storage redesign.

## Constitution Check

### Pre-design gate — PASS

| Principle | Plan response |
| --- | --- |
| I. Local-First, Least-Privilege Operation | Retains loopback-only API and registered project ID selection. Preview blockers are bounded and redacted; they expose no credentials, raw scopes, repository content, or arbitrary filesystem capability. |
| II. Evidence Before Completion | Keeps dry-run outcomes explicitly `NOT_RUN`; blocked responses create no workflow. Tests must prove the state and recovery message, not infer success from UI appearance. Offline demo remains visibly simulated. |
| III. One Product Scope, Real Spec-Kit Semantics | Extends the active feature specification only; no duplicate product scope or simulated planning result. |
| IV. Durable, Observable, and Recoverable Workflows | The dashboard remains a projection of server-side readiness and preview checks. No scheduler or workflow state moves into the browser; blocked and unavailable states remain explicit. |
| V. Provider-Neutral, Accessible, and Maintainable Delivery | Uses the existing provider-neutral readiness taxonomy and typed, additive HTTP contract. It adds accessible component and browser coverage without new dependencies. |

No constitutional exception or complexity justification is required.

### Post-design re-check — PASS

The data model and contract preserve all of the pre-design safeguards: no new persisted sensitive data, no browser authority expansion, one canonical server preview check, explicit evidence states, and accessible testable presentation. The only endpoint change is an additive, redacted blocked-preview projection.

## Experience Direction

**App-screen direction**: `true-black-flat` for a dark developer-tool overview (seed `1504748980`; sheet at `.taste-skill/app/direction.json`). The existing dark palette/type are retained, so the direction governs composition and hierarchy rather than triggering a brand migration.

**Screen question**: “What must I do next to safely run this project?” The answer is the dominant block on the workflow screen, not a secondary warning competing with history and duplicate forms.

**Product face and signature element**: A compact **readiness path** for the selected SpecTide project. It shows the server-observed progression—route, profile access, capability evidence, dry preview, then start—with only the current unmet step actionable. This creates a product-specific visual/interaction grammar from SpecTide's own durable workflow stages rather than generic dashboard KPI cards.

**Composition rules**:

- Keep the existing true-black ground, elevation by lightness steps, minimal color budget, and bright-text active navigation. Do not add glow, gradients, glass, tinted icon tiles, or colored navigation/button chrome.
- Use the existing rail only for navigation; the top bar must carry the selected project/scope and real utility, not decorative empty chrome.
- The workflow canvas begins with the selected-project context and readiness path, followed by the single task draft and a preview/start decision area. Recent history and run detail become secondary evidence rather than competing first-screen actions.
- Use compact, sentence-case labels and normal dashboard type scale. Status color is reserved for state/data; text and position must make all states understandable without color.
- Keep panels flat and purposeful. The readiness path and recovery card should never use empty KPI-style tiles, generic icons, or visualized progress that implies a server check has passed.
- For narrow screens, fold navigation and retain scope, current readiness, task draft, and next action above history. Ensure 44px action targets, visible focus, and no page-level horizontal scrolling.

**Visual verification loop**: Capture before/after screenshots at 1440px, 768px, and 390px; inspect the rendered dashboard with the app-screen inspector; review state screens for all readiness and route-preview outcomes; correct identified hierarchy, density, focus, overflow, or anti-pattern issues before handoff. This feature has no decorative motion requirement—only responsive interaction feedback and reduced-motion-safe state changes.

## Project Structure

### Documentation (this feature)

```text
specs/003-dashboard-guidance/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-api.md
│   └── dashboard-ui.md
└── tasks.md                 # Created later by /speckit.tasks
```

### Source Code

```text
cmd/
└── spectide/                # Local binary entrypoint

internal/
├── api/                     # HTTP envelope and dry-run preview response
├── model/                   # Existing project/readiness projections
├── projects/                # Read-only project readiness calculation
├── routing/                 # Canonical route/profile/capability validation
└── security/                # Error redaction

web/
├── src/
│   ├── App.tsx              # Dashboard state, guidance, and recovery panels
│   ├── domain.ts            # Typed client projections
│   ├── style.css            # Responsive and focus-visible presentation
│   └── accessibility.test.tsx
└── e2e/
    └── dashboard.spec.ts    # Browser, Axe, and visual state coverage
```

**Structure Decision**: Extend the existing Go API and single React dashboard rather than introduce a new service, store, route editor, or client-side scheduler. The feature-local documents describe the additive API and UI contracts.

## Implementation Approach

1. Add one routing/API-level conversion from an eligible dry-run validation failure into a bounded preview blocker. It uses stable categories and optionally safe stage/profile/provider context; unexpected failures retain a generic safe message. Preserve the existing redaction and `execution: NOT_RUN` semantics.
2. Keep `ProjectReadiness` as the read-only readiness source and add a pure dashboard guidance mapping for its existing statuses. The mapping chooses exactly one next action in prerequisite order and does not claim later checks have passed.
3. Refactor dashboard state around `activeProjectId` and one in-memory task draft. Remove the editable project-path fields from regular preview/start flows; invalidate a prior preview when the selected project or draft changes. An explicit project switch discards the prior draft, while a project that becomes unavailable retains its unsent draft in memory but disables project-specific actions until the developer selects a valid project.
4. Implement the readiness-first composition in the selected app-screen direction: primary scope/readiness/task/action flow, secondary project management/history, flat dark surfaces, and no generic dashboard decoration.
5. Decode failed HTTP responses into typed client error data and render the recovery card from safe fields. Never render a serialized error envelope, blocked-reasons string, or request identifier as the primary error message.
6. Add regression tests across Go API, React/Vitest, and Playwright/Axe. Rebuild dashboard assets before API/browser checks because the Go binary embeds the web build; complete the before/after visual inspection loop at desktop, tablet, and phone widths.
