# Bug Assessment: Next-phase run reuses a completed spec and fails Clarify

- **Slug**: clarify-model-visibility
- **Created**: 2026-10-24
- **Source**: pasted text and screenshots
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

The user ran this live Craftlane request:

> please analyze and implement spec based on this plan for next phase /Users/macbookprom4/Projects/craftlane/craftlane-plan-go.md

Run `7dc29766` shows `Specify` succeeded through the `craftlane-codex` profile, then `Clarify` failed and one `clarify_format_repair` retry also failed. The recorded invocations display `default` model and `default effort`, so the user cannot determine the effective Codex model/profile or reconcile account usage. The sole artifact is `specs/006-threejs-workspace/spec.md`, which is the already-completed feature rather than a new next-phase specification. The user reports that no new spec was created.

## Symptom

A prompt explicitly asking for a next phase is treated as an amendment to the selected completed feature. Specify is therefore allowed to update only the old `006-threejs-workspace/spec.md`, never create a new feature directory/spec; after that, Clarify can terminate the whole workflow even after its bounded format-repair retry. The dashboard also renders an unset route as `default`, which is not an actual model identity or usage record.

## Reproduction

1. In a project whose `.specify/feature.json` selects a completed feature such as `specs/006-threejs-workspace`, submit a live prompt that requests implementation of a supplied plan "for next phase."
2. Observe that the run selects that existing directory, and Specify succeeds with `specs/006-threejs-workspace/spec.md` as its only artifact.
3. Observe that no new `specs/<next-phase>/spec.md` is created.
4. Let the Clarify response make no valid spec change and omit the required standalone `SPECTIDE_NO_CLARIFICATION: <reason>` marker; observe the corrective `clarify_format_repair` invocation and a terminal failed run when it also lacks valid evidence.
5. Inspect the invocation history: it shows `codex / craftlane-codex · default · default effort`, not the model selected by Codex CLI nor token/usage accounting.

## Suspected Code Paths

- `internal/speckit/parse.go:112` — `SelectFeatureDirectory` prioritizes `.specify/feature.json`, therefore resolving the existing `006-threejs-workspace` scope before considering the next-phase request.
- `internal/workflow/engine.go:739` — `amendSpecificationPrompt` explicitly commands the provider to update only the selected existing `spec.md` and not create another feature directory.
- `internal/speckit/invoke.go:23` — `FeatureScopeArguments` applies the same hard single-directory restriction to every provider prompt.
- `internal/workflow/artifact_lifecycle.go:77` — treats `next phase` as an amendment exception to Specify's create-once policy, not as an intent to create a new feature scope.
- `internal/workflow/live_stage.go:55` — retries Clarify exactly once after the required no-op evidence is missing, then returns the error if the repair response is still invalid.
- `internal/workflow/stage_semantics.go:17` — validates the no-op marker with a strict standalone, unindented, non-fenced, 12-character-reason format.
- `internal/adapters/codex/codex.go:17` — omits `--model` and the reasoning-effort config when their route values are empty, delegating both to the Codex profile/CLI defaults.
- `web/src/App.tsx:567` and `web/src/App.tsx:614` — render absent model/effort as `default` / `default effort`; they do not display a resolved model or usage.
- `internal/store/attempts.go:55` and `internal/model/model.go:73` — persist configured provider/profile/model/effort and safe execution metadata, but no provider-reported resolved model or token/usage fields.

## Root Cause Hypothesis

Confidence: high. The current implementation intentionally interprets phrases including "next phase" as `specificationIntentAmend`, then pins the run to the feature selected by `.specify/feature.json`. The provider is explicitly prohibited from creating a new feature directory. This makes the observed existing `specs/006-threejs-workspace/spec.md` artifact expected behavior of the code, but incorrect behavior for a request that means a distinct next-phase feature. Separately, Clarify's first no-op-evidence failure does trigger the intended retry, as the run history proves; the second response still fails the exact marker/artifact contract, which is terminal by design. Finally, `default` is only a presentation fallback for an empty route field: SpecTide invokes the Codex CLI without `--model`, so the effective model is chosen by the profile's Codex configuration and is neither resolved nor persisted. The dashboard consequently cannot show the real model or account-level usage, and it should not imply that no usage occurred.

## Proposed Remediation

**Preferred**: Introduce an explicit, persisted specification-scope intent with at least `continue-existing`, `amend-existing`, and `create-next-feature`. A next-phase request must not silently map to `amend-existing`. Before launch, require either a proposed new slug/directory with user confirmation or an explicit feature selector from the dashboard/CLI. For `create-next-feature`, derive and validate a new direct child of `specs/`, run Specify there, and scope all subsequent stages to that new directory. Preserve the existing selected-feature amendment path only when the user explicitly chooses it.

Improve Clarify's repair result by persisting a safe error-code/detail panel and returning a clearly actionable blocked state after the bounded retry, rather than a generic failed workflow. Keep strict no-op evidence, but make the repair prompt a minimal response contract and test it against real Codex JSON event output.

For model transparency, distinguish `configured model: CLI default` from a real model identifier. Persist the exact CLI invocation policy (including the profile/config-directory reference) and, where Codex emits a trustworthy resolved model and usage in structured terminal events, parse and persist sanitized `resolved_model`, input/output tokens, and rate-limit snapshot. If that data is unavailable, show `CLI default (not reported by provider)` and explicitly state that account usage is managed by the Codex account/profile, not inferred from the dashboard. Do not claim per-run billing without provider evidence.

**Alternatives**:

- Require all new feature runs to be created through a separate `new feature` flow. This is safer but adds a workflow step and does not support a natural-language next-phase prompt alone.
- Continue treating next phase as an amendment but change the wording and dashboard artifact display. This matches current behavior but does not meet the user's expectation of a new spec.
- Relax Clarify's marker requirement after a successful repair. This reduces false failures but weakens auditable no-op evidence.

**Files likely to change**:

- `internal/speckit/parse.go`
- `internal/speckit/invoke.go`
- `internal/workflow/engine.go`
- `internal/workflow/artifact_lifecycle.go`
- `internal/workflow/live_stage.go`
- `internal/workflow/stage_semantics.go`
- `internal/adapters/adapter.go`
- `internal/adapters/codex/codex.go`
- `internal/store/attempts.go`
- `internal/model/model.go`
- `internal/store/migrations/` (if resolved model/usage becomes durable attempt evidence)
- `web/src/domain.ts`
- `web/src/App.tsx`

**Tests to add or update**:

- A project with `.specify/feature.json` set to a completed feature plus a next-phase request: assert a separately selected new feature directory/spec, never modification of the old feature.
- An explicit amendment request: assert Specify remains limited to the selected existing feature and preserves the existing safety boundaries.
- An ambiguous next-phase request: assert preview/start asks for or blocks on the new feature slug rather than guessing.
- A real-shaped Codex JSONL stream whose repair response includes a valid marker, and one whose repair response is still invalid; assert safe diagnostic evidence and the chosen terminal state.
- Codex route tests for explicit `--model`/effort versus omitted values, including a UI assertion that omitted is labeled `CLI default (not reported)` rather than `default`.
- Provider-event parsing and report/API tests for optional resolved-model and usage fields, including absent/unknown values and redaction.

## Risks & Considerations

- Creating a new feature scope must be explicit and collision-safe; it must not bypass the `.specify/feature.json` containment and symlink protections.
- A run must persist its selected scope before provider execution so resume/retry cannot switch between the old and new feature.
- Codex's displayed account usage can be delayed or aggregated outside this process; SpecTide must treat provider-reported fields as optional and never fabricate billing data.
- Token/usage fields and provider events need the existing redaction, retention, and schema-compatibility protections.
- Do not broaden the new-feature path so a provider can create arbitrary directories outside `specs/<one-direct-child>`.

## Open Questions

- [NEEDS CLARIFICATION: Should the dashboard automatically propose a new next-phase slug, or require the user to type/select it before starting?]
- [NEEDS CLARIFICATION: Is the desired behavior always a new feature for wording such as "next phase", or do some projects intend an amendment to the current feature?]
- [NEEDS CLARIFICATION: Which Codex account/configuration directory is assigned to `craftlane-codex`, and does its installed CLI expose resolved-model/usage fields in its JSON output?]
