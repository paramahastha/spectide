# Research: Build SpecTide v1

## Decisions

### Local durable state
- **Decision**: Use a local SQLite database with WAL and transactional event/state updates.
- **Rationale**: It is a single-file local dependency, supports concurrent dashboard reads, and creates inspectable recovery evidence.
- **Alternatives considered**: In-memory state loses crash recovery; a hosted service violates local-first operation.

### Workflow authority
- **Decision**: SpecTide owns run scheduling and invokes verified individual Spec-Kit stages.
- **Rationale**: Per-stage profile routing, safety policy, and normalized events require one authoritative scheduler.
- **Alternatives considered**: Competing upstream workflow ownership risks duplicate worktree writes and ambiguous status.

### Provider verification
- **Decision**: Ship deterministic fake-provider fixtures as required offline acceptance; probe live CLIs before command generation.
- **Rationale**: CI and fresh installs cannot require paid credentials, while live capability must never be fabricated.
- **Alternatives considered**: Mock-only adapters would not validate invocation contracts; hard-coded CLI options rot across versions.

### Event transport
- **Decision**: Persist redacted events and expose cursor-based SSE replay with bounded consumers.
- **Rationale**: Browser-native reconnect behavior fits a localhost monitor and persists canonical ordering.
- **Alternatives considered**: WebSockets add bidirectional complexity without a user need; polling alone is not sufficiently live.

### Dashboard delivery
- **Decision**: Author dashboard in React/TypeScript and embed its production assets into the local binary.
- **Rationale**: A single local process avoids deployment dependencies while retaining testable UI composition.
- **Alternatives considered**: Server-rendered pages reduce interactivity; a separately hosted frontend violates simple local setup.
