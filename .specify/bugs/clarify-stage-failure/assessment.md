# Bug Assessment: Existing feature skips Specify and fails Clarify

- **Slug**: clarify-stage-failure
- **Created**: 2026-10-24
- **Source**: pasted text and screenshots
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

The user started a live Craftlane workflow with the prompt:

> please analyze and implement spec based on this plan for next phase /Users/macbookprom4/Projects/craftlane/craftlane-plan-go.md

The dashboard shows Constitution and Specify as `SKIPPED_WITH_REASON`, then Clarify as `FAILED`; the run ends `FAILED` before quality gates run. The user expects Specify to run first because the request supplies a plan for a new Craftlane phase.

Persisted evidence for run `a4bc58fd-22f0-435b-af23-cf2b5a506e0b` confirms that `specify` was skipped solely because its selected feature already contains `specs/006-threejs-workspace/spec.md`. The provider process then exited Clarify with code `0` after 21,397 ms but the attempt was recorded as `FAILED` with `STAGE_EXPLICIT_NO_OP_REQUIRED: SPECTIDE_NO_CLARIFICATION`. No Clarify artifact was registered.

## Symptom

A live workflow for a new phase against an existing Craftlane feature does not run Specify: SpecTide preserves the prior `spec.md` unconditionally and moves to Clarify. It then stops at Clarify even though Codex completed normally, because a no-change response lacks the exact no-op marker. The result is that the provided next-phase plan is neither incorporated into the specification nor implemented.

## Reproduction

1. Register `/Users/macbookprom4/Projects/craftlane` with a live Codex profile and select the existing `006-threejs-workspace` feature.
2. Start a run that asks to analyze and implement the supplied next-phase plan.
3. Observe Constitution and Specify skip because their owner artifacts already exist, including `specs/006-threejs-workspace/spec.md`.
4. Have the Clarify provider exit successfully without changing `spec.md` and without outputting one standalone `SPECTIDE_NO_CLARIFICATION: <reason>` line.
5. Observe `clarify` and the entire run become `FAILED` with `STAGE_EXPLICIT_NO_OP_REQUIRED: SPECTIDE_NO_CLARIFICATION`.

## Suspected Code Paths

- `internal/speckit/invoke.go:151` — appends the required Clarify no-op marker instruction to the provider prompt.
- `internal/workflow/artifact_lifecycle.go:13` — makes a present selected `spec.md` sufficient to suppress the Specify command, without considering whether the run requests a specification amendment/new phase.
- `internal/workflow/engine.go:936` — marks a create-once owner stage as `SKIPPED_WITH_REASON` whenever that artifact exists; the persisted run's `stage.owner_artifact_present` event confirms this branch for Specify.
- `internal/workflow/live_stage.go:316` — interprets an unchanged Clarify artifact set as a no-op and requires the marker.
- `internal/workflow/stage_semantics.go:17` — rejects output unless it contains exactly one, unindented, non-fenced marker with a 12-character reason.
- `internal/workflow/engine.go:1183` — persists any stage error as a failed stage and terminates the live run; it provides no marker-specific corrective retry or blocked state.
- `internal/speckit/invoke_test.go:74` and `internal/workflow/stage_semantics_test.go:64` — cover prompt injection and validation primitives, but not a live provider no-op response that omits the marker.

## Root Cause Hypothesis

Confidence: high. `existingOwnerArtifact` treats any existing `spec.md` in the selected feature as proof that Specify must never run. That implements a create-once safety policy, but it is incorrectly applied to an explicit request to evolve the existing feature from a next-phase plan. The workflow therefore advances to Clarify without giving the provider a specification-update stage. The provider completed Clarify successfully (`exit_code=0`) and no artifact change was detected, so `runLiveStage` classified the outcome as a no-op. `requireExplicitNoop` then rejected its terminal text because it did not meet the exact sentinel format. The two policies compound: the intended update is skipped, then the fallback stage has no bounded recovery for a normal model-format deviation.

## Proposed Remediation

**Preferred**: Add an explicit stage intent for updating an existing feature (for example, a `new phase` / `amend specification` intent derived at route preview and persisted in immutable run configuration). For that intent, do not skip Specify merely because `spec.md` exists; invoke it with a constrained amendment prompt that requires updating only the selected feature's `spec.md`, preserving its identity and prior requirements. Keep the current skip behavior for a continuation/resume intent where the specification is already accepted.

Also preserve the explicit no-op evidence requirement, but treat a missing Clarify marker after a successful, unchanged invocation as a bounded corrective retry. Reinvoke Clarify once with a concise format-repair prompt that demands either an update to the selected `spec.md`, exactly one marker line, or a structured clarification-needed result that pauses/blocks with questions surfaced to the user. Persist both attempts and the corrective reason.

**Alternatives**:
- Relax `requireExplicitNoop` to infer no-op success from unchanged artifacts and a valid existing specification. This reduces false failures but weakens auditable evidence and can silently accept an incomplete Clarify response.
- Keep current execution semantics but expose the attempt error and a retry action in the dashboard. This improves diagnosis but still requires a manual rerun for a recoverable output-format error.

**Files likely to change**:

- `internal/workflow/live_stage.go`
- `internal/workflow/engine.go`
- `internal/workflow/artifact_lifecycle.go`
- `internal/workflow/effective_config.go`
- `internal/workflow/stage_semantics.go`
- `internal/workflow/artifact_lifecycle_test.go`
- `internal/workflow/live_stage_test.go`
- `internal/workflow/stage_semantics_test.go`
- `web/src/App.tsx` (if adding a focused remediation action or error presentation)

**Tests to add or update**:

- A live Clarify test where the first successful provider response makes no artifact change and omits the marker, then a corrective retry supplies a valid marker and the stage succeeds.
- A live existing-feature, next-phase run test showing that Specify executes and may amend the selected `spec.md`, instead of being skipped only because it exists.
- A continuation/resume run test showing that existing-artifact preservation remains in force when no specification-amendment intent is requested.
- A test where both bounded attempts omit valid evidence and the terminal state and persisted reason match the selected blocked/failure policy.
- A test where the provider writes a valid selected `spec.md` update without a no-op marker and succeeds without retry.
- A dashboard/API test that exposes the persisted attempt error and the corrective action or blocking reason.

## Risks & Considerations

- A retry must be strictly bounded to avoid infinite provider loops and must be persisted as a separate execution attempt.
- Allowing Specify to amend an existing feature must not allow it to create a second feature directory, replace the Constitution, or silently change an unrelated feature.
- Do not automatically infer that no clarification is required merely from unchanged files; that would weaken the workflow's evidence contract.
- Corrective prompts must remain scoped to the selected feature directory and preserve the existing workspace-write authorization model.
- The provider's raw terminal text is intentionally not persisted; remediation must rely on safe summaries, hashes, and explicit attempt/error evidence.

## Open Questions

- [NEEDS CLARIFICATION: Should an unresolved Clarify question pause/block the run for user input, or should SpecTide record a documented assumption and continue?]
- [NEEDS CLARIFICATION: Is one corrective provider retry acceptable for all live profiles, or should it be configurable per profile/stage?]
- [NEEDS CLARIFICATION: Should the dashboard expose an explicit `New phase / amend specification` control, or should a plan-path prompt automatically select that intent?]
