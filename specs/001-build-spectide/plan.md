# Implementation Plan: Build SpecTide v1

**Branch**: `001-build-spectide` | **Date**: 2026-10-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command; its definition describes the execution workflow.

## Summary

Build a local-first orchestration application that turns a developer task into a durable, evidence-backed Spec-Kit workflow. Go owns the CLI, state machine, provider process supervision, local HTTP/SSE API, and SQLite truth; an embedded React dashboard renders that observed state. The initial acceptance path is deterministic simulation; Claude Code, Codex, and OpenCode adapters use capability probing and fail closed when unsupported or unauthenticated.

## Technical Context


**Language/Version**: Go 1.27; TypeScript 5.x / Node 26 for dashboard tooling

**Primary Dependencies**: Cobra CLI, pure-Go SQLite driver, standard HTTP/SSE; React, Vite, TypeScript for the dashboard

**Storage**: Local SQLite in WAL mode; bounded sanitized logs and embedded static assets

**Testing**: `go test`, race detector, fixture adapter tests, HTTP/SSE integration tests; dashboard unit and browser tests once React build is added

**Target Platform**: Local macOS/Linux developer machines; Windows build validation where provider capabilities permit

**Project Type**: Local CLI plus embedded web application

**Performance Goals**: Local health response within 2 seconds; p95 simulated event visibility within 1 second; incremental replay for 10,000 stored events

**Constraints**: Loopback-only; no stored provider credentials; no browser-originated arbitrary shell; one workspace writer; simulation never presented as live verification

**Scale/Scope**: One local user/daemon, 100 recent runs and 10,000 events per run as initial acceptance capacity

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Constitutional gate | Decision | Status |
| --- | --- | --- |
| Local-first, least privilege | Loopback HTTP, CSRF/Host/Origin validation, explicit process arguments, no secrets in storage | PASS |
| Evidence before completion | Typed gates and simulated labeling; live checks remain `NOT_RUN` without authorization | PASS |
| One product scope, real Spec-Kit semantics | One feature directory; bridge must detect commands and block if absent | PASS (implementation pending) |
| Durable/observable workflows | SQLite transaction/event model, checkpoints and one writer lock | PASS (implementation staged) |
| Provider-neutral, accessible delivery | Adapter contract, deterministic fake provider, accessible dashboard | PASS (implementation staged) |

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/spectide/              # command entrypoint
internal/{app,cli,model}/  # composition, commands, domain types
internal/{store,workflow}/ # SQLite truth, evented state machine
internal/{adapters,runner,speckit,routing,security}/ # provider and safety boundaries
internal/api/              # HTTP handlers and SSE
internal/webassets/        # embedded dashboard build
web/                       # React/TypeScript source and browser tests
migrations/                # versioned schema SQL
docs/, examples/, testdata/
```

**Structure Decision**: A single Go module supplies the local authority and embeds one React dashboard. Package boundaries match constitutional security, persistence, provider, and workflow ownership.

## Complexity Tracking

No constitutional exceptions requested. Provider adapters remain separate because their executable capabilities and event formats are incompatible; they share one narrow adapter contract.
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
