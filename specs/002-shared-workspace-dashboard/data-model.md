# Data Model: Shared Workspace Dashboard

## Existing entities changed

### Local workspace metadata

| Field | Type / rules | Purpose |
|---|---|---|
| `workspace_id` | UUID, generated once, immutable, non-secret | Verifies dashboard/server ownership without exposing a storage path. |
| `created_at` | UTC timestamp | Audits initialization. |
| `schema_version` | Existing migration state remains authoritative | Allows compatibility diagnostics only; it is not an authorization control. |

**Validation**: Exactly one metadata record exists after opening a workspace. It is created atomically during migration/initialization and survives reopen, backup, and restore. Browser responses expose only `workspace_id`, version, loopback dashboard URL, and local-only status.

### Registered project

| Field | Type / rules | Purpose |
|---|---|---|
| `id` | Existing stable UUID | Authoritative selection and run association key. |
| `canonical_path` | Unique absolute resolved directory; no symlink escape; never a browser directory listing | Execution and project-boundary identity. |
| `display_name` | Sanitized basename or safe user label; not an execution selector | Human-readable dashboard label. |
| `allowed_scopes` | Existing canonical project scope only | Prevents registration from broadening provider access. |
| `created_at` | Existing UTC timestamp | Audit and ordering metadata. |
| `readiness` | Computed response projection, never the authorization source | Configuration and provider readiness summary. |

**Registration rules**:
- CLI may resolve a relative path; dashboard registration accepts an absolute local path only.
- Resolve symlinks, require a current directory, reject secret-like values and path drift.
- Repeated registration of the same canonical path returns the existing project without copying or broadening configuration, scopes, profile permissions, or secrets.
- Display names never select a project for execution.

### Project readiness (computed projection)

| Field | Values / rules | Purpose |
|---|---|---|
| `state` | `READY`, `MISSING_CONFIGURATION`, `INVALID_CONFIGURATION`, `MISSING_ROUTE`, `MISSING_PROFILE`, `PROFILE_NOT_AUTHORIZED`, `CAPABILITY_NOT_VERIFIED`, `PATH_CHANGED` | Machine-readable condition for UI and CLI. |
| `action` | A least-privilege remediation code/short command guidance | Enables one direct next step. |
| `message` | Sanitized, bounded user-facing explanation | Explains why a run cannot start. |

**Evaluation rules**: Re-resolve the stored canonical path; load only the safe project configuration path; then validate route, profile scope, provider match, and verified capability. Listing/readiness evaluation must not launch providers, browse project contents, or create a run.

### Workflow run

| Field | Type / rules | Purpose |
|---|---|---|
| `id` | Existing UUID | Run identity. |
| `project_id` | Nullable foreign key to `projects.id`; required for all newly created runs | Stable project scope and filtered history. |
| `project` | Existing canonical-path string, retained | Backward-compatible historical/report projection. |
| remaining run fields | Existing immutable/evidence fields | Existing workflow lifecycle. |

**Migration and transition rules**:
1. Add nullable `project_id` rather than replacing `project`.
2. Backfill only when a historical `project` exactly matches one registered canonical path; leave other rows null and visible in all-project history.
3. New workflow start resolves a registered project, rechecks readiness and writer protection, then writes both values atomically.
4. `project_id` filtering applies before ordering, pagination, count, and response construction.

### Workspace import record

| Field | Type / rules | Purpose |
|---|---|---|
| `id` | UUID | Import identity. |
| `source_label` | Sanitized non-secret source descriptor | Auditable source reference without browser path disclosure. |
| `source_workspace_id` | Optional UUID when available | Source identity. |
| `destination_workspace_id` | UUID | Result identity. |
| `backup_reference` | Private local backup reference; omitted from browser APIs | Recovery evidence. |
| `confirmed_at`, `completed_at` | UTC timestamps | Confirmation and completion audit. |
| `result` | `COMPLETED`, `REJECTED`, `FAILED` with sanitized reason | Truthful outcome. |

**State transitions**: `DETECTED` → `AWAITING_CONFIRMATION` → `BACKED_UP` → `RESTORED` → `VALIDATED` → `COMPLETED`. A refusal, ambiguity, non-empty destination, integrity failure, or schema failure ends in `REJECTED`/`FAILED` without overwriting or deleting source state.

### Dashboard runtime lease (local runtime artifact, not browser data)

| Field | Rules | Purpose |
|---|---|---|
| workspace identity | Must match metadata UUID | Prevents a lock from being reused for another workspace. |
| endpoint | Default loopback address only | Coordinates the normal dashboard. |
| process ID and start marker | Treated as advisory; stale records are recoverable | Avoids launch races. |
| lease/lock ownership | Private filesystem permissions; never returned to the browser | Coordinates managed process lifecycle. |

**Validation**: A lease never proves server ownership by itself. The dashboard manager must confirm a live workspace-status response with the expected UUID before reuse or opening.

## Relationships

```text
Local workspace (1) ──< Registered project (many)
Registered project (1) ──< Workflow run (many; legacy runs may be unassigned)
Local workspace (1) ──< Workspace import record (many)
Local workspace (1) ──1 Dashboard runtime lease (at most one active normal lease)
```

## Invariants

- A new run belongs to exactly one registered canonical project; an old unresolved run remains readable only in the all-projects view.
- Project configuration remains repository-local; workspace metadata never absorbs routes, profiles, or credentials.
- Different project IDs can hold workflow writer locks concurrently; the same canonical project cannot.
- Any project path drift, scope failure, or readiness failure blocks run creation before durable workflow state is created.
- Workspace IDs, project IDs, and sanitized readiness are safe to return locally; storage paths, lease paths, credentials, provider configuration, and repository contents are not.
