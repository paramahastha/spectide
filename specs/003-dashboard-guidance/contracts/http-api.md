# HTTP Contract: Guided Route Preview

This feature preserves the loopback-only HTTP API, Host/Origin/CSRF controls, request-size limits, idempotency requirements, redaction, and existing server-side registered-project resolution. It extends only the blocked dry-run response with an additive, bounded recovery projection.

## `POST /api/v1/runs` — dry route preview

### Request

```json
{
  "project_id": "project-uuid",
  "prompt": "Validate the dashboard guidance flow",
  "dry_run": true
}
```

- Exactly one registered `project_id` is required for ordinary dashboard preview.
- `prompt` is required and follows existing bounded-input validation.
- `Idempotency-Key` and CSRF headers remain required.
- A display name or browser-entered filesystem path is not an execution selector.

### Successful response — `200 OK`

```json
{
  "dry_run": true,
  "valid": true,
  "execution": "NOT_RUN",
  "mode": "hybrid",
  "decisions": [
    {
      "stage": "implement",
      "route": {
        "provider": "codex",
        "profile": "project-codex"
      },
      "explanation": "configured route"
    }
  ]
}
```

The current safe route decision projection remains unchanged. The response confirms only a dry check; it must not claim that a provider ran or repository content was sent.

### Blocked response — `422 Unprocessable Entity`

```json
{
  "error": {
    "code": "ROUTE_PREVIEW_BLOCKED",
    "message": "The selected project's routes need attention before they can be previewed.",
    "details": {},
    "request_id": "support-reference"
  },
  "request_id": "support-reference",
  "dry_run": true,
  "execution": "NOT_RUN",
  "preview_blockers": [
    {
      "category": "MISSING_PROFILE",
      "message": "Profile ‘project-codex’ is needed for the constitution stage.",
      "remediation": {
        "target": "profiles",
        "label": "Review profile setup"
      },
      "stage": "constitution",
      "profile": "project-codex",
      "provider": "codex"
    }
  ]
}
```

Rules:

- `preview_blockers` contains exactly one first actionable blocker for this release. Its array shape permits future compatibility without requiring browser parsing of error strings.
- `category` is one of `MISSING_ROUTE`, `MISSING_PROFILE`, `PROFILE_NOT_AUTHORIZED`, `CAPABILITY_NOT_VERIFIED`, `PROJECT_UNAVAILABLE`, `CONFIGURATION_INVALID`, or `UNKNOWN`.
- `remediation.target` is one of `routing`, `profiles`, `readiness`, or `project-selection`; the dashboard maps it only to an existing local view.
- `stage`, `profile`, and `provider` are optional. Return them only when derived from the selected route and safe after redaction.
- Do not include raw validation strings, stack traces, filesystem details, allowed scopes, credentials, unrelated profile data, or repository content.
- The generic `error` envelope and request ID remain for support compatibility. The dashboard uses `preview_blockers` as primary display content and may make a support reference secondary.

### Invariants

- Both `200` and `422` dry-preview paths create zero runs, stages, events, provider calls, or repository transfers.
- The server revalidates current project configuration, canonical path, profile scope, and capability evidence for each request.
- The command-line validation/preview flow continues to share the underlying routing validation; changing dashboard presentation must not weaken CLI/API parity or authorization checks.

## Existing project list contract

`GET /api/v1/projects` continues returning safe project projections with `readiness.status`, `readiness.reason`, and optional `readiness.action`. The response does not gain a task draft, profile secret, or arbitrary diagnostic data. The dashboard turns these existing statuses into one next action locally, then treats the next dry-run/start response as authoritative.
