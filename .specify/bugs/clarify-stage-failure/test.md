# Bug Verification: Existing feature skips Specify and fails Clarify

- **Slug**: clarify-stage-failure
- **Tested**: 2026-10-24
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: partial

## Summary

The targeted tests for amendment intent and the one-time Clarify format-repair retry pass, and the full workflow package suite passes. The reported live Craftlane workflow was not run, so the fix is not verified end-to-end.

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | Start the reported live run in `/Users/macbookprom4/Projects/craftlane` with a configured Codex profile and the existing `006-threejs-workspace` feature | skipped | Requires a live provider profile and executes a workflow that can modify Craftlane artifacts; no live run was started. |
| New / updated tests | `go test ./internal/workflow -run 'TestSpecifyAmendmentIntentRunsDespiteExistingSpecification|TestSpecificationIntentRecognizesPhaseAmendmentPrompts|TestClarifyRetriesOnceForMissingNoopEvidence' -count=1` | pass | `ok github.com/spectide/spectide/internal/workflow 0.201s` |
| Regression suite | `go test ./internal/workflow` | pass | `ok github.com/spectide/spectide/internal/workflow 64.620s` |
| Lint / type-check | `go vet ./internal/workflow`; `git diff --check` | pass | Both exited 0 with no output. |

## Output Excerpts

```text
ok  github.com/spectide/spectide/internal/workflow  0.201s
ok  github.com/spectide/spectide/internal/workflow  64.620s
```

`go vet ./internal/workflow` and `git diff --check` exited 0.

## Residual Risks

- No live run confirmed that an existing selected feature's `spec.md` is amended from the supplied next-phase plan.
- The automated Clarify test covers a successful repair on the second attempt. A second attempt without valid evidence still follows the existing stage-failure path.
- The assessment's structured clarification-needed pause/block behavior remains unimplemented, as recorded in `fix.md`.

## Recommendation

Hold for an end-to-end run with a configured live profile and a controlled Craftlane workspace. The automated checks support the code-level fix, but the original live symptom has not been exercised after the change.
