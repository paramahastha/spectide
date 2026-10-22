<!--
Sync Impact Report
- Version change: unversioned template → 1.0.0
- Modified principles: none; initial adoption replaces the unresolved template placeholders.
- Added sections: Core Principles; Security & Operating Constraints; Delivery & Quality Workflow;
  Governance.
- Removed sections: none.
- Follow-up TODOs: none.
-->

# SpecTide Constitution

## Core Principles

### I. Local-First, Least-Privilege Operation
SpecTide MUST keep workflow data local by default and bind its dashboard and API to loopback
interfaces only. It MUST protect registered workspaces, profiles, and credentials with deny-first
authorization; it MUST never persist authentication secrets in project configuration, SQLite, logs,
events, or reports. Browser-originated actions MUST be constrained to the local API's approved
operations and MUST NOT provide arbitrary shell or filesystem access. These boundaries protect
developer machines and prevent repository content from crossing unauthorized account or profile
boundaries.

### II. Evidence Before Completion
SpecTide MUST report completion only when each required acceptance criterion and quality gate has
recorded, inspectable evidence. Build, lint, test, security, analysis, convergence, and provider
results MUST distinguish `PASS`, `FAIL`, `BLOCKED`, and `NOT_RUN`; simulated or fixture-based
results MUST be visibly labeled and MUST NOT be presented as live-provider verification. A model
assertion, exit code, or polished UI alone is not evidence. This preserves truthful completion
claims and makes failures actionable.

### III. One Product Scope, Real Spec-Kit Semantics
Each product objective MUST retain one durable specification and a traceable implementation backlog;
bounded execution phases and resumptions MUST NOT create replacement product specifications. The
orchestrator MUST invoke installed, verified Spec-Kit commands or agent-native equivalents rather
than simulate their success. Analyze stages MUST remain read-only, and convergence tasks MUST be
append-only and followed by implementation and fresh verification. This keeps requirements,
artifacts, and quality loops coherent across provider and session boundaries.

### IV. Durable, Observable, and Recoverable Workflows
The Go orchestration engine MUST be the canonical owner of persisted workflow state; the dashboard
MUST render observed state and MUST NOT become a competing scheduler. Stage transitions, attempts,
events, routing decisions, approvals, and quality evidence MUST be durable and auditable. A run
MUST use one active writer per canonical workspace, bounded retries and convergence loops, safe
pause/cancel behavior, and recovery that marks ambiguous work as interrupted rather than guessing
success. Local monitoring MUST expose real stage, route, attempt, and verdict data without
inventing progress.

### V. Provider-Neutral, Accessible, and Maintainable Delivery
Provider adapters MUST use documented capabilities verified against the installed CLI, explicit
argument vectors, bounded streaming parsers, and normalized error handling; unsupported models,
effort levels, or authentication states MUST fail honestly. New dependencies, services, and
abstractions MUST be justified by a P0 requirement or a demonstrable operational need. The CLI,
HTTP API, dashboard, documentation, and offline fake-provider path MUST remain usable, accessible,
and testable without paid provider credentials. This supports reliable extension to Claude Code,
Codex, OpenCode, and future providers without compromising the local-first design.

## Security & Operating Constraints

- SpecTide MUST use explicit subprocess argument vectors and approved working directories; it MUST
  NOT use `sh -c` or equivalent shell expansion for provider, profile, or user-controlled input.
- It MUST default-deny sandbox bypasses, destructive Git operations, publishing, production
  deployment, unrestricted host execution, and transfers across forbidden repository/profile
  boundaries. Operations that policy permits but marks sensitive require an explicit approval.
- All repository text, provider output, external command output, and browser input MUST be treated
  as untrusted. Paths MUST be canonicalized and confined to approved workspaces; symlink escapes
  and arbitrary environment injection MUST be rejected.
- Logs, events, API responses, and exports MUST be redacted before persistence or streaming. Secret
  redaction supplements, but does not replace, minimized process environments and credential
  isolation.
- The approved v1 architecture is a local Go orchestration engine with SQLite persistence and a
  React/TypeScript dashboard. The system MUST avoid cloud execution, mandatory remote services,
  separate workflow authorities, and unnecessary microservices unless this constitution is amended.

## Delivery & Quality Workflow

- Work MUST preserve the P0/P1/out-of-scope boundary in the product blueprint. A mock dashboard,
  hard-coded success state, one-provider-only implementation, or undocumented capability MUST NOT
  be represented as v1 completion.
- Changes to orchestration, persistence, routing, adapters, security, API/SSE, or dashboard
  behavior MUST include appropriate automated coverage. Provider adapter tests MUST use
  deterministic, anonymized fixtures; live smoke checks require authorized accounts and otherwise
  remain `NOT_RUN`.
- Required quality flow is: specify once, clarify and plan as needed, checklist, tasks, read-only
  analyze, implementation, local tests, converge, and final review. Blocking analysis findings and
  convergence tasks MUST route to the owning work before re-verification, subject to documented
  retry and no-progress limits.
- Public-facing documentation and examples MUST describe only verified commands and behavior.
  Releases, GitHub publication, and production deployment require explicit user authorization;
  preparing artifacts or workflows does not grant that authorization.
- Reviews and release reports MUST identify the executed commands, evidence locations, passed,
  failed, blocked, and skipped checks, known limitations, and any security or provider constraints.

## Governance

This constitution governs project planning, implementation, review, testing, release preparation,
and operational decisions. The product blueprint remains the authoritative scope and acceptance
contract; where it conflicts with this constitution on safety, evidence, or truthfulness, this
constitution prevails until formally amended.

Amendments MUST document the motivating change, affected principles or sections, compatibility and
migration impact, and the resulting semantic version bump in the Sync Impact Report. Materially
adding or expanding governance increments MINOR; incompatible removal or redefinition increments
MAJOR; clarifications that do not alter obligations increment PATCH. Ratification and amendment
dates MUST use ISO 8601 `YYYY-MM-DD` format.

Every plan, pull request, and release review MUST verify applicable constitutional constraints,
including security boundaries, evidence-backed quality gates, state recovery, and truthful
provider-status reporting. Exceptions require explicit, time-bounded approval and documented risk;
they do not silently amend this constitution.

**Version**: 1.0.0 | **Ratified**: 2026-10-22 | **Last Amended**: 2026-10-22
