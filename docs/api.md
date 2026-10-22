# Local HTTP API

The API binds to loopback and rejects other Host/Origin values. Mutation requests require the local `X-SpecTide-CSRF` header. JSON requests have strict size limits and reject unknown fields. Run creation uses an `Idempotency-Key` header; set `dry_run: true` to preview routes without creating a run.

| Method | Path | Behavior |
|---|---|---|
| GET | `/healthz`, `/api/v1/healthz`, `/api/v1/readyz` | Local health response |
| GET | `/api/v1/version` | Version and provider readiness labels |
| GET, POST | `/api/v1/runs` | List runs or create a run/route preview |
| GET | `/api/v1/runs/{id}` | Run report snapshot |
| GET | `/api/v1/runs/{id}/report` | Run report snapshot |
| GET | `/api/v1/runs/{id}/events?after=N` | Ordered SSE replay and live tail |
| POST | `/api/v1/runs/{id}/pause`, `/resume`, `/cancel` | Safe run controls |
| POST | `/api/v1/runs/{id}/approvals` | Create an approval |
| POST | `/api/v1/runs/{id}/approvals/{approvalId}` | Resolve an approval |
| GET, POST | `/api/v1/profiles` | List or save local profiles |
| GET, POST | `/api/v1/projects` | List or register canonical local projects |
| GET | `/api/v1/projects/{id}/config` | Read effective project configuration |

Errors use a JSON object with an `error` member containing `code` and `message`. Responses and event payloads redact common credential forms. The API does not expose arbitrary shell commands or provider credentials.

The machine-readable contract is [openapi.yaml](openapi.yaml). The feature-level HTTP/SSE contract is also in [the master specification](../specs/001-build-spectide/contracts/http-api.md).
