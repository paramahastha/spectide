# SpecTide verification evidence

**Status: IN PROGRESS — not a release-readiness approval.** This report records measured criteria and commands as implementation proceeds. Open or unrun acceptance criteria remain explicit; T046 owns the final evidence index and release-readiness summary.

## SC-001 — local build and startup health

**Result: PASS on the recorded host.**

- Date: 2026-10-22
- OS: macOS 15.1
- CPU: Apple M4
- Installed RAM: 16.0 GiB
- Architecture: arm64
- Build command: `make build` (`go build -o bin/spectide ./cmd/spectide`)
- Startup/health command: `make smoke` (`scripts/smoke.sh` starts `spectide serve --port 0`, then requests `/healthz`)
- Measurement: latest `make verify` run measured 20.021 ms from the monotonic timestamp immediately before `Popen` through the first HTTP 200 from `/healthz`; acceptance limit is 2,000 ms. Earlier measurements are retained in the implementation checkpoint history.
- The same smoke run completed the offline demo and reported `SIMULATED`.

## Other verification executed

| Check | Result | Evidence |
|---|---|---|
| Go unit/integration suite | PASS | `go test ./...` |
| Go static analysis | PASS | `go vet ./...` |
| Dashboard tests | PASS | `make web-test` (2 tests) |
| Dashboard typecheck | PASS | `make lint` |
| Dashboard production build | PASS | `make web-build` |
| Offline demo and startup smoke | PASS | `make smoke` |
| Unified verification and secret scan | PASS | `make verify` (format, vet, Go tests, dashboard checks/build, binary build, secret scan, and smoke) |
| Runner timeout/output/cancellation | PASS | `go test ./internal/runner -count=1` |
| Deterministic repeated lifecycle | PASS | `TestTwentyDeterministicRunsCompleteWithRequiredEvidence` (20 runs) |
| Workspace path/scope enforcement | PASS | Focused `go test ./internal/workflow ./internal/api ./internal/workspace`; missing paths and disallowed roots rejected before run creation, canonical symlink path accepted within scope |
| Process policy and event/checkpoint sanitization | PASS | Focused `go test ./internal/security ./internal/events ./internal/runner ./internal/store`; runner validates process policy before launch and payload sanitization covers nested map/slice credential variants |
| Persisted stage cursors | PASS | Focused `go test ./internal/workflow ./internal/store`; completed simulated runs retain one config-bound cursor checkpoint per canonical stage; checkpoints do not authorize restart resume |
| Bounded workflow convergence foundation | PASS, partial requirement | `go test ./internal/workflow -run 'RunConvergence|LoopBudget' -count=1`; appended tasks are implemented and tested under a five-round bound; final-review return and official provider stage wiring remain incomplete |
| Scoped route preview | PASS | Full `make verify`; API preview rejects workspace scope escapes without run creation |
| Live route profile boundaries | PASS | Focused routing/workflow/API tests and full `make verify`; profile existence, provider match, and canonical allowed scope are required before preview or run creation; live invocation remains blocked |
| Persisted capability and fallback authorization policy | PASS, partial requirement | Focused store/routing/workflow/API/CLI tests and full `make verify`; version-only probe evidence persists with authentication `NOT_PROBED`, stale snapshots invalidate after profile edits, explicit fallback intent stays within project boundaries, and preview/start fail closed. T024 is complete; automatic fallback retries remain open with workflow-loop integration (T016). |
| Verified live-stage workflow path | PASS, fixture-backed implementation | `go test ./internal/workflow ./internal/speckit ./internal/store`; engine fixture invoked a verified command template through a provider adapter, persisted sanitized events, validated capability again at invocation, ran registered local checks, and persisted `required_tests=PASS` while leaving incomplete live quality gates blocked. No real provider credentials or official live stage execution were used. T014 implementation is complete; T016 remains open for analyze and convergence integration. |
| Persisted live test gate | PASS, fixture-backed implementation | Focused workflow loop tests and `make verify`; default local checks persist individual verdicts and aggregate `required_tests`, with failed-check-only repair prompts and fail-closed no-progress/exhaustion handling. T016 remains open for live analyze remediation and convergence/final-review integration. |
| Durable blocked live-stage outcome | PASS | Focused workflow/CLI/API/Spec-Kit tests and full `make verify`; authorized but unavailable live routes persist a `BLOCKED` run, skipped first stage, `NOT_RUN` event, and blocked gate |
| Local documentation contract | PASS | Full `make verify`; `ruby -e 'require "yaml"; YAML.load_file("docs/openapi.yaml")'`; CLI examples checked against implemented command flags |
| SC-006 SSE moderate-load profile | PASS | `make load-test-sse`: 10 clients, 100 events/s × 100 s, 10,000 persisted backlog events; 200,010 total non-heartbeat frames received with contiguous sequences; all-frame p95 270.133 ms |
| Spec-Kit analyze consistency gate | PASS, read-only equivalent | `specs/001-build-spectide/analysis.md`; installed Codex CLI executed the repository instructions after OpenCode returned HTTP 503; no official Spec-Kit invocation is claimed |

## Not run or incomplete

- Official Spec-Kit/provider execution and convergence against a real authenticated account: NOT_RUN. The verified workflow path is covered by a fixture adapter; `doctor` reports the installed provider CLIs as not authenticated. T014 implementation is complete; T045 remains open.
- Live workflow final quality acceptance: BLOCKED until run-level analyze remediation, test repair, and convergence integration is complete (T016, T035).
- Browser end-to-end and accessibility scans/screenshots: NOT_RUN. The local browser inspection was denied by the browser access gate; T031/T046 remain open.
- Full release acceptance: INCOMPLETE. See [the acceptance ledger](acceptance-ledger.md) and [the implementation task ledger](../specs/001-build-spectide/tasks.md).
