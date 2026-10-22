# SpecTide

**Orchestrate. Implement. Converge.**

SpecTide is a local-first control plane for evidence-backed, profile-aware Spec-Kit workflows. The current bootstrap build provides a durable SQLite event log, simulated full lifecycle, loopback dashboard/API, SSE replay, and a CLI. Claude Code, Codex, and OpenCode are discovered by `doctor`; live orchestration is intentionally not claimed until their fail-closed adapters and fixture contracts are complete.

## Offline quickstart

```bash
make test
make build
./bin/spectide serve
# in another terminal
./bin/spectide demo --project ./examples/fake-provider-demo
```

Open <http://127.0.0.1:4747>. The demo is visibly **SIMULATED** and does not use provider credentials.

## Security status

The server only accepts loopback hosts and requires a local anti-CSRF header for mutations. The current development token is intentionally local and bootstrap-only; production session handling, authorization, and real provider process execution remain tracked in `specs/001-build-spectide/tasks.md`.

## Development ledger

The source blueprint is tracked as one product feature in `specs/001-build-spectide/`. See its `spec.md`, `plan.md`, `tasks.md`, and `analysis.md` for scope and quality evidence.
