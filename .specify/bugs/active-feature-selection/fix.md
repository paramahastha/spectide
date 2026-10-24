# Bug Fix: Honor the active Spec-Kit feature selector

- **Slug**: active-feature-selection
- **Fixed**: 2026-10-24
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

Spectide now validates and honors `.specify/feature.json` before scanning
`specs/*`, allowing Craftlane's selected feature scope to start a live workflow
despite other feature directories. The route preview now presents stage routing
as responsive cards rather than a constrained three-column text grid.

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/speckit/parse.go` | modified | Validates the explicit direct-child `specs/<feature>` selector; rejects malformed, unavailable, nested, escaping, and symlink paths; preserves no-selector ambiguity behavior. |
| `internal/speckit/parse_test.go` | modified | Covers valid explicit selection, malformed JSON, missing selector field, traversal, nested paths, unavailable directories, symlink selection, and no-selector ambiguity. |
| `web/src/App.tsx` | modified | Gives each previewed stage a semantic route card and named decision list. |
| `web/src/style.css` | modified | Adds responsive route-card layout and preserves the established dark local-control visual language. |
| `web/src/accessibility.test.tsx` | modified | Pins the named stage-decision list and stage heading. |
| `web/e2e/dashboard.spec.ts` | modified | Updates the route-preview structure assertion. |
| `internal/webassets/static/` | regenerated | Embeds the rebuilt dashboard bundle. |
| `docs/screenshots/*.png` | regenerated | Refreshes dashboard evidence from the browser test, including route preview. |

## Tests Added or Updated

- `internal/speckit/parse_test.go::TestSelectFeatureDirectoryHonorsExplicitActiveFeature` — explicit selection survives multiple artifact scopes and later validation.
- `internal/speckit/parse_test.go::TestDiscoverFeatureDirectoryHonorsExplicitActiveFeature` — complete-spec discovery follows the same explicit selector.
- `internal/speckit/parse_test.go::TestSelectFeatureDirectoryRejectsInvalidExplicitSelectors` — safe typed failures for invalid selector forms.
- `internal/speckit/parse_test.go::TestSelectFeatureDirectoryRejectsExplicitSelectorSymlink` — blocks a symlink selector.
- `internal/speckit/parse_test.go::TestSelectFeatureDirectoryRetainsNoSelectorAmbiguity` — retains fail-closed legacy behavior.
- `web/src/accessibility.test.tsx` route-preview state test — preserves semantic UI affordances.

## Local Verification

- Commands run: `go test ./internal/speckit` → PASS.
- Commands run: `go test ./...` → PASS.
- Commands run: `go test -count=1 ./internal/workflow ./internal/workspace` → PASS.
- Commands run: `cd web && npm test -- --run` → PASS (64 tests).
- Commands run: `cd web && npm run build` → PASS.
- Commands run: `cd web && npm run test:e2e -- --grep "ready route preview"` → PASS (browser, mocked local API, 1 test).
- Manual checks: inspected the refreshed `docs/screenshots/route-preview-desktop.png`; all twelve routes are readable as four-column cards with no narrow route metadata column.

## Deviations from Assessment

None. Checkpoint persistence already stored a canonical relative
`feature_directory`; this fix feeds the explicit selected scope into that
existing, tested checkpoint and workspace-drift path rather than changing its
format.

## Follow-ups

- Restart any already-running Spectide server before testing Craftlane manually,
  so it loads the rebuilt embedded dashboard and selector implementation.
