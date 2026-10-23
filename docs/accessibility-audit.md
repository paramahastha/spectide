# Dashboard accessibility audit

**Status: PARTIAL — component coverage passes, and live Chrome inspection has started; rendered browser acceptance remains open (T031/T077/T103).**

## Automated state and surface coverage

Command: `cd web && npm test -- --run`
Run: 2026-10-23, Vitest 4.1.12 / jsdom on Node 26. Result: **34 tests passed**. The FR-026 Axe scan renders each named surface and reports **0 critical and 0 serious violations**. The suite includes keyboard-operable approval controls and two-viewer event replay; rendered browser evidence remains outstanding.

| Surface | States covered by automated tests |
|---|---|
| Run list | Empty, disconnected, created, running, paused, blocked, completed, cancelled, failed, simulated label |
| Run detail | Created, running, paused, blocked, completed, cancelled, failed; skipped stage, not-run gate, simulated route label |
| Route preview | Ready, blocked, disconnected; actual view sends `dry_run: true` |
| Profile readiness | Ready, disconnected, error, no profiles |
| Shared empty surface | Empty |
| Shared error surface | Error alert |
| Shared disconnected surface | Disconnected status |

The Axe pass scans one rendered state for each of the seven named surfaces. Its `color-contrast` rule is disabled because jsdom does not implement canvas-based color calculations; contrast is checked separately from the stylesheet tokens. `--ink`, `--muted`, and `--muted-2` each exceed 4.5:1 against the dashboard and panel backgrounds; the lowest tested ratio is 5.29:1 (`--muted-2` on `--panel`).

Keyboard focus has a visible `:focus-visible` outline, and the stylesheet has a reduced-motion rule that removes transitions. Automated tests verify these rules exist. The UI controls are semantic buttons and form fields; browser-level tab order and activation have not been inspected.

## Chrome inspection — 2026-10-23

The owner authorized `http://127.0.0.1:4747` in Chrome's saved origin policy. After restarting the server to load the fixed embedded bundle, the authorized Chrome tab rendered the completed simulated run detail, including all 12 stages and six labeled quality gates. **Profiles & models** displayed “No profiles registered”; **Profile readiness** displayed “No profiles to assess.” The Route preview surface rendered a ready result for all 12 stages after the visible **Preview routes** interaction; it remained labeled `NOT RUN` and stated that no run starts from the screen. The browser console had no warning or error entries. Screenshots were inspected during the session, but no image files were archived in this repository.

The initial live pass left **Profiles & models** and **Profile readiness** blank. Diagnosis found that an empty profile query was serialized as JSON `null` and then treated as an array by the dashboard. The store now initializes the result as an empty slice (`[]` on the wire), and the dashboard defensively normalizes unexpected null responses. Go API/store tests and a frontend regression test cover both contracts. After restarting the server, Chrome confirmed the profile empty states render. The full page/state matrix was not completed.

## Not verified in a browser

- Browser E2E across the full page/state matrix: NOT_RUN.
- Actual 200% zoom at the desktop viewport: PARTIAL. Chrome confirmed 200% zoom; the page viewport was 600 CSS px wide, document width remained 600 px with no page-level horizontal overflow, and the navigation used its horizontal scroll area. The full responsive page/state matrix at zoom remains NOT_RUN.
- Keyboard/focus: PARTIAL. Tab moved focus to Run history and the focus outline was visible in the rendered screenshot. Full focus order and keyboard activation across all controls remain NOT_RUN.
- Rendered browser contrast and reduced-motion behavior: NOT_RUN. Contrast and reduced-motion source/component checks are recorded above, but a rendered browser pass has not been completed.
- Archived screenshot set at desktop, tablet, and phone widths: NOT_RUN; one owner-provided desktop view is noted below.

The owner supplied a Chrome screenshot in the conversation on 2026-10-23 showing the Workflow control dashboard with one completed simulated run at a desktop viewport. The image source was 3024×1964 and displayed here downscaled to 1971×1280. This is preliminary visual evidence for that single view only; it is not archived in this repository and does not verify the FR-026 page/state matrix, keyboard behavior, zoom, contrast, or reduced motion.

The Chrome origin is authorized now, and the corrected profile views have been confirmed live after restarting the server. Browser E2E across the full page/state matrix, remaining rendered interaction states, 200% zoom across the responsive matrix, complete keyboard/focus order, rendered contrast, reduced-motion presentation, two-client browser behavior, and the archived screenshot set remain incomplete. T031/T077/T103 remain unchecked until the remaining rendered results and screenshot files are recorded.
