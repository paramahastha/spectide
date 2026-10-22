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
- Measurement: latest `make verify` run measured 21.319 ms from the monotonic timestamp immediately before `Popen` through the first HTTP 200 from `/healthz`; acceptance limit is 2,000 ms. Earlier measurements are retained in the implementation checkpoint history.
- The same smoke run completed the offline demo and reported `SIMULATED`.

## Other verification executed

| Check | Result | Evidence |
|---|---|---|
| Go unit/integration suite | PASS | `go test ./...` |
| Go static analysis | PASS | `go vet ./...` |
| Dashboard tests | PASS | `make web-test` (27 tests) |
| Dashboard typecheck | PASS | `make lint` |
| Dashboard production build | PASS | `make web-build` |
| Dashboard component states and Axe scan | PASS, jsdom scope | `cd web && npm test` (27 tests; seven surfaces; zero critical/serious); see [accessibility audit](accessibility-audit.md) |
| Offline demo and startup smoke | PASS | `make smoke` |
| Unified verification and secret scan | PASS | `make verify` (format, vet, Go tests, dashboard checks/build, binary build, secret scan, and smoke) |
| Frozen effective run configuration | PASS | `make verify`; immutable run configuration stores effective routes, profile executable/config/scope and capability references, required-check definitions, bounded loop settings, and stage timeouts. Tests cover defaults, overrides, supported ranges, profile drift, adapter timeout propagation, and restoration from the hashed run snapshot (T064). |
| Credential-safe provider status and capability probes | PASS, partial capability coverage | Focused adapter/CLI/store/routing/workflow tests and `make verify`; supported CLI auth status is parsed without persisting credentials, exact available model/effort identifiers are checked, unavailable capabilities remain blocked, and immutable run snapshots include the observed capability values (T065). Live account execution remains `NOT_RUN`. |
| Evidence-based live/mixed final acceptance | PASS, fixture-backed implementation | `make verify`; mixed fixture workflow reaches COMPLETED only with PASS analyze/local-test/convergence/final-review evidence. Reports and `run.acceptance` events distinguish LIVE/MIXED/SIMULATED; simulation-only E2E is NOT_RUN for live/mixed runs and missing gates remain BLOCKED (T066). |
| Bounded provider event streaming and terminal extraction | PASS, fixture-backed implementation | `make verify`; runner streams within its 1 MiB output cap, Spec-Kit consumes provider-specific extracted terminal text, and normalized events retain no payload body. Partial frames, malformed/version/error records, truncation, and oversized frames have fixtures (T067). |
| Project-aware local quality checks | PASS, fixture-backed implementation | `make verify`; run configuration freezes discovered checks, Go and Node layouts are recognized without SpecTide path assumptions, Python markers select pytest, and missing commands/scripts or unsupported layouts persist BLOCKED evidence without repair attempts (T068). |
| Durable sensitive-action approval | PASS, fixture-backed implementation | `make verify`; live implementation pauses before workspace writes, run-bound approvals resume only after approval, denial blocks and skips implementation, cross-run and duplicate decisions fail, persisted approvals survive store reopen, and paused pending decisions have accessible dashboard controls (T069). |
| Inner-loop pause and local-check cancellation | PASS, fixture-backed implementation | `make verify`; provider/check evidence persists before pause, checkpoints retain the active stage and loop counters, the analyze loop resumes after pause, and a second SQLite controller cancels a long-running registered check within the bounded process cleanup interval (T070). |
| Foreground CLI restart resume | PASS, subprocess fixture | `go test ./internal/workflow -run TestCLISubprocessResumeWaitsForDurableTerminalEvidence -count=1`; the built `spectide resume` process remains alive through recovery claim and simulated completion, then the reopened SQLite report contains terminal acceptance and `run.COMPLETED` evidence (T071). |
| Runner timeout/output/cancellation | PASS | `go test ./internal/runner -count=1` |
| Deterministic repeated lifecycle | PASS | `TestTwentyDeterministicRunsCompleteWithRequiredEvidence` (20 runs) |
| Workspace path/scope enforcement | PASS | Focused `go test ./internal/workflow ./internal/api ./internal/workspace`; missing paths and disallowed roots rejected before run creation, canonical symlink path accepted within scope |
| Process policy and event/checkpoint sanitization | PASS | Focused `go test ./internal/security ./internal/events ./internal/runner ./internal/store`; runner validates process policy before launch and payload sanitization covers nested map/slice credential variants |
| Persisted stage cursors and verified restart recovery | PASS | `go test ./internal/workflow ./internal/store -count=1` and `make verify`; restart resume validates canonical identity, lock ownership, immutable config/checkpoint/cursor, artifact hashes, Git HEAD/index, managed-path hashes, and loop counters. Drift is blocked without overwriting workspace evidence; see [recovery guide](recovery.md). |
| Cross-layer recovery, secret-leak, and bounded-loop acceptance | PASS | Focused workflow/API/store tests and `make verify`; API checks cover stale resume blocking, prompt redaction through create response, SQLite, report, and SSE replay, and idempotency distinguishes raw secret-bearing requests. Workflow checks assert exact 3/4/5 exhaustion and no-progress termination; restart recovery preserves evidence on drift. |
| Bounded workflow convergence and final review | PASS, fixture-backed implementation | `make verify`; live fixture routes execute bounded append-only convergence, implement only appended tasks, rerun tests, and return final-review findings through fix/test/converge under a shared per-run budget. Official provider authentication/convergence remains T045. |
| Scoped route preview | PASS | Full `make verify`; API preview rejects workspace scope escapes without run creation |
| Live route profile boundaries | PASS | Focused routing/workflow/API tests and full `make verify`; profile existence, provider match, and canonical allowed scope are required before preview or run creation; live invocation remains blocked |
| Persisted capability and fallback authorization policy | PASS, partial requirement | Focused store/routing/workflow/API/CLI tests and full `make verify`; supported status probes persist safe auth/capability observations, stale snapshots invalidate after profile edits, explicit fallback intent stays within project boundaries, and preview/start fail closed on unavailable evidence. T024/T065 are complete; fallback execution is not represented as verified and remains subject to explicit route policy. |
| Live Spec-Kit artifact lifecycle | PASS, fixture-backed implementation | `make verify`; fresh/partial feature-directory selection, existing constitution/spec preservation, missing plan creation, path containment, and selected-directory checkpoint persistence are covered by `internal/speckit` and `internal/workflow` tests (T063). |
| Verified live-stage workflow path | PASS, fixture-backed implementation | `make verify`; engine fixture invoked verified plan, analyze, converge, and final-review routes through provider adapters, persisted sanitized events and loop counts, validated capabilities at invocation, and persisted `analyze_clean`, `required_tests`, `converged`, and `final_review` PASS gates. Simulation-only E2E is reported NOT_RUN for the mixed fixture; no real provider credentials or official live stage execution were used. T014/T016/T035/T066 implementation is complete. |
| Persisted live test gate | PASS, fixture-backed implementation | Focused workflow loop tests and `make verify`; default local checks persist individual verdicts and aggregate `required_tests`, with failed-check-only repair prompts and fail-closed no-progress/exhaustion handling. T016 is complete. |
| Durable blocked live-stage outcome | PASS | Focused workflow/CLI/API/Spec-Kit tests and full `make verify`; authorized but unavailable live routes persist a `BLOCKED` run, skipped first stage, `NOT_RUN` event, and blocked gate |
| Local documentation contract | PASS | Full `make verify`; `ruby -e 'require "yaml"; YAML.load_file("docs/openapi.yaml")'`; CLI examples checked against implemented command flags |
| SC-006 SSE moderate-load profile | PASS | `make load-test-sse`: 10 clients, 100 events/s × 100 s, 10,000 persisted backlog events; 200,010 total non-heartbeat frames received with contiguous sequences; all-frame p95 270.133 ms |
| Spec-Kit analyze consistency gate | PASS, read-only equivalent | `specs/001-build-spectide/analysis.md`; installed Codex CLI executed the repository instructions after OpenCode returned HTTP 503; no official analyze result is claimed |
| Official Spec-Kit converge | TASKS_APPENDED; not converged | `specify` CLI 1.2.2.dev0 invoked the installed `speckit.converge` through OpenCode on 2026-10-22. It checked 30 FRs, 8 buildable SCs, 14 acceptance scenarios, 56 existing tasks, 9 plan areas, and 5 constitution principles; appended T055–T079 (9 critical, 14 high, 2 medium). A hash/byte-length assertion verified that the prior 13,716 bytes of `tasks.md` were unchanged. Fresh `go test -count=1 ./...`, `go vet ./...`, and dashboard tests passed. T045 remains open until appended work is implemented and convergence reruns clean. |

## Not run or incomplete

- Authenticated provider execution and real-provider convergence: NOT_RUN. The official Spec-Kit converge command ran and appended T055–T079, but the project is not yet converged. The verified workflow path is fixture-backed; `doctor` reports provider CLIs as not authenticated. T045 remains open.
- Mixed live workflow final acceptance: BLOCKED at `simulated_e2e`; T016/T034/T035 implementation and T036 fixture-backed failure/recovery/secret-leak/exhaustion scenarios pass, but no authenticated provider run is available.
- Browser E2E, actual 200% zoom, rendered keyboard/focus/contrast and reduced-motion checks, and screenshots: NOT_RUN. No Chromium/Chrome/Edge executable is installed and the prior localhost browser inspection was denied by the browser access gate; T031/T046 remain open. Component-level Axe and stylesheet contrast checks are recorded separately.
- Full release acceptance: INCOMPLETE. T031 browser evidence, T045 official convergence, and T046 screenshots/release report remain open. See [the acceptance ledger](acceptance-ledger.md) and [the implementation task ledger](../specs/001-build-spectide/tasks.md).
