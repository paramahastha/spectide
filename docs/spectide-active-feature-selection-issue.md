# Issue: Honor the active Spec-Kit feature when selecting a Spectide workflow scope

## Summary

Spectide cannot start a live workflow for Craftlane because the repository has
multiple valid feature directories beneath `specs/`. Craftlane already declares
the active feature in `.specify/feature.json`, but Spectide's feature discovery
does not use that declaration.

## Observed behavior

Running:

```sh
bin/spectide run "Describe the Craftlane change you want to build"
```

creates a durable `LIVE` run that immediately ends `BLOCKED` before invoking
Codex. The persisted gate evidence is:

```text
SPECIFICATION_AMBIGUOUS: found 6 feature artifact directories
```

The affected run is `5ae24fa9-73d4-4725-8c7d-9a2c742097bb`.

## Expected behavior

When a project contains `.specify/feature.json` with a valid
`feature_directory`, Spectide should select that canonical directory as the
single feature scope for the run. For Craftlane this is:

```text
specs/006-threejs-workspace
```

Spectide must continue to fail closed when the selector is missing, malformed,
escapes the project root, or points to a missing feature directory.

## Proposed change

Update Spectide feature discovery to:

1. Read and validate `.specify/feature.json` before scanning `specs/*`.
2. Use its `feature_directory` when it is a canonical, project-contained,
   direct child of `specs/`.
3. Preserve the current ambiguity checks when no valid explicit selector is
   present.
4. Persist the selected directory in the existing run checkpoint and retain
   current workspace-drift validation.

## Acceptance criteria

- [ ] A project with multiple `specs/*` feature directories and a valid
  `.specify/feature.json` starts using the selected feature directory.
- [ ] The selected scope is included in every Spec-Kit stage prompt and run
  checkpoint.
- [ ] Missing, malformed, out-of-root, nested, or unavailable selectors block
  safely with an actionable typed error.
- [ ] A project without a selector retains the existing single-feature and
  ambiguity behavior.
- [ ] Tests cover valid selection, malformed JSON, path traversal, missing
  directory, and the no-selector regression path.

## Notes

The Craftlane Spectide route and `craftlane-codex` profile are otherwise valid:
the Codex executable is authenticated and supports both read-only and
workspace-write execution. The block occurs before any provider invocation or
repository mutation.
