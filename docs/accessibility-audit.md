# Dashboard accessibility audit

**Status: PARTIAL — rendered browser acceptance remains open (T031/T077).**

## Automated state and surface coverage

Command: `cd web && npm test`  
Run: 2026-10-23, Vitest 4.1.12 / jsdom on Node 26. Result: **29 tests passed**. The FR-026 Axe scan renders each named surface and reports **0 critical and 0 serious violations**. The suite includes keyboard-operable approval controls and two-viewer event replay; rendered browser evidence remains outstanding.

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

Google Chrome is installed and available, but opening the local dashboard at `http://127.0.0.1:4747` was rejected by the browser origin approval gate. The saved browser permission policy denied local network access and disallowed alternate browser surfaces or indirect access. No alternate access path was used. T031/T077 remain unchecked until the origin is authorized and the rendered checks can run.
