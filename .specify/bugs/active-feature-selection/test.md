# Bug Verification: Active feature selector is honored

- **Slug**: active-feature-selection
- **Tested**: 2026-10-24
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

The post-fix automated reproduction selects the explicit active feature in a
multi-feature workspace rather than producing `SPECIFICATION_AMBIGUOUS`.
Selector safety failures, selector-free ambiguity, workflow regressions, and
the route-preview accessibility surface all remain covered and passing.

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | `go test -count=1 ./internal/speckit -run 'Test(SelectFeatureDirectoryHonorsExplicitActiveFeature\|DiscoverFeatureDirectoryHonorsExplicitActiveFeature\|SelectFeatureDirectoryRejectsInvalidExplicitSelectors\|SelectFeatureDirectoryRejectsExplicitSelectorSymlink\|SelectFeatureDirectoryRetainsNoSelectorAmbiguity)$'` | pass | Recreates a multi-feature project, selects `specs/006-selected`, and confirms safe invalid-selector and no-selector behavior. |
| Workflow regression | `go test -count=1 ./internal/workflow ./internal/workspace` | pass | Confirms checkpoint, stage execution, and workspace behavior remain sound. |
| Full Go suite | `go test ./...` | pass | All project Go packages, including `internal/workflow` and `internal/workspace`, passed. |
| Frontend tests | `cd web && npm test -- --run` | pass | 64 tests passed, including route-preview semantics. |
| Type check | `cd web && npm run typecheck` | pass | TypeScript completed with no errors. |
| Browser route preview | `cd web && npm run test:e2e -- --grep "ready route preview"` | pass | Prior final-fix check passed with mocked local API and validated the 12-card route preview. |

## Output Excerpts

```text
ok   github.com/spectide/spectide/internal/speckit  0.176s

Test Files  3 passed (3)
Tests       64 passed (64)

1 passed (2.5s)  # focused Playwright route-preview test
```

## Residual Risks

- The live Craftlane provider workflow was not launched during verification:
  that would invoke the user's authenticated provider and may mutate Craftlane.
  The pre-provider selection path is exercised by the automated reproduction.
- Any already-running Spectide process must be restarted before manual
  Craftlane verification so it loads the rebuilt selector implementation.

## Recommendation

Close the bug as verified. The failure mode is reproduced and resolved by the
automated selector and workflow checks, while invalid selectors still fail
closed. Restart the local SpecTide server before a manual Craftlane run; a new
live workflow should retain `specs/006-threejs-workspace` in its checkpoint.
