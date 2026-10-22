# Local HTTP and Event Contract

The server accepts only loopback traffic. Mutating calls require the same-origin anti-CSRF header and do not accept arbitrary shell, environment, or filesystem input.

| Method | Route | Success |
| --- | --- | --- |
| GET | `/api/v1/healthz`, `/readyz`, `/version`, `/capabilities` | Local status and persisted non-secret provider capability observations |
| POST | `/api/v1/config/validate` | Validate project routing/capability configuration with no run creation |
| GET | `/api/v1/runs?limit=100&offset=0` | Newest-first bounded run page; max 100 and explicit pagination headers |
| POST | `/api/v1/runs` | Validated run creation or `dry_run` route preview |
| GET | `/api/v1/runs/{id}` | Snapshot with stage attempts and quality gates |
| GET | `/api/v1/runs/{id}/stages`, `/artifacts` | Explicit ordered stage and safe artifact metadata projections |
| GET | `/api/v1/runs/{id}/report` | Full report projection with acceptance, route, evidence, artifact, and approval fields |
| GET | `/api/v1/runs/{id}/events?after=N` | SSE replay plus live tail; `Last-Event-ID` takes precedence over `after`, event ID equals run sequence |
| POST | `/api/v1/runs/{id}/pause`, `/resume`, `/cancel` | Safe lifecycle request |
| GET | `/api/v1/runs/{id}/report` | Sanitized JSON or Markdown evidence report |

Every API error uses `{ "error": { "code": "…", "message": "…", "details": {}, "request_id": "…" }, "request_id": "…" }`. Both request IDs match the `X-Request-ID` response header. Messages and details are sanitized and never include credentials.
