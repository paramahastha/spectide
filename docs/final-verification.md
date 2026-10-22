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
- Measurement: latest corrected `make verify` run measured 20.369 ms from the monotonic timestamp immediately before `Popen` through the first HTTP 200 from `/healthz` (prior same-process run: 19.318 ms); acceptance limit is 2,000 ms. An earlier attempt measured timestamps in separate Python processes, which use different monotonic origins in this host environment; those earlier values (0.152 ms and 0.295 ms) and the negative result are invalid and superseded.
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
| Spec-Kit analyze consistency gate | PASS, read-only equivalent | `specs/001-build-spectide/analysis.md`; installed Codex CLI executed the repository instructions after OpenCode returned HTTP 503; no official Spec-Kit invocation is claimed |

## Not run or incomplete

- Official Spec-Kit stage execution and convergence: NOT_RUN. The current workflow engine is simulated; T014/T045 remain open.
- Live provider workflow: NOT_RUN. `doctor` found the provider CLIs but reported each as not authenticated; the application also blocks live stage execution before run creation.
- Browser end-to-end and accessibility scans/screenshots: NOT_RUN. The local browser inspection was denied by the browser access gate; T031/T046 remain open.
- SC-006 moderate-load SSE p95 profile: NOT_RUN (T043a).
- Full release acceptance: INCOMPLETE. See [the acceptance ledger](acceptance-ledger.md) and [the implementation task ledger](../specs/001-build-spectide/tasks.md).
