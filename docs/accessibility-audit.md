# Dashboard accessibility audit

**Status: PARTIAL — browser acceptance remains open (T031).**

## Automated state and surface coverage

Command: `cd web && npm test`  
Run: 2026-10-22, Vitest 4.1.12 / jsdom. Result: **26 tests passed**. The FR-026 Axe scan renders each named surface and reports **0 critical and 0 serious violations**.

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

## Not verified in a browser

- Browser E2E across the full page/state matrix: NOT_RUN.
- Actual 200% zoom reflow and horizontal overflow: NOT_RUN.
- Rendered browser contrast, keyboard/focus sequence, and reduced-motion behavior: NOT_RUN.
- Screenshots at desktop, tablet, and phone widths: NOT_RUN.

No Chromium, Chrome, or Edge executable is installed in this environment. A previous localhost browser inspection was denied by the browser access gate. No alternate browser access path was used. T031 remains unchecked until these browser checks can run in an authorized browser environment.
