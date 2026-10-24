# Data Model: Dashboard Guidance and Route Recovery

This feature adds transient response and browser view-model data. It does not add a database table, migration, or credential-retention path.

## Existing entity used unchanged: Registered project

| Field | Existing rule | Feature use |
| --- | --- | --- |
| `id` | Stable registered-project identifier | Sole ordinary dashboard selector for preview and workflow start. |
| `canonical_path` | Server-canonicalized local directory | Displayed only as selected-project context; never editable execution authority in routine dashboard flow. |
| `display_name` | Safe human-readable identifier | Primary project label. |
| `readiness.status` | Server-derived condition | Input to the dashboard's next-action mapping. |
| `readiness.reason` / `action` | Sanitized explanation and existing guidance | Supporting explanation, never a reason to bypass the server check. |

## Existing entity used unchanged: Project readiness

| Status | Priority and dashboard action |
| --- | --- |
| `MISSING_CONFIGURATION`, `INVALID_CONFIGURATION`, `MISSING_ROUTE` | First priority; direct the developer to route configuration/review. |
| `MISSING_PROFILE` | Second priority; direct the developer to profile setup. |
| `PROFILE_NOT_AUTHORIZED` | Third priority; direct the developer to scoped-profile review. |
| `CAPABILITY_NOT_VERIFIED` | Fourth priority; direct the developer to profile readiness/probe guidance. |
| `PATH_CHANGED` | Select/restore a valid project; no preview or start. |
| `READY` | Enable preview-before-start and workflow preparation. |

The UI shows one current action only. The ordering is not proof that later checks have passed; a fresh server check remains authoritative at preview/start time.

## New transient API entity: Preview blocker

| Field | Type / allowed values | Validation and privacy rules |
| --- | --- | --- |
| `category` | Stable code: `MISSING_ROUTE`, `MISSING_PROFILE`, `PROFILE_NOT_AUTHORIZED`, `CAPABILITY_NOT_VERIFIED`, `PROJECT_UNAVAILABLE`, `CONFIGURATION_INVALID`, or `UNKNOWN` | Derived server-side from a dry-run failure; unknown/unclassified failures use `UNKNOWN`. |
| `message` | Bounded safe sentence | Redacted; explains the immediate prerequisite without raw internal error text. |
| `remediation.target` | `routing`, `profiles`, `readiness`, or `project-selection` | Bounded navigation target; not a command or arbitrary URL. |
| `remediation.label` | Bounded user-facing action label | Must describe recovery without suggesting credentials are entered into the dashboard. |
| `stage` | Optional known workflow-stage key | Included only when validation safely identifies the configured affected stage. |
| `profile` | Optional configured profile name | Included only for the selected route's named profile; never includes credentials or unrelated profiles. |
| `provider` | Optional configured provider identifier | Included only with a safe profile-specific blocker. |

**Lifecycle**: Created only for the HTTP response to a blocked dry run, consumed by the dashboard, and never persisted as a workflow run, event, task draft, profile secret, or capability observation.

## New transient browser entity: Task draft

| Field | Rule |
| --- | --- |
| `projectId` | Must equal the active registered project ID when used for a preview or start request. |
| `prompt` | Trimmed, non-empty developer input sent only through the existing request path. |
| `previewState` | `idle`, `ready`, `blocked`, or `disconnected`; indicates the state of the draft's latest preview rather than workflow execution. |

**Lifecycle and transitions**:

```text
active project selected + draft entered
  → idle
  → preview requested
  → ready (NOT_RUN) | blocked (NOT_RUN) | disconnected
  → project or draft changes
  → idle
```

The draft survives navigation to corrective dashboard views for the same active project. An explicit change to a different active project discards the prior draft and preview state. If the active project becomes unavailable, its unsent draft remains in page memory, but its preview state and project-specific actions are cleared until the developer selects a valid project. The draft is not stored in local storage or server persistence.

## New transient browser entity: Recovery guidance

| Field | Source | Rule |
| --- | --- | --- |
| `title` | Readiness status or preview blocker category | Plain-language description of the immediate state. |
| `explanation` | Sanitized readiness reason or blocker message | Never the raw serialized HTTP envelope. |
| `nextAction` | Fixed mapping or bounded `remediation.target` | Opens an existing dashboard view; cannot invoke shell, filesystem, or provider operations. |
| `actionLabel` | Fixed mapping or bounded response label | Has an accessible name and remains visible/focusable. |
| `execution` | Preview response | Displays `NOT_RUN` for all preview outcomes. |

## Relationships and invariants

```text
Registered project (persisted, one active in dashboard)
  ├── Project readiness (server-derived projection)
  └── Task draft (browser memory, same active project)
       └── Route preview outcome (transient)
            └── Preview blocker (only when blocked; never a workflow run)
```

- Only the server resolves a project ID to a canonical path and validates route, scope, and capability prerequisites.
- No preview blocker or task draft can broaden project/profile scope or retain credentials.
- A successful or blocked preview must retain `execution: NOT_RUN`; both create zero workflow records.
- A visible recovery action changes dashboard context only. It does not assert that configuration, authorization, or capabilities are now ready.
