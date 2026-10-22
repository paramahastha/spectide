# SpecTide acceptance ledger

This ledger maps every functional requirement in the single master specification to current evidence. `Partial` means some behavior is implemented, but the complete requirement is not accepted. `Open` means required behavior or acceptance evidence is missing. The checked-in demo is simulated and never counts as a live-provider pass.

| Requirement | Status | Evidence |
|---|---|---|
| FR-001 CLI commands | Partial | [CLI command tree](../internal/cli/root.go); project registration, profile/model offline probes, approval lifecycle, and stable completed/blocked/failed/cancelled run exit codes are implemented; model catalogs remain explicitly `NOT_PROBED` until provider-specific authorized discovery is available |
| FR-002 durable local evidence | Partial | [store repositories](../internal/store); [evidence tests](../internal/store/evidence_test.go); immutable configuration and append-only stage attempts are covered, while retention and recovery remain open (T011, T034, T051) |
| FR-003 immutable configuration and attempts | Partial | [versioned persistence and tests](../internal/store/migrations/004_run_configuration.sql); effective routes are hashed into run reports and immutable stage attempts are append-only, but retry orchestration and end-to-end historical attempt reporting remain open |
| FR-004 single owner and workspace writer | Partial | [workspace lock/owner lease tests](../internal/workflow/engine_test.go); stale owner reconciliation is implemented, while process-start identity and complete restart validation remain open (T034) |
| FR-005 canonical workflow stages | Partial | [workflow engine](../internal/workflow/engine.go); stage execution is deterministic simulation only; live Spec-Kit orchestration remains open (T016) |
| FR-006 verified official Spec-Kit invocation | Open | [Spec-Kit discovery and command mapping](../internal/speckit/discovery.go); stage invocation orchestration remains open (T016) |
| FR-007 one product spec through retries | Partial | [master feature spec](../specs/001-build-spectide/spec.md), [task ledger](../specs/001-build-spectide/tasks.md); retry/convergence workflow remains open (T035) |
| FR-008 bounded read-only analysis remediation | Partial | [analyze parser](../internal/speckit/parse.go), [analysis report](../specs/001-build-spectide/analysis.md); bounded workflow loop remains open (T016) |
| FR-009 append-only bounded convergence loop | Partial | [append-only validator](../internal/speckit/parse.go); implementation/test/converge return loop remains open (T035) |
| FR-010 evidence-backed quality verdicts | Partial | [quality registry tests](../internal/quality/registry_test.go); run-level required evidence remains simulated |
| FR-011 deterministic simulated workflow | Pass | [20-run SC-002 acceptance test](../internal/workflow/engine_test.go); [isolated smoke script](../scripts/smoke.sh) |
| FR-012 three provider adapters | Partial | [adapter contract tests](../internal/adapters/adapters_test.go) and [scripted fake tests](../internal/adapters/fake/fake_test.go); stable typed errors and fail-closed invocation policy are tested, with scripted failure/cancellation/rate-limit behavior; canonical workflow integration remains open (T014) |
| FR-013 independently configured safe profiles | Partial | [profile persistence tests](../internal/store/profiles_test.go); authorized live profile execution remains disabled |
| FR-014 capability and execution-policy validation | Partial | [routing policy tests](../internal/routing/routing_test.go); config/profile/capability policy is not yet integrated into run launch (T024) |
| FR-015 routing modes and audit decisions | Partial | [config routing tests](../internal/config/config_test.go), [workflow route persistence test](../internal/workflow/engine_test.go); full policy inputs remain incomplete (T024) |
| FR-016 deny-first workspace/profile boundaries | Partial | [workspace tests](../internal/workspace/workspace_test.go), [routing tests](../internal/routing/routing_test.go); end-to-end launch authorization remains open (T024) |
| FR-017 bounded safe subprocess execution | Partial | [runner](../internal/runner/runner.go); output is capped at 1 MiB per stream, timeouts default to 30 minutes and are bounded from 1 second to 4 hours, and cancellation escalates after a 2-second grace period; workflow integration remains open |
| FR-018 atomic transitions and interrupted recovery | Partial | [store transition/recovery implementation](../internal/store/store.go); expired owner leases become blocked with durable interruption evidence; full checkpoint recovery remains open (T034) |
| FR-019 safe pause, resume, cancellation, crash recovery | Partial | [cross-process and stale-owner tests](../internal/workflow/engine_test.go); fresh owners accept controls, expired owners block with `WORKSPACE_DRIFT` and release locks; positive restart resume with matching checkpoint/artifact/Git fingerprints remains unimplemented (T034) |
| FR-020 loopback HTTP interface | Partial | [API handlers and tests](../internal/api/server_test.go); capabilities and some documented endpoint behaviors remain open |
| FR-021 ordered SSE replay and slow clients | Partial | [API replay tests](../internal/api/server_test.go); 10,000-event p95 measurement remains open (T043a) |
| FR-022 browser/API request protection | Pass | [API security tests](../internal/api/server_test.go) cover host, origin, CSRF, limits, path replacement, and request validation |
| FR-023 secret-safe event and report handling | Partial | [redaction tests](../internal/security/redact_test.go), [store evidence tests](../internal/store/evidence_test.go); full export/display coverage remains open (T036) |
| FR-024 persisted-state dashboard | Partial | [dashboard source](../web/src/App.tsx); screenshot/browser acceptance is open (T031, T046) |
| FR-025 route preview and truthful states | Partial | [CLI/API preview](../internal/cli/root.go), [API symlink-boundary tests](../internal/api/server_test.go), and [dashboard](../web/src/App.tsx); preview resolves configured stage routes, reports `NOT_RUN`, creates no run, and rejects config symlink escapes; browser page/state acceptance remains open (T031) |
| FR-026 accessible desktop monitoring | Partial | [dashboard stylesheet](../web/src/style.css), [domain tests](../web/src/domain.test.ts); T031 records the run-list/detail, route-preview, profile-readiness and empty/error/disconnected matrix plus scan results in `docs/accessibility-audit.md` |
| FR-027 migrations, recovery, retention, export | Partial | [migration tests](../internal/store/profiles_test.go), [backup/retention tests](../internal/store/retention_test.go); private integrity-checked backup/restore, 90-day detail expiration, 1,000-run terminal-first pruning, and active-cap rejection pass; sanitized export formatting and recovery integration remain open |
| FR-028 documented offline verification commands | Partial | [Makefile](../Makefile), [smoke scripts](../scripts); CI/security/E2E coverage is incomplete (T003, T043) |
| FR-029 automated cross-layer coverage | Partial | [Go and frontend test suites](../internal); browser E2E, restart recovery, convergence failure injection, and SSE load evidence remain open (T031, T036, T043a) |
| FR-030 local-first verified documentation | Partial | [README](../README.md), [architecture](architecture.md), [security model](security-model.md); provider, recovery, API, troubleshooting, and release documentation remains incomplete (T040) |

## Buildable success criteria

| Criterion | Current evidence/status |
|---|---|
| SC-001 | Pass on recorded host: macOS 15.1, Apple M4, 16 GiB, arm64; latest corrected same-process monotonic `make verify` measured 18.830 ms process-start-to-first-HTTP-200 at `/healthz`. See [verification evidence](final-verification.md). |
| SC-002 | Pass: 20 deterministic runs passed in `TestTwentyDeterministicRunsCompleteWithRequiredEvidence`. |
| SC-003 | Partial: quality verdict types exist; terminal simulated reports pass; live blocked/not-run behavior needs end-to-end acceptance. |
| SC-004 | Partial: persisted replay and cursor tests pass; two independent browser clients are not yet covered. |
| SC-005 | Partial: API/routing/security unit coverage passes; one unified unauthorized-route attempt suite remains open. |
| SC-006 | Open: the profile is specified in `spec.md`; 10 clients, 100 events/s for 100 seconds, 10,000-event backlog, timestamp-to-frame p95, and no-missing-event assertion remain to be measured (T043a). |
| SC-007 | Pass: `make smoke` completed with explicit `SIMULATED` output and isolated temporary state. |
| SC-008 | Open: the spec now defines every FR-001 through FR-030 as required for v1; complete linked evidence remains outstanding. |
