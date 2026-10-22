# Local HTTP API

The API binds to loopback and rejects other Host/Origin values. Mutation requests require the local `X-SpecTide-CSRF` header. JSON requests have strict size limits and reject unknown fields. Run creation uses an `Idempotency-Key` header; set `dry_run: true` to preview routes without creating a run.

| Method | Path | Behavior |
|---|---|---|
| GET | `/healthz`, `/api/v1/healthz`, `/api/v1/readyz` | Local health response |
| GET | `/api/v1/version` | Version and provider readiness labels |
| GET | `/api/v1/capabilities` | Persisted non-secret provider/profile capability observations; does not probe or authenticate |
| POST | `/api/v1/config/validate` | Validate project routing and capabilities, return a dry-run decision set without creating a run |
| GET | `/api/v1/runs?limit=100&offset=0` | Bounded run-history page (maximum 100); pagination metadata is in `X-Total-Count`, `X-Page-Limit`, `X-Page-Offset`, and `X-Has-More` headers |
| POST | `/api/v1/runs` | Create a run or use `dry_run: true` for route preview |
| GET | `/api/v1/runs/{id}` | Run report snapshot |
| GET | `/api/v1/runs/{id}/report` | Run report snapshot |
| GET | `/api/v1/runs/{id}/stages` | Ordered stage-attempt projection |
| GET | `/api/v1/runs/{id}/artifacts` | Registered artifact metadata projection; no file contents |
| GET | `/api/v1/runs/{id}/events?after=N` | Ordered SSE replay and live tail |
| POST | `/api/v1/runs/{id}/pause`, `/resume`, `/cancel` | Safe run controls |
| POST | `/api/v1/runs/{id}/approvals` | Create an approval |
| POST | `/api/v1/runs/{id}/approvals/{approvalId}` | Resolve an approval |
| GET, POST | `/api/v1/profiles` | List or save local profiles |
| GET, POST | `/api/v1/projects` | List or register canonical local projects |
| GET | `/api/v1/projects/{id}/config` | Read effective project configuration |

The run-history endpoint defaults to 100 newest runs and accepts `limit` 1–100 plus a non-negative `offset` up to 1,000,000. Errors use `{ "error": { "code": "…", "message": "…", "details": {}, "request_id": "…" }, "request_id": "…" }`; the body identity matches the `X-Request-ID` response header. Error messages and response payloads are sanitized. The API does not expose arbitrary shell commands, provider credentials, or approved executable paths through capability discovery.

Run report snapshots have explicit `run`, `acceptance`, `stages`, `execution_attempts`, `quality_executions`, `routing_decisions`, `gates`, `artifacts`, `checkpoints`, and `approvals` projections. Attempts retain route, phase/iteration, process ID, timing, exit/result metadata, safe error text, sanitized evidence, stream hashes, and IDs linking registered artifact hashes in the report; raw provider output is omitted.

The machine-readable contract is [openapi.yaml](openapi.yaml). The feature-level HTTP/SSE contract is also in [the master specification](../specs/001-build-spectide/contracts/http-api.md).
