# Bug Assessment: Active feature selector is ignored

- **Slug**: active-feature-selection
- **Created**: 2026-10-24
- **Source**: pasted text (`docs/spectide-active-feature-selection-issue.md` and supplied screenshots)
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

Craftlane has multiple valid feature directories beneath `specs/` and declares
`.specify/feature.json` with `feature_directory: "specs/006-threejs-workspace"`.
Starting a live workflow creates a durable `LIVE` run which immediately ends
`BLOCKED` with `SPECIFICATION_AMBIGUOUS: found 6 feature artifact directories`.
The route-preview view also renders all twelve stages as a dense, hard-to-scan
three-column list.

## Symptom

Spectide does not honor Craftlane's explicit active Spec-Kit feature scope, so
workflow start blocks before any provider is invoked. The preview is technically
ready but makes the stage routes difficult to inspect because stage, provider
route, and explanation compete in narrow columns.

## Reproduction

1. Register a project with multiple direct-child `specs/*` artifact directories.
2. Add `.specify/feature.json` with a valid `feature_directory` selecting one
   direct child of `specs/`.
3. Start a live Spectide workflow for that project.
4. Observe `SPECIFICATION_AMBIGUOUS` before a provider invocation.
5. Open Route preview for a configured project and inspect the rendered stage list.

## Suspected Code Paths

- `internal/speckit/parse.go:93` — `SelectFeatureDirectory` only scans
  `specs/*`; it does not read `.specify/feature.json`.
- `internal/workflow/engine.go:119` — live start invokes that selector before
  creating the checkpoint and provider execution.
- `internal/workflow/checkpoint.go:131` — the selected scope is already safely
  persisted relative to the canonical workspace, so no new checkpoint format is
  required.
- `internal/speckit/parse_test.go:259` — selection behavior has coverage but
  lacks explicit-selector validation cases.
- `web/src/App.tsx:630` and `web/src/style.css` — route-preview decisions are
  rendered in a cramped fixed-column grid, which produces the visible wrapping
  in the supplied screenshot.

## Root Cause Hypothesis

`SelectFeatureDirectory` was designed to safely reject ambiguity by counting
direct-child artifact directories. It never consults the canonical selector that
Spec-Kit writes in `.specify/feature.json`, so a valid selected feature is
indistinguishable from an unsafe ambiguous workspace. Confidence: high.

## Proposed Remediation

**Preferred**: Read `.specify/feature.json` before enumerating `specs/*`. When
present, require JSON object field `feature_directory` to be a canonical,
project-contained, direct child path in the form `specs/<name>`; resolve it
without accepting symlink escapes and require that directory to exist. Return
typed errors for malformed JSON, invalid paths, and missing directories. Keep
the existing selector-free ambiguity behavior. Update uniqueness validation to
allow the explicitly selected scope while continuing to reject an invalid or
changed selector. The existing checkpoint will preserve the selected relative
scope and its workspace-drift validation.

Refactor the route-preview decisions into compact stage cards with a clear stage
heading, route metadata, and a full-width reason below. Preserve semantic status
and responsive behavior while preventing narrow-column wrapping at desktop and
phone widths.

**Files likely to change**:

- `internal/speckit/parse.go`
- `internal/speckit/parse_test.go`
- `web/src/App.tsx`
- `web/src/style.css`
- `web/src/accessibility.test.tsx`

**Tests to add or update**:

- Valid explicit selection among multiple feature directories.
- Malformed JSON, traversal, nested selector, symlink escape, and missing
  selected directory typed-error cases.
- No-selector single-feature and ambiguous-feature regression cases.
- Checkpoint persistence of the selected relative directory.
- Route-preview semantic structure and responsive UI regression coverage.

## Risks & Considerations

- The selector must not permit a nested path, absolute path, path traversal, or
  symlink escape outside the registered project.
- Later live-stage validation currently assumes exactly one artifact scope and
  must be reconciled with a valid explicit selection to avoid blocking after
  the first stage.
- Existing projects without a selector must keep their fail-closed behavior.
- UI changes must retain keyboard focus, readable status text, and no horizontal
  overflow on phone widths.

## Open Questions

- None; the report, current source, and acceptance criteria specify the
  required selector contract.
