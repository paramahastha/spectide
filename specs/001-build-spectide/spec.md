# Feature Specification: Build SpecTide v1

**Feature Branch**: `001-build-spectide`

**Created**: 2026-10-22

**Status**: In implementation

**Input**: User description: "Implement SpecTide end to end from `SpecTide_End_to_End_Implementation_Plan.md` as one cohesive, local-first v1 product."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run a complete development workflow (Priority: P1)

A developer can submit one software task to a registered local repository and have SpecTide create, execute, monitor, and report a durable development workflow without manually coordinating ordinary specification-driven stages.

**Why this priority**: The complete, evidence-backed workflow is the core product value; all other capabilities support it.

**Independent Test**: Using the deterministic simulated provider in a disposable repository, submit a task and verify a truthful terminal state (`COMPLETED`, `BLOCKED`, or `FAILED`) with persisted stages, events, quality gates, and a report. Simulated stage results must be visibly `SIMULATED` and must never claim an official Spec-Kit command was invoked; a real workflow that requires a missing or unusable Spec-Kit command must be `BLOCKED`.

**Acceptance Scenarios**:

1. **Given** an initialized allowed repository and valid simulated profile, **When** a developer starts a full workflow, **Then** the system creates one durable run and records the configured specification, analysis, implementation, test, convergence, review, and reporting stages as explicitly simulated; no simulated stage may be represented as an official Spec-Kit invocation.
2. **Given** a stage produces required evidence, **When** that stage completes, **Then** its route, attempt, result, artifacts, and quality evidence are recorded and visible in the run report.
3. **Given** a required provider command or required evidence is unavailable, **When** the workflow reaches that stage, **Then** the run is marked blocked or failed with an actionable reason rather than reported as successful.

---

### User Story 2 - Route work across authorized profiles (Priority: P1)

A developer can register local Claude Code, Codex, OpenCode, and simulated profiles, assign a provider/profile/model/effort route to each workflow stage, and preview the resolved routes before work begins.

**Why this priority**: Stage-specific, permission-aware provider selection differentiates SpecTide from a single-agent command wrapper.

**Independent Test**: Configure separate simulated profiles for planning, implementation, and review; preview and run the workflow; verify the persisted route and events for every stage match the approved configuration.

**Acceptance Scenarios**:

1. **Given** explicit valid manual routes, **When** a run starts, **Then** every stage uses the selected route without silent substitution.
2. **Given** hybrid or automatic routing, **When** a route is resolved, **Then** the system records the inputs, policy, selected route, fallback rules, and explanation.
3. **Given** a profile is not authorized for a project, **When** it is selected directly or as a fallback, **Then** launch is denied and no repository content is sent to that profile.

---

### User Story 3 - Observe and control a live local run (Priority: P1)

A developer can use the CLI and local dashboard to see real-time run state, stage routes, attempts, logs, approvals, quality verdicts, and reports, and can safely pause, resume, cancel, or decide an approval.

**Why this priority**: Trustworthy observability and recoverable control are essential when autonomous local processes modify a repository.

**Independent Test**: Start a simulated run, attach two dashboard clients, reconnect one client after events are emitted, exercise pause/resume/cancel or an approval, and verify both clients reconstruct the same persisted state.

**Acceptance Scenarios**:

1. **Given** an active run, **When** a viewer opens its detail page, **Then** it sees the actual active stage, provider, profile, model, effort, attempt count, event stream, and quality state without a page refresh.
2. **Given** a viewer reconnects with the last received event position, **When** the connection is restored, **Then** it receives ordered missed events without loss or duplicate canonical state changes.
3. **Given** a run is paused, interrupted, cancelled, or waiting for approval, **When** the developer issues the permitted control action, **Then** the resulting transition is durable, audited, and safe for the current stage.

---

### User Story 4 - Recover safely from failures (Priority: P1)

A developer can recover a failed or interrupted local workflow without duplicate writes or false completion claims.

**Why this priority**: Long-running multi-provider workflows are only useful if failures, rate limits, restarts, and convergence gaps are handled honestly.

**Independent Test**: Inject simulated provider failure, test failure, convergence additions, daemon interruption, and workspace lock contention; verify bounded repair/retry behavior and accurate terminal reports.

**Acceptance Scenarios**:

1. **Given** a test fails after implementation, **When** a permitted repair is available within configured limits, **Then** the system routes to implementation, reruns required tests, and reruns convergence after tests pass.
2. **Given** convergence adds work, **When** the additions are valid and progress is made, **Then** the system implements only the appended work and repeats verification within the configured convergence limit.
3. **Given** a running process is ambiguous after restart, **When** recovery occurs, **Then** the corresponding work is marked interrupted and requires a verified safe resume path rather than assumed successful.

---

### User Story 5 - Use and validate a local installation (Priority: P2)

A new contributor can build SpecTide, run the offline simulated demonstration, understand its security and provider limitations, and use documented CLI and dashboard workflows without paid provider access.

**Why this priority**: Reproducible local evaluation and truthful documentation make the project usable and portfolio-quality.

**Independent Test**: In a clean local checkout without provider credentials, follow the documented build and demo commands and verify the local dashboard, report, and automated checks operate with simulated status visibly labeled.

**Acceptance Scenarios**:

1. **Given** a clean supported development environment, **When** a contributor runs documented build and test commands, **Then** a runnable local binary and dashboard are produced and required offline checks pass.
2. **Given** no live provider credentials, **When** the contributor runs the demonstration, **Then** it completes with deterministic simulated evidence and does not claim live-provider verification.

### Edge Cases

- A selected executable is absent, non-executable, an interactive shell alias, unauthenticated, or does not support the selected model, effort, or safety mode.
- A provider emits incomplete, malformed, unknown-version, oversized, or secret-containing output.
- A required official Spec-Kit stage is missing, writes during read-only analysis, creates no expected artifact, or reports ambiguous convergence output.
- A browser request has an invalid host, origin, CSRF token, run identifier, cursor, path, or oversized payload.
- A workspace path, artifact path, or configured executable resolves through a symlink outside its approved boundary.
- Two runs request writes to the same canonical workspace; a stale lock, daemon restart, timeout, cancellation, or process identifier reuse occurs.
- Event clients are slow, disconnect during replay, or reconnect after a high event volume.
- A fallback route would cross a forbidden project/profile data boundary or a profile needs interactive authentication.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide installable local commands to diagnose, initialize, configure, manage profiles and models, create, observe, pause, resume, cancel, list, log, report, serve, open the dashboard, and run an offline demonstration.
- **FR-002**: The system MUST persist projects, runs, stage attempts, routing decisions, events, approvals, artifacts, checkpoints, test executions, and quality verdicts locally without persisting provider authentication secrets.
- **FR-003**: The system MUST treat persisted run configuration and historical attempt results as immutable evidence while retaining new attempts separately.
- **FR-004**: The system MUST execute one canonical workflow owner per run and permit only one active writer for each canonical workspace.
- **FR-005**: The system MUST implement the canonical stages: constitution when absent, specify once, clarify, plan, checklist, tasks, analyze, implement, tests, converge, final review, and report.
- **FR-006**: The system MUST invoke verified installed official Spec-Kit stage integrations or report the stage blocked; it MUST NOT fabricate a successful invocation.
- **FR-007**: The system MUST preserve one product specification throughout remediation, retries, and resumptions rather than create replacement specifications.
- **FR-008**: The system MUST keep analysis read-only, route blocking analysis findings to the appropriate earlier artifact owner, and rerun analysis within a configured bound. Critical and high findings block progress; medium findings block only when they leave a security boundary, required acceptance criterion, or prerequisite unsatisfied. Route scope and acceptance findings to `spec.md`, architecture/data/API findings to `plan.md` or its referenced design contract, and task coverage/order findings to `tasks.md`. A remediation round is limited to editing the owning artifact and then running a fresh read-only analysis.
- **FR-009**: The system MUST run convergence only after implementation and tests for current tasks, accept only append-only valid task additions, and rerun implementation, tests, and convergence until converged or bounded progress fails. Defaults are at most 3 analyze remediation rounds, 4 test repair attempts, and 5 convergence rounds per run. The initial baseline analysis is not a remediation round; each remediation round consists of one finding-driven artifact change followed by one read-only re-analysis. A convergence round makes progress only when valid new tasks are appended or a previously failing required gate becomes passing; an unchanged task-ledger hash and unchanged required-gate verdicts is no progress and blocks the run.
- **FR-010**: The system MUST require inspectable evidence for required quality gates and represent pass, fail, blocked, and not-run verdicts distinctly.
- **FR-011**: The system MUST provide deterministic simulated provider workflows and failure scenarios that run without paid accounts and are visibly marked simulated.
- **FR-012**: The system MUST provide process adapters for Claude Code, Codex, and OpenCode, including capability probing, sanitized invocation generation, streaming event normalization, typed failures, and fixture-based contract tests.
- **FR-013**: The system MUST support independently configured local profiles, including executable and permitted configuration references, without reading or copying account credentials.
- **FR-014**: The system MUST validate provider, profile, model, effort, fallback, capability, and execution-policy selections before launch and reject unsupported combinations unless an explicitly approved fallback applies.
- **FR-015**: The system MUST support manual, hybrid, and deterministic automatic routing; each resolved stage route MUST be recorded with its policy inputs and explanation.
- **FR-016**: The system MUST enforce deny-first project/profile authorization and MUST deny routing or fallback that violates a configured data boundary.
- **FR-017**: The system MUST execute provider processes using explicit argument vectors, constrained working directories, minimized environments, bounded output handling, timeouts, and safe cancellation; it MUST NOT expose arbitrary browser-controlled shell execution. Retain at most 1 MiB each of stdout and stderr. Provider stage processes default to a 30-minute timeout configurable from 1 second through 4 hours. On cancellation, send a graceful termination signal and force-kill the process group after a 2-second grace period.
- **FR-018**: The system MUST persist state transition and associated event records atomically, allocate monotonic event positions per run, and recover ambiguous running work as interrupted.
- **FR-019**: The system MUST support safe pause boundaries, resume validation, graceful cancellation followed by bounded termination, and crash recovery without duplicate run creation or conflicting workspace writes. Live provider work pauses only between adapter invocations, after the current stage result and evidence are persisted; SpecTide does not suspend a provider process. The deterministic fake may pause during its simulated wait while preserving the remaining wait duration. Before resuming after interruption, verify canonical workspace identity, acquire its exclusive writer lock, match the immutable effective run-configuration hash and checkpoint identifier, verify every checkpointed artifact path and content hash, and compare the current Git HEAD and managed-path hash set with the checkpoint. If any check differs, do not resume or overwrite; preserve evidence, report `WORKSPACE_DRIFT`, and require explicit recovery through a new verified checkpoint or a new run.
- **FR-020**: The system MUST expose a loopback-only local HTTP interface for health, readiness, version, projects, runs, stages, safe artifact metadata, reports, profiles, capabilities, configuration validation, approvals, and permitted run controls.
- **FR-021**: The system MUST stream ordered live run events, replay events after a supplied cursor, provide reconnect-safe behavior and heartbeat signaling, and ensure slow viewers cannot block workflow execution.
- **FR-022**: The system MUST protect browser mutations using host and origin validation, anti-CSRF controls, input limits, path validation, standardized errors, and local-only binding.
- **FR-023**: The system MUST redact sensitive values before event persistence, streaming, export, and display, and MUST avoid exposing provider hidden reasoning or authentication material.
- **FR-024**: The dashboard MUST render persisted API state rather than hardcoded workflow status and show current stage, route, attempt history, live activity, approvals, quality evidence, artifact metadata, reports, and permitted controls.
- **FR-025**: The dashboard MUST allow a user to preview resolved routing and dry-run intent before a real run and clearly distinguish disconnected, blocked, skipped, failed, not-run, and simulated states.
- **FR-026**: The system MUST provide accessible desktop-first monitoring with keyboard-operable controls, a visible keyboard focus indicator, body-text contrast of at least 4.5:1, no loss of content or control at 200% zoom, and reduced-motion-safe presentation. Every status MUST include a text label or symbol in addition to color. Automated accessibility scans MUST report zero critical or serious violations on the run-list, run-detail, route-preview, profile-readiness, empty, error, and disconnected dashboard surfaces. Record the page/state matrix and scan results in `docs/accessibility-audit.md`.

  The required page/state pairs are: run-list—empty, disconnected, running, paused, blocked, completed, cancelled, and simulated; run-detail—running, paused, blocked, completed, cancelled, failed, skipped, not-run, and simulated; route-preview—ready, blocked, and disconnected; profile-readiness—ready, disconnected, and error; shared empty surface—empty; shared error surface—error; shared disconnected surface—disconnected. Status-state fixtures MUST include distinct text or symbol labels for disconnected, blocked, skipped, failed, not-run, and simulated, even where a state appears on more than one surface.
- **FR-027**: The system MUST maintain versioned local data migrations, safe schema upgrades, backup-aware recovery, bounded retention, and sanitized report export. By default, expire individual event payloads and activity details 90 days after their persisted timestamps; independently cap detailed history at 1,000 runs by pruning details from the oldest terminal runs first. Compact run summaries are retained indefinitely and do not count toward the detailed-history cap. Never prune active runs; preserve each summary, stage outcomes, routing decisions, quality verdicts, approvals, and sanitized report metadata. If every detailed-history slot belongs to an active run, reject new run creation with an actionable retention-cap error until a slot becomes available.
- **FR-028**: The system MUST provide documented build, formatting, lint, unit, integration, end-to-end, smoke, and security verification commands that work without live provider credentials where simulations suffice.
- **FR-029**: The system MUST include automated coverage for workflow transitions, routing and boundaries, persistence and recovery, subprocess safety, adapter stream compatibility, API security, event replay, dashboard interaction, and simulated complete lifecycle scenarios.
- **FR-030**: The system MUST include local-first installation, configuration, architecture, provider, security, recovery, troubleshooting, API, contribution, license, and release-readiness documentation that makes only verified claims.

### Key Entities

- **Project**: An approved canonical local repository with a display name and data-boundary policy.
- **Profile**: A named provider execution context with an approved executable/configuration reference, allowed projects, and sanitized capability status.
- **Run**: Immutable user intent and frozen effective routing/configuration together with the mutable lifecycle summary.
- **Stage Attempt**: One historical execution of a stage, including iteration, route, timing, process identity, semantic outcome, and evidence.
- **Routing Decision**: The auditable policy inputs, selected route, fallback chain, and reason for a stage.
- **Event**: An ordered, redacted observable occurrence belonging to a run and optionally a stage attempt.
- **Artifact**: Safe metadata and integrity information for a workspace-bound output observed during a run.
- **Quality Gate**: A typed verdict and evidence links for a required acceptance, verification, security, analysis, convergence, or review condition.
- **Approval**: A pending or resolved authorization for a sensitive permitted action.
- **Checkpoint and Workspace Lock**: Durable recovery state and exclusive writer ownership for a canonical workspace.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On the reference validation class (arm64 macOS 15.1 or later, local SSD; record CPU model and installed RAM in the verification report), a contributor can build and start the local application and receive a successful `/healthz` response within 2 seconds of process launch. Measure with a monotonic clock from process start until the first HTTP 200 response; report the exact OS, CPU, RAM, build command, and elapsed time.
- **SC-002**: An offline simulated full workflow completes with every required simulated gate evidenced, or reports a specific blocked or failed reason, in 100% of 20 deterministic repeated test runs.
- **SC-003**: In automated scenarios, 100% of terminal run reports distinguish required checks as pass, fail, blocked, or not run, with no unsupported live-provider result reported as a pass.
- **SC-004**: In automated multi-viewer and reconnect tests, viewers recover the same canonical run state and receive ordered post-cursor events with no missing persisted transition events.
- **SC-005**: In automated policy tests, 100% of attempted unauthorized profile/workspace routes, cross-origin mutations, path escapes, and synthetic-secret exposures are rejected or redacted as applicable.
- **SC-006**: At least 95% of locally generated simulated events appear in a healthy connected dashboard within 1 second under this reproducible moderate-load profile: one local server, 10 concurrent SSE clients, 100 events per second for 100 seconds, and a 10,000-event persisted backlog. Measure from the event's persisted UTC timestamp to receipt of its SSE frame at each client; calculate p95 over all delivered frames after excluding connection setup and heartbeat frames. A client missing persisted events fails the scenario regardless of latency.
- **SC-007**: A developer can complete the documented offline demo, observe its report, and identify that it is simulated without a provider login or manual database intervention.
- **SC-008**: The published verification suite covers every v1 functional requirement, FR-001 through FR-030, with one or more linked automated tests or documented inspected evidence. All listed requirements are release acceptance requirements; no undefined P0 subset is implied.

### Release priority

Every functional requirement FR-001 through FR-030 and every success criterion SC-001 through SC-008 is required for v1 release acceptance. Story priorities describe implementation order, not permission to omit requirements from the final acceptance set.

## Assumptions

- v1 targets local developer-controlled macOS and Linux environments first; platform-specific limitations are documented rather than hidden.
- Live provider credentials and authorization may be unavailable during automated verification; fixture contracts and the simulated adapter are the required offline baseline, while live smoke checks remain not run until authorized.
- Official Spec-Kit and provider command syntax can change; the product validates discovered local capabilities and reports incompatibility instead of assuming undocumented flags.
- The dashboard is local, same-origin, and desktop-first; it is not a hosted multi-user service or remote execution console.
- A Git repository is the normal workspace transport; existing user changes are preserved and reported rather than destructively reset.
- Sensitive actions that policy permits require explicit local approval; publishing, remote deployment, and credential management remain outside v1's automated authority.
- The initial release prepares binaries and release workflows but does not publish code, releases, or packages without explicit owner authorization.
