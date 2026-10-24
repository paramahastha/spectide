# Bug Verification: Next-phase scope and invocation visibility

- **Slug**: clarify-model-visibility
- **Tested**: 2026-10-24
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: partial

## Summary

The automated checks for next-feature selection, intent classification, Codex usage extraction, UI model labeling, and frontend behavior pass. I did not run the original live Craftlane/Codex reproduction, and the broader Go regression command still fails in this sandbox when workspace ownership/locks and a local test listener are exercised, so the full fix is not verified end-to-end.

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | Reviewed assessment's Craftlane run `7dc29766`; no live Craftlane project/provider session available from this workspace. | not-run | Automated equivalents below exercise intent selection and slug safety, but do not create a real next-phase spec or re-run Clarify in Codex. |
| New / updated tests | `GOCACHE=/tmp/spectide-gocache go test ./internal/speckit -run 'TestSelectNextFeatureDirectoryRequiresUnusedSafeSlug|TestSelectFeatureDirectoryHonorsExplicitActiveFeature'` | pass | Confirms new scope is distinct from the selected feature and rejects unsafe/colliding slugs. |
| New / updated tests | `GOCACHE=/tmp/spectide-gocache go test ./internal/workflow -run 'TestCodexUsageEvidence|TestSpecificationIntentSeparatesNextFeatureFromAmendment|TestSpecifyAmendmentIntentRunsDespiteExistingSpecification'` | pass | Covers explicit amendment vs next-feature intent and safe Codex terminal usage parsing. |
| New / updated tests | `npm test -- -t 'labels an omitted Codex model|shows only the sanitized resolved Codex model'` | pass | 2 dashboard tests passed. |
| Regression suite | `npm test` | pass | All 66 tests passed. |
| Regression suite | `GOCACHE=/tmp/spectide-gocache go test ./internal/speckit ./internal/workflow ./internal/api ./internal/adapters/...` | fail | `speckit` and adapter packages passed. Workflow/API integration tests failed with sandbox `operation not permitted` errors for workspace ownership/locks and local listener binding. |
| Compile check | `GOCACHE=/tmp/spectide-gocache go test ./internal/api -run '^$'` | pass | API package compiled; no API tests were run by this command. |
| Lint / type-check | `npm run typecheck` | pass | TypeScript check passed. |
| Build | `npm run build` | pass | Vite built the dashboard and embedded assets. |

## Output Excerpts

- Targeted Go tests: `internal/speckit` and `internal/workflow` passed.
- Frontend suite: `Test Files 3 passed (3)`; `Tests 66 passed (66)`.
- Broader Go suite: workflow failures include `WORKSPACE_OWNER_ACTIVE: operation not permitted`; API's SSE test could not bind `tcp6 [::1]:0` (`operation not permitted`).

## Residual Risks

- The user's original live request and Codex `clarify_format_repair` sequence were not replayed; outcome is not verified against that external run.
- Go tests that rely on workspace locking and local listener creation need to be rerun in an environment with those permissions.
- The next-feature scope has focused unit coverage but lacks the assessment-requested end-to-end run/resume assertion that the old spec stays unchanged and the new spec is created.
- Clarify repair diagnostics do not yet have real-shaped Codex JSONL integration coverage; rate-limit snapshots are not parsed.

## Recommendation

Hold as **partial**. Run the next-phase scenario with a completed active feature and a new slug in an environment with a configured live provider; confirm the old spec remains unchanged, the new scope receives the artifacts, and a failed Clarify repair ends BLOCKED with the diagnostic event. Rerun the broader Go suite outside the restricted sandbox before closing the bug.
