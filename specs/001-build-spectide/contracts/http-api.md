# Local HTTP and Event Contract

The server accepts only loopback traffic. Mutating calls require the same-origin anti-CSRF header and do not accept arbitrary shell, environment, or filesystem input.

| Method | Route | Success |
| --- | --- | --- |
| GET | `/api/v1/healthz`, `/readyz`, `/version` | Local service status/capabilities |
| GET/POST | `/api/v1/runs` | Paginated durable runs / validated run creation or dry-run |
| GET | `/api/v1/runs/{id}` | Snapshot with stage attempts and quality gates |
| GET | `/api/v1/runs/{id}/events?after=N` | SSE replay plus live tail; event ID equals run sequence |
| POST | `/api/v1/runs/{id}/pause`, `/resume`, `/cancel` | Safe lifecycle request |
| GET | `/api/v1/runs/{id}/report` | Sanitized JSON or Markdown evidence report |

Every API error uses `{ "code": "…", "message": "…", "details": {}, "request_id": "…" }` and never includes credentials.
