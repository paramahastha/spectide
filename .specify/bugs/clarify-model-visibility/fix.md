# Bug Fix: Next-phase scope and invocation visibility

- **Slug**: clarify-model-visibility
- **Fixed**: 2026-10-24
- **Assessment**: ./assessment.md
- **Status**: partial

## Summary

Next-phase wording now creates a separately named feature scope and cannot silently amend the active feature. Clarify repair failures leave safe diagnostic evidence and end in a blocked state. Codex invocation history labels unreported defaults honestly and displays resolved model/token fields only when a sanitized terminal event reports them.

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/workflow/effective_config.go` | modified | Distinguishes continue, explicit amendment, and next-feature intent; stores and validates immutable scope. |
| `internal/workflow/engine.go` | modified | Requires a new slug for next-phase requests, selects the new directory, and persists its scope before execution. |
| `internal/speckit/parse.go` | modified | Validates safe slugs, rejects collisions, and constrains a new scope to a direct child of `specs/`. |
| `internal/api/server.go` | modified | Accepts `feature_slug` on run creation. |
| `internal/workflow/live_stage.go` | modified | Records sanitized Codex usage evidence and a safe Clarify repair diagnostic. |
| `internal/workflow/codex_usage.go` | added | Extracts only validated resolved model and input/output token counts from Codex `turn.completed` JSON. |
| `web/src/App.tsx` | modified | Requests a slug for next-phase prompts, displays truthful default labels and reported usage, and renders safe repair diagnostics. |
| `web/src/domain.ts` | modified | Exposes persisted attempt evidence JSON to the dashboard. |
| `internal/speckit/parse_test.go` | modified | Covers safe, collision-free next-feature selection with an existing active feature. |
| `internal/workflow/artifact_lifecycle_test.go` | modified | Pins separate next-feature and explicit amendment intent. |
| `internal/workflow/codex_usage_test.go` | added | Checks extraction from real-shaped Codex terminal events and rejects unsafe/invalid values. |
| `web/src/accessibility.test.tsx` | modified | Covers unreported defaults and sanitized resolved-model/token display. |
| `internal/webassets/static/index.html`, `internal/webassets/static/assets/*` | rebuilt | Updated embedded production dashboard bundle. |

## Diff Highlights

- A next-phase prompt requires a user-entered slug. The slug is checked for safe characters and collisions, then persisted as `specs/<slug>` in immutable run configuration and the checkpoint.
- Explicit amendment wording retains the existing selected-feature amendment path.
- If Clarify's bounded repair still produces neither an artifact change nor valid no-op evidence, the run becomes BLOCKED and records a safe error code and recovery action.
- Codex terminal evidence stores only a safe model identifier and nonnegative integer input/output token counts. Raw provider output and unrelated event fields are not persisted in that evidence.

## Tests Added or Updated

- `internal/speckit/parse_test.go::TestSelectNextFeatureDirectoryRequiresUnusedSafeSlug` — checks direct-child path selection, invalid slugs, and collisions.
- `internal/workflow/artifact_lifecycle_test.go::TestSpecificationIntentSeparatesNextFeatureFromAmendment` — checks next-feature and explicit amendment classification.
- `internal/workflow/codex_usage_test.go::TestCodexUsageEvidenceKeepsOnlySafeTerminalFields` — verifies terminal model/token extraction and omission of unrelated fields.
- `internal/workflow/codex_usage_test.go::TestCodexUsageEvidenceOmitsAbsentAndUnsafeValues` — rejects unsafe model identifiers and invalid token counts.
- `web/src/accessibility.test.tsx` — verifies truthful Codex default labels and reported usage display without leaking unrelated evidence.

## Local Verification

- `GOCACHE=/tmp/spectide-gocache go test ./internal/speckit -run 'TestSelectNextFeatureDirectoryRequiresUnusedSafeSlug|TestSelectFeatureDirectoryHonorsExplicitActiveFeature'` → passed.
- `GOCACHE=/tmp/spectide-gocache go test ./internal/workflow -run 'TestCodexUsageEvidence|TestSpecificationIntentSeparatesNextFeatureFromAmendment|TestSpecifyAmendmentIntentRunsDespiteExistingSpecification'` → passed.
- `GOCACHE=/tmp/spectide-gocache go test ./internal/api -run '^$'` → package compiled successfully.
- `npm test -- -t 'labels an omitted Codex model|shows only the sanitized resolved Codex model'` → 2 targeted tests passed.
- `npm run typecheck` → passed.
- `npm run build` → passed and regenerated embedded dashboard assets.
- `GOCACHE=/tmp/spectide-gocache go test ./internal/speckit ./internal/workflow ./internal/api ./internal/adapters/...` → speckit and adapter packages passed; broader workflow tests hit sandbox `operation not permitted` errors around workspace ownership/locks, and an API test could not bind its local listener. The command did not pass as a whole.
- `npm test` → 64 passed, 1 failed. `shows a route-preview transport failure and keeps the selected project and task` did not render the expected disconnected route-preview surface.

## Deviations from Assessment

- This is **partial** because the assessment's tests for an end-to-end next-feature run/resume and real-shaped Clarify repair success/failure are not yet present. Existing focused tests cover scope selection, intent classification, usage parsing, and the relevant dashboard fields.
- Resolved model and input/output token fields are stored in the existing sanitized attempt `evidence_json`; no migration was needed. A Codex rate-limit snapshot is not parsed yet.
- The UI requires the user to type the new slug when the prompt includes “next phase” or “new phase”; it does not propose a slug automatically. Account-level billing remains outside dashboard evidence and is not inferred.

## Follow-ups

- Add an end-to-end run/resume test that verifies a completed active feature remains unchanged while the new scope receives Specify and subsequent artifacts.
- Add realistic Codex JSONL tests for Clarify repair responses with valid and invalid no-op evidence, including safe blocked-state diagnostics.
- Parse and show a rate-limit snapshot only if a trustworthy Codex event contract is available.
- Re-run the full Go and UI suites in an environment that permits the test workspace locks and local listener; investigate the remaining route-preview UI failure.
