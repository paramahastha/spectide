# Local HTTP API

The API binds to loopback and rejects other Host/Origin values. Mutation requests require the local `X-SpecTide-CSRF` header. JSON requests have strict size limits and reject unknown fields. Run creation uses an `Idempotency-Key` header; set `dry_run: true` to preview routes without creating a run.

| Method | Path | Behavior |
|---|---|---|
| GET | `/healthz`, `/api/v1/healthz`, `/api/v1/readyz` | Local health response |
| GET | `/api/v1/version` | Version and provider readiness labels |
| GET | `/api/v1/workspace/status` | Browser-safe workspace UUID, schema version, dashboard URL, and local-only flag |
| GET | `/api/v1/capabilities` | Persisted non-secret provider/profile capability observations; does not probe or authenticate |
| GET | `/api/v1/providers/capabilities` | Source-compatible alias for persisted sanitized capability observations; does not probe providers |
| GET | `/api/v1/approvals` | Pending approvals across runs |
| GET | `/api/v1/config/resolved` | Sanitized effective global configuration; no project query is accepted |
| POST | `/api/v1/config/validate` | Validate project routing and capabilities, return a dry-run decision set without creating a run |
| GET | `/api/v1/runs?limit=100&offset=0&project_id=…` | Bounded run-history page (maximum 100), optionally filtered to a registered stable project ID; pagination metadata is in `X-Total-Count`, `X-Page-Limit`, `X-Page-Offset`, and `X-Has-More` headers |
| POST | `/api/v1/runs` | Create a run or use `dry_run: true` for route preview; select exactly one registered `project_id` (legacy `project` remains supported) |
| GET | `/api/v1/runs/{id}` | Run report snapshot |
| GET | `/api/v1/runs/{id}/report` | Run report snapshot |
| GET | `/api/v1/runs/{id}/stages` | Ordered stage-attempt projection |
| GET | `/api/v1/runs/{id}/artifacts` | Registered artifact metadata projection; no file contents |
| GET | `/api/v1/runs/{id}/events?after=N` | Ordered SSE replay and live tail |
| POST | `/api/v1/runs/{id}/pause`, `/resume`, `/cancel` | Safe run controls |
| POST | `/api/v1/runs/{id}/approvals` | Create an approval |
| POST | `/api/v1/runs/{id}/approvals/{approvalId}` | Resolve an approval |
| GET, POST | `/api/v1/profiles` | List or save local profiles |
| PATCH | `/api/v1/profiles/{id}` | Update a profile by stable ID; invalidates prior capability evidence |
| GET, POST | `/api/v1/projects` | List projects with computed readiness or register an absolute canonical local path; POST returns whether it was newly created |
| GET | `/api/v1/projects/{id}/config` | Read effective project configuration |

The run-history endpoint defaults to 100 newest runs and accepts `limit` 1–100 plus a non-negative `offset` up to 1,000,000. Errors use `{ "error": { "code": "…", "message": "…", "details": {}, "request_id": "…" }, "request_id": "…" }`; the body identity matches the `X-Request-ID` response header. Error messages and response payloads are sanitized. The API does not expose arbitrary shell commands, provider credentials, or approved executable paths through capability discovery.

Run report snapshots have explicit `run`, `acceptance`, stable `release_gates` (`QG-01` through `QG-13`), `stages`, `execution_attempts`, `quality_executions`, `routing_decisions`, detailed `gates`, `artifacts`, `checkpoints`, and `approvals` projections. Missing mapped evidence appears as `NOT_RUN`; detailed gates are preserved. Attempts retain route, phase/iteration, process ID, timing, exit/result metadata, safe error text, sanitized evidence, stream hashes, and IDs linking registered artifact hashes in the report; raw provider output is omitted.

The machine-readable contract is [openapi.yaml](openapi.yaml). The feature-level HTTP/SSE contract is also in [the master specification](../specs/001-build-spectide/contracts/http-api.md).

All mutating compatibility requests (including profile `PATCH`) require `X-SpecTide-CSRF` and pass the same Host/Origin checks as existing API routes. A profile update is a complete replacement of its editable fields (`name`, `provider`, `executable`, optional `config_directory`, and non-empty `allowed_scopes`); capability observations are probe-owned and cannot be set through this endpoint. The pending-approval queue is read-only; decisions remain run-scoped through `/api/v1/runs/{id}/approvals/{approvalId}`.
