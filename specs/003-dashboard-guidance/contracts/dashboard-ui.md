# Dashboard UI Contract: Readiness-First Workflow

## Workflow control information order

1. **Project scope** — identify the selected project by name with its canonical path as supporting context. All-project scope is a history view and asks the developer to select a project before preview or start.
2. **Readiness path** — show the current server-observed status, its plain-language consequence, and one next action. It represents route, profile access, capability evidence, dry preview, and start in order; only the current unmet step is actionable.
3. **Task draft** — show one editable prompt for the active project. Do not render duplicate prompt fields on Route preview and Workflow control.
4. **Preview and start** — ready projects can preview the shared draft before starting. Non-ready projects show the guidance action instead of an apparently ready start action.
5. **Secondary work** — project switching/adding, history, profile details, and the offline demo remain reachable but do not compete with the primary next action.

## State matrix

| State | Primary message | Primary action | Required evidence |
| --- | --- | --- | --- |
| All projects selected | Choose a project to prepare or run. | Select project | No preview/start request is sent. |
| Missing configuration or route | Explain that routes must be configured before work can be checked or started. | Review route settings | Preview/start unavailable for this context. |
| Missing profile | Explain that the configured profile must be registered. | Review profile setup | No claim that a provider or run started. |
| Unauthorized profile | Explain that the profile is not permitted for this project. | Review profile access | Do not show raw allowed scopes. |
| Unverified capability | Explain that provider readiness needs verification. | Review profile readiness | Do not imply authentication/capability is established. |
| Ready | Explain that routes and stored provider evidence are ready for a dry check. | Preview routes | Preview success remains `NOT_RUN`. |
| Preview blocked | Name the safe immediate cause and available stage/profile context. | Use blocker remediation action | Visible `NOT_RUN`; no raw envelope/request ID as main message. |
| Preview ready | List resolved stage routes. | Start workflow | Visible `NOT_RUN`; starting remains a separate explicit action. |
| Local API disconnected | Explain that current readiness cannot be confirmed. | Retry after reconnecting | Do not recommend route/profile changes based on stale state. |

## Interaction rules

- Changing active project or task draft resets preview state to `idle` and prevents late results from replacing the current intent.
- Following a remediation action preserves the active project and in-memory draft. Returning to preview does not require re-entering either.
- Changing to a different active project clears the prior task draft and preview state safely. If the active project is lost or unavailable, retain the unsent draft in page memory but clear its preview state and disable project-specific actions until the developer selects a valid project.
- The offline demonstration is labeled simulated and visually/logically separate from a selected project's live readiness.
- Every corrective action is a keyboard-operable button or link with a visible focus indicator, descriptive accessible name, and text that remains understandable without color.
- The dark composition uses lightness for surface elevation and reserves the accent for status/data; it must not introduce gradients, glows, colored nav, or decorative icon tiles.
- Narrow-screen layouts preserve the above order without horizontal loss or hidden required actions.
