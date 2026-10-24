# Local HTTP and Event Contract Summary

The authoritative, machine-readable API contract is [`docs/openapi.yaml`](../../../docs/openapi.yaml) (OpenAPI 3.1). This feature-level file is a concise scope summary; it does not replace the OpenAPI contract or add endpoint requirements beyond it.

The server accepts only loopback traffic. Mutating calls require the same-origin anti-CSRF header and do not accept arbitrary shell, environment, or filesystem input.

| Method | Route | Success |
| --- | --- | --- |
| GET | `/api/v1/healthz`, `/readyz`, `/version`, `/capabilities` | Local status and persisted non-secret provider capability observations |
| POST | `/api/v1/config/validate` | Validate project routing/capability configuration with no run creation |
| GET, POST | `/api/v1/projects` | List or register validated canonical local projects |
| GET | `/api/v1/projects/{id}/config` | Read the effective non-secret configuration for one project |
| GET | `/api/v1/runs?limit=100&offset=0` | Newest-first bounded run page; max 100 and explicit pagination headers |
| POST | `/api/v1/runs` | Validated run creation or `dry_run` route preview |
| GET | `/api/v1/runs/{id}` | Snapshot with stage attempts and quality gates |
| GET | `/api/v1/runs/{id}/stages`, `/artifacts` | Explicit ordered stage and safe artifact metadata projections |
| GET | `/api/v1/runs/{id}/report` | Full report projection with acceptance, route, evidence, artifact, and approval fields |
| GET | `/api/v1/runs/{id}/events?after=N` | SSE replay plus live tail; `Last-Event-ID` takes precedence over `after`, event ID equals run sequence |
| POST | `/api/v1/runs/{id}/{action}` | Safe lifecycle request where `action` is `pause`, `resume`, or `cancel` |
| POST | `/api/v1/runs/{id}/approvals`, `/api/v1/runs/{id}/approvals/{approvalId}` | Create or resolve a run-scoped approval |
| GET, POST | `/api/v1/profiles` | List or save sanitized local profile records |
| PATCH | `/api/v1/profiles/{id}` | Replace editable profile fields; invalidates probe-derived capabilities and requires CSRF |
| GET | `/api/v1/approvals` | List pending approvals across runs; decisions remain run-scoped |
| GET | `/api/v1/providers/capabilities` | Source-compatible alias for persisted sanitized capability observations |
| GET | `/api/v1/config/resolved` | Sanitized effective global configuration; accepts no project query parameter |

Every API error uses `{ "error": { "code": "…", "message": "…", "details": {}, "request_id": "…" }, "request_id": "…" }`. Both request IDs match the `X-Request-ID` response header. Messages and details are sanitized and never include credentials.

The source-compatible paths above preserve existing routes and use the same loopback, request validation, redaction, and mutation protection policies. The authoritative machine-readable contract is [`docs/openapi.yaml`](../../../docs/openapi.yaml).
