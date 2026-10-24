# Bug Fix: Existing feature skips Specify and fails Clarify

- **Slug**: clarify-stage-failure
- **Fixed**: 2026-10-24
- **Assessment**: ./assessment.md
- **Status**: partial

## Summary

Added a persisted specification intent inferred from explicit phase/amendment wording, allowing Specify to update the selected feature's existing spec while preserving create-once behavior for continuations. Clarify now records one bounded corrective retry when a successful unchanged invocation omits its required no-op marker.

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/workflow/effective_config.go` | modified | Added amendment/continuation intent detection and immutable configuration validation. |
| `internal/workflow/artifact_lifecycle.go` | modified | Allows Specify to run on an existing spec when amendment intent is explicit. |
| `internal/workflow/engine.go` | modified | Persists intent at run creation, restores it on resume, and constrains amendment prompts to the selected spec. |
| `internal/workflow/live_stage.go` | modified | Persists a corrective-retry event and runs Clarify once more under a separate attempt phase. |
| `internal/workflow/artifact_lifecycle_test.go` | modified | Covers continuation preservation, amendment eligibility, and phase wording detection. |
| `internal/workflow/live_stage_test.go` | modified | Covers a missing Clarify marker followed by a valid marker on the one corrective attempt. |

## Tests Added or Updated

- `internal/workflow/artifact_lifecycle_test.go::TestSpecifyAmendmentIntentRunsDespiteExistingSpecification` — confirms continuation skips an existing spec and amendment intent schedules Specify.
- `internal/workflow/artifact_lifecycle_test.go::TestSpecificationIntentRecognizesPhaseAmendmentPrompts` — covers the reported “next phase” wording and continuation intent.
- `internal/workflow/live_stage_test.go::TestClarifyRetriesOnceForMissingNoopEvidence` — confirms the first failed-evidence attempt and successful repair are separately persisted and the retry prompt is constrained.

## Local Verification

- Commands run: `go test ./internal/workflow` → passed (67.565s).
- Commands run: `go test ./internal/workflow -run 'TestSpecifyAmendmentIntentRunsDespiteExistingSpecification|TestSpecificationIntentRecognizesPhaseAmendmentPrompts|TestClarifyRetriesOnceForMissingNoopEvidence' -count=1` → passed.
- Commands run: `git diff --check` → passed.
- Manual checks: confirmed `.specify/extensions.yml` has no hooks registered under `hooks.before_bug_fix` or `hooks.after_bug_fix`.

## Deviations from Assessment

- A second Clarify attempt that still lacks valid evidence follows the existing persisted stage-failure path. A structured clarification-needed result that pauses/blocks the run and surfaces questions is not implemented; the assessment's open question about that user-input policy remains unresolved.
- No dashboard/API remediation action was added, so the assessment's suggested dashboard/API exposure test was not applicable to this code change.
- Regression coverage verifies amendment eligibility and prompt intent, but does not run a full live workflow through Specify against an existing feature.

## Follow-ups

- Consider adding a structured Clarify questions result and a blocked state that surfaces its questions to the user.
- Add an end-to-end live workflow test proving Specify amends only the selected feature's existing `spec.md`.
- Run `$speckit-bug-test slug=clarify-stage-failure`.
