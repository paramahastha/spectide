# HTTP Contract: Shared Workspace Dashboard

All endpoints remain loopback-only and use the existing Host, Origin, CSRF, strict JSON, redaction, and bounded-request protections. Browser clients must use stable project IDs; display names and raw paths are never authorization selectors.

## `GET /api/v1/workspace/status`

Returns only safe local identity data used by the dashboard command and UI.

```json
{
  "workspace_id": "b3c9…",
  "version": "0.1.0-dev",
  "dashboard_url": "http://127.0.0.1:4747",
  "local_only": true
}
```

It must not return the database path, runtime lease/PID location, provider configuration location, credentials, or repository contents.

## `GET /api/v1/projects`

Preserves the collection route and returns registered projects with a server-derived readiness projection.

```json
[
  {
    "id": "project-uuid",
    "display_name": "spectide",
    "canonical_path": "/Users/example/Projects/spectide",
    "allowed_scopes": ["/Users/example/Projects/spectide"],
    "readiness": {
      "state": "READY",
      "action": "START_WORKFLOW",
      "message": "Project is ready for a configured workflow."
    }
  }
]
```

- Evaluation is read-only: it does not start providers, create workflows, or inspect repository content beyond safe path/config validation.
- Results are sanitized and bounded.

## `POST /api/v1/projects`

Registers one deliberately chosen local directory.

```json
{ "path": "/absolute/local/project" }
```

- `path` is required, absolute, canonicalized, and must resolve to an existing directory.
- The server derives a safe display name. A legacy optional name field may be accepted only for compatibility and cannot select execution or broaden scopes.
- The response is a project/readiness projection. Use `201` when created and `200` when the canonical project already exists.
- A relative path, file, unavailable path, symlink boundary failure, or invalid request returns a sanitized client error. No directory browsing or repository-content transfer occurs.

## `GET /api/v1/runs?limit=&offset=&project_id=`

- Retains the current unfiltered behavior when `project_id` is absent.
- Optional `project_id` selects exactly one registered project; unknown/malformed IDs are rejected.
- The filter is applied before ordering, page limits, offsets, counts, and `X-Has-More` calculation.
- For a selected project, this bounded newest-first run list is the dashboard's recent-activity source; detailed events remain available only for a selected run in that same scope.
- Existing maximum `limit` of 100 and maximum `offset` of 1,000,000 remain.
- Each new-run summary includes `project_id` in addition to the existing canonical `project` field. Historical runs may have `project_id: null` and remain visible in the all-projects view.

## `POST /api/v1/runs`

Normal dashboard requests use a stable project ID:

```json
{ "project_id": "project-uuid", "prompt": "Add an audit log", "dry_run": false }
```

- Existing idempotency and CSRF headers remain mandatory.
- A dry run uses the same registered-project resolution and returns `execution: "NOT_RUN"` without creating a workflow.
- Before either preview or start, the server resolves the stored canonical path, rechecks path drift, configuration, profile scope, capability evidence, and writer availability.
- The compatibility `project` path form remains supported only when it resolves to a registered project; it must never auto-register a directory.
- A request must specify one project selector, not both. Display name is not accepted.

## Existing project configuration route

`GET /api/v1/projects/{id}/config` remains project-ID scoped and must revalidate path/config boundaries before returning only the sanitized configuration projection.

## Error contract

Continue using the existing sanitized error envelope and request ID. New stable codes include:

- `PROJECT_NOT_REGISTERED`
- `PROJECT_PATH_CHANGED`
- `PROJECT_NOT_READY`
- `PROJECT_ID_INVALID`
- `WORKSPACE_ID_MISMATCH`
- `DASHBOARD_ENDPOINT_UNVERIFIED`
- `LEGACY_IMPORT_CONFIRMATION_REQUIRED`
- `LEGACY_IMPORT_AMBIGUOUS`

No error may expose secrets, credentials, lease locations, raw database locations to browser clients, or repository content.
