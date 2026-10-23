# SpecTide

**Orchestrate. Implement. Converge.**

SpecTide is a local-first control plane for evidence-backed, profile-aware Spec-Kit workflows. It provides durable SQLite history, a loopback dashboard/API, SSE replay, and a CLI. Claude Code, Codex, and OpenCode have fail-closed adapters and fixture-backed workflow coverage; authenticated execution against real provider accounts and official Spec-Kit convergence remain separate acceptance checks and are not claimed as passed. The offline demo uses a deterministic simulated provider.

## Offline quickstart

Development requires Go 1.27 and Node.js 26 with npm. CI uses the same toolchain versions.

```bash
make test
make build
./bin/spectide serve
# in another terminal
./bin/spectide demo --project ./examples/fake-provider-demo
```

Open <http://127.0.0.1:4747>. The demo is visibly **SIMULATED** and does not use provider credentials.

Read the [local user guide](docs/user-guide.md), [configuration reference](docs/configuration.md), [provider readiness](docs/providers.md), [recovery guidance](docs/recovery.md), and [local API contract](docs/api.md).

For local macOS/Linux release artifact preparation, checksums, and verification steps, see [release preparation](docs/release-preparation.md). This workflow prepares files only; it does not publish them.

## Security status

The server only accepts loopback hosts and requires a local anti-CSRF header for mutations. The current development token is intentionally local and bootstrap-only; production session handling, authorization, and real provider process execution remain tracked in `specs/001-build-spectide/tasks.md`.

## Development ledger

The source blueprint is tracked as one product feature in `specs/001-build-spectide/`. See its `spec.md`, `plan.md`, `tasks.md`, and `analysis.md` for scope and quality evidence.
