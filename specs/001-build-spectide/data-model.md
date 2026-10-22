# Data Model

## Persistent entities

| Entity | Required fields | Rules |
| --- | --- | --- |
| Project | id, canonical path, boundary | Canonical path is unique and must stay inside allowed workspace roots. |
| Run | id, project, prompt, frozen configuration, state, timestamps | Intent/configuration are immutable; terminal states have a reason and evidence. |
| StageAttempt | id, run, key, iteration, attempt, route, state, timing | Historical attempts are never overwritten; state transitions are centrally validated. |
| Event | run, monotonically increasing sequence, timestamp, type, redacted payload | Append-only and written in the same transaction as its state transition. |
| Profile | id, provider, executable reference, config reference, authorization scope | Credentials are never persisted. |
| QualityGate | run, key, verdict, evidence | Required gate must be `PASS` for real completion; `NOT_RUN` remains visible. |
| Approval | run, action, reason, status, timestamps | Pending sensitive work fails closed. |
| Artifact/Checkpoint/Lock | run/workspace, safe metadata, integrity data | Paths remain workspace confined; lock admits one active writer. |

## State transitions

`CREATED → RUNNING → {PAUSED, BLOCKED, FAILED, CANCELLED, COMPLETED}`. Interrupted process work is recoverable only after artifact and process identity validation. Stage attempts progress from `PENDING` to `RUNNING` to a terminal semantic result; an exit status alone is insufficient evidence.
