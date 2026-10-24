# Source Blueprint Traceability

**Feature**: `001-build-spectide`  
**Source**: [`SpecTide_End_to_End_Implementation_Plan.md`](../../SpecTide_End_to_End_Implementation_Plan.md)  
**Purpose**: Make the source blueprint's coverage explicit. This is a documentation and review aid; it does not change product scope, implementation status, or release acceptance.

## How to read this matrix

- **Covered** means the source section has a normative home in the feature specification, plan, contract, task ledger, or released project documentation.
- **Reference-only** means the section guides the original bootstrap/review process rather than being a v1 runtime requirement.
- **P1/out of scope** means it is deliberately not required for v1, as established by the source blueprint.
- **Variance to decide** means the source describes a concrete public contract or technology decision that is not identical to the current documented contract. It is recorded here for an explicit product decision; it does not authorize code changes.

The authoritative v1 release criteria remain [FR-001–FR-030 and SC-001–SC-008](spec.md#functional-requirements). Current implementation and evidence status is maintained separately in [`docs/acceptance-ledger.md`](../../docs/acceptance-ledger.md); coverage in this matrix must not be mistaken for an acceptance pass.

## Section coverage

| Source section | Classification | Authoritative destination(s) | Coverage / decision |
| --- | --- | --- | --- |
| §0 One-time execution | Reference-only | [spec.md](spec.md#assumptions), [tasks.md](tasks.md) | Defines how an external agent bootstraps this product and the truthful-completion rule. The one-product-spec, resumability, blocker, and evidence outcomes are captured by FR-005–FR-010 and FR-018–FR-019. The kickoff prompt itself is retained only in the source blueprint. |
| §1 Vision and identity | Covered | [spec.md](spec.md#user-scenarios--testing-mandatory), [README.md](../../README.md) | The five user stories and product description express the local-first, provider-neutral orchestration value. |
| §2 Scope and priorities | Covered; P1/out of scope retained as reference | [spec.md](spec.md#assumptions), FR-001–FR-030 | P0 is made release-required by the spec's release-priority statement. Hosted SaaS, concurrent writers/worktrees, remote access, deployments, and other listed exclusions remain outside v1. P1 is not silently promoted to a v1 requirement. |
| §3 Decided stack and distribution | Covered as implementation context | [plan.md](plan.md#technical-context), [research.md](research.md), [docs/architecture.md](../../docs/architecture.md), [docs/openapi.yaml](../../docs/openapi.yaml), [spec.md](spec.md#assumptions) | Decision: named libraries are architectural guidance, not v1 product acceptance constraints. Go/React/SQLite/SSE/embed, offline testing, and macOS/Linux support remain documented constraints. |
| §4 User stories and acceptance | Covered | [spec.md](spec.md#user-scenarios--testing-mandatory), FR-001–FR-030, SC-001–SC-008 | US-01 through US-10 map to the five consolidated feature stories; provider outage/unsupported capability is covered by FR-012, FR-014, FR-015, and the edge cases. |
| §5 Architecture | Covered | [plan.md](plan.md#summary), [docs/architecture.md](../../docs/architecture.md), FR-004–FR-025 | One scheduler, local daemon/foreground behavior, safe process lifecycle, artifact handoff, SQLite truth, and dashboard/API boundaries have normative homes. Optional upstream-workflow compatibility remains later-version reference material. |
| §6 Repository layout | Covered as implementation context | [plan.md](plan.md#project-structure), [tasks.md](tasks.md) | The feature plan documents package boundaries; the source blueprint's illustrative tree is not a required byte-for-byte repository layout. |
| §7 Domain model and state machine | Covered | [data-model.md](data-model.md), FR-002–FR-004, FR-018–FR-019, [docs/architecture.md](../../docs/architecture.md) | Immutable runs/attempts, routing decisions, events, artifacts, gates, approvals, checkpoints, and writer locks are defined and acceptance-tested through the referenced requirements. |
| §8 Routing, profiles, models, effort | Covered | FR-013–FR-016, [docs/configuration.md](../../docs/configuration.md), [docs/providers.md](../../docs/providers.md), T008/T024/T064 | Configuration precedence, safe profiles, manual/hybrid/automatic routing, validation, boundaries, and dry-run intent are documented and task-traced. Cost limits remain best-effort guidance, not a billing guarantee. |
| §9 Provider adapter contracts | Covered | FR-011–FR-017, [docs/providers.md](../../docs/providers.md), [docs/security-model.md](../../docs/security-model.md), adapter tasks/tests | Claude Code, Codex, OpenCode, and Fake fixtures; probing; sanitized invocation; stream normalization; failure types; and installed Spec-Kit discovery are v1 requirements. Exact CLI switches remain capability-verified rather than frozen in the product specification. |
| §10 Persistence contract | Covered | [data-model.md](data-model.md), FR-002–FR-003, FR-018, FR-027, [docs/recovery.md](../../docs/recovery.md) | SQLite migrations, immutable evidence, atomic events, recovery, retention, backup-aware behavior, and sanitized export are normative. |
| §11 Events, SSE, monitoring | Covered | FR-018, FR-021, FR-023–FR-025, [contracts/http-api.md](contracts/http-api.md), [docs/openapi.yaml](../../docs/openapi.yaml) | Ordered cursors/replay, heartbeats, bounded viewers, redaction, and persisted dashboard state are requirements. The versioned wire schema is maintained in the OpenAPI/API documentation. |
| §12 HTTP API | Covered; approved source-compatibility work pending | FR-020–FR-023, [contracts/http-api.md](contracts/http-api.md), [docs/api.md](../../docs/api.md), [docs/openapi.yaml](../../docs/openapi.yaml), T141 | `docs/openapi.yaml` remains authoritative. Decision: the blueprint's global approvals queue, profile `PATCH`, distinct `/providers/capabilities`, and `/config/resolved` paths are P0 compatibility requirements and will be added without removing current routes. |
| §13 Dashboard UX | Covered | FR-024–FR-026, [docs/accessibility-audit.md](../../docs/accessibility-audit.md), [docs/screenshots/README.md](../../docs/screenshots/README.md) | Persisted state, route preview, controls, live activity, approvals, quality evidence, accessibility matrix, and screenshot evidence are represented. The source's optional light theme and mobile enhancements are not v1 release requirements. |
| §14 CLI | Covered; approved command-surface work pending | FR-001, [README.md](../../README.md), [docs/user-guide.md](../../docs/user-guide.md), T142 | Decision: `workflow list`, `workflow explain`, and the three global output controls are P0. `--detach` is P1 because it adds daemon-lifecycle complexity and is not necessary for the normal attached run path. |
| §15 Quality gates and completion | Covered; stable taxonomy work pending | FR-008–FR-010, FR-028–FR-030, SC-001–SC-008, [docs/acceptance-ledger.md](../../docs/acceptance-ledger.md), [docs/final-verification.md](../../docs/final-verification.md), T143 | Decision: QG-01 through QG-13 are stable release-evidence IDs and must appear in the ledger/report. They need not be public HTTP enum values or prohibit more detailed internal gates. |
| §16 Security and trust boundaries | Covered | FR-016–FR-023, [docs/security-model.md](../../docs/security-model.md), [docs/recovery.md](../../docs/recovery.md) | Loopback/CSRF/Host/Origin controls, path and profile boundaries, safe subprocesses, approval boundaries, redaction, and safety tests are normative. |
| §17 Execution roadmap | Reference-only historical execution plan | [tasks.md](tasks.md), [analysis.md](analysis.md), [docs/implementation-progress.md](../../docs/implementation-progress.md) | Phase ordering and checkpoints are planning history, not separate product requirements. |
| §18 Test strategy | Covered | FR-011–FR-012, FR-017–FR-019, FR-021–FR-023, FR-028–FR-029, SC-001–SC-008, [docs/acceptance-ledger.md](../../docs/acceptance-ledger.md) | Unit, fixture adapter, fake-provider lifecycle, live-smoke `NOT_RUN`, resilience, and coverage expectations are covered. Evidence status remains separate from scope coverage. |
| §19 CI, packaging, portfolio quality | Covered | FR-028–FR-030, [README.md](../../README.md), [docs/release-preparation.md](../../docs/release-preparation.md), [`.github/workflows`](../../.github/workflows) | Offline CI, build/release preparation, checksums, documented toolchains, portfolio documentation, and no-auto-publish behavior are represented. Windows remains validation where feasible, not a primary v1 support promise. |
| §20 Documentation deliverables | Covered | FR-030, [docs/](../../docs), root policy files, T040/T046 | The listed contributor guides, architecture/configuration/provider/security/recovery/troubleshooting/API docs, screenshots, examples, and policy files are traceable to FR-030. |
| §21 Risk register | Covered by distributed mitigations | FR-004, FR-006, FR-008–FR-023, [docs/security-model.md](../../docs/security-model.md), [docs/recovery.md](../../docs/recovery.md) | The register is design rationale; individual mitigations are enforced by the corresponding functional requirements and evidence. |
| §22 Mac bootstrap | Reference-only | [README.md](../../README.md), [docs/user-guide.md](../../docs/user-guide.md), [quickstart.md](quickstart.md) | Bootstrap instructions for building the originally absent product are retained as project-use guidance, not runtime behavior. |
| §23 Final release checklist | Covered | SC-008, [docs/acceptance-ledger.md](../../docs/acceptance-ledger.md), [docs/final-verification.md](../../docs/final-verification.md) | The feature requires evidence-linked release acceptance and accurate `NOT_RUN`/blocker reporting. |
| §24 Source references | Reference-only, verify at implementation time | Source blueprint §24; [docs/providers.md](../../docs/providers.md) | External CLI documentation can change. Installed capability checks, fixture contracts, and provider docs are the maintained validation mechanism. |
| §25 Executive summary | Covered by the complete matrix | [spec.md](spec.md), [plan.md](plan.md), [tasks.md](tasks.md) | It introduces no independent product requirement beyond the preceding sections. |

## Approved blueprint-parity decisions

These decisions update the v1 acceptance contract. They do not change code or claim implementation completion. T141–T143 are release-priority work and must be implemented, verified, and analyzed before expanded-scope release acceptance.

| Source requirement | Decision | Future implementation task |
| --- | --- | --- |
| Blueprint §12 lists `GET /api/v1/approvals`, `PATCH /api/v1/profiles/{id}`, `GET /api/v1/providers/capabilities`, and `GET /api/v1/config/resolved`. | These exact paths are P0 additions; current equivalent routes remain available for compatibility. | T141 |
| Blueprint §14 lists `spectide workflow {list,explain}`, `--detach`, and global `--no-color`, `--quiet`, and `--verbose`. | `workflow list/explain` and output controls are P0; `--detach` is P1. | T142 |
| Blueprint §3 names `chi`, Tailwind/shadcn, TanStack Query, and React Flow. | These are non-binding architecture guidance. Existing or future implementation may differ while preserving the approved behavioral constraints. | None |
| Blueprint §15 names thirteen gate IDs. | QG-01–QG-13 are stable in release evidence and reports, but not required as public API enum values. | T143 |

## Review procedure after this matrix changes

1. Implement T141–T143 within their approved scope and without mixing unrelated convergence work.
2. Add tests and release-evidence links for the new FR-001, FR-010, and FR-020 requirements.
3. Re-run the read-only Spec-Kit analysis workflow and update this matrix with its result.
