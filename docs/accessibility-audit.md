# Dashboard accessibility audit

**Status: PASS for the specified offline/browser fixture scope (2026-10-23); authenticated-provider and live SSE acceptance remain NOT_RUN.**

## Automated state and surface coverage

Command: `cd web && npm test -- --run`
Run: 2026-10-23, Vitest 4.1.12 / jsdom on Node 26. Result: **36 tests passed**. The FR-026 Axe scan renders each named surface and reports **0 critical and 0 serious violations**. The suite includes keyboard-operable approval controls, ordered two-viewer event replay, independent API/SSE connection status, initial API handshake retry, and profile recovery.

Playwright/Chromium adds six rendered browser tests and Axe checks on the 26 required page/state pairs, with zero critical or serious findings. The archived fixture screenshots and viewport/zoom/focus/reduced-motion observations are indexed in [screenshots](screenshots/README.md). These checks cover deterministic API fixtures; they do not claim authenticated provider execution or live network SSE recovery.

## FR-026 page/state matrix

Run: `cd web && npm run test:e2e -- --grep 'all 26 required dashboard'` on 2026-10-23. Result: **26/26 rendered page/state pairs passed** with zero critical or serious Axe findings. Contrast rules were included with WCAG 2.0/2.1 A/AA tags.

| Surface | Scanned states | Result |
|---|---|---|
| Run list (8) | Empty, disconnected, running, paused, blocked, completed, cancelled, simulated | PASS, 8/8 |
| Run detail (9) | Running, paused with pending approval controls, blocked, completed, cancelled, failed, skipped, not-run gate, simulated | PASS, 9/9 |
| Route preview (3) | Ready, blocked, disconnected initial API | PASS, 3/3 |
| Profile readiness (3) | Ready, disconnected, error | PASS, 3/3 |
| Shared surfaces (3) | Empty, error, disconnected | PASS, 3/3 |

All 26 scans returned zero critical/serious violations. This is deterministic browser fixture coverage; live provider content and real-network SSE outage behavior remain NOT_RUN.

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

## Browser evidence summary

- Browser E2E: PASS. Six Playwright tests exercise the rendered dashboard with deterministic API fixtures; the 26-pair matrix records zero critical/serious Axe findings. Component tests additionally cover run-state controls and connection recovery.
- Actual browser zoom: PARTIAL. The authorized Chrome inspection recorded 200% zoom, a 600 CSS px viewport, and no document-level overflow. Full zoom coverage at phone/tablet widths was not captured. The automated CSS page-scale emulation is not counted as actual browser zoom.
- Keyboard/focus: PARTIAL. Browser keyboard navigation showed a visible focus outline; complete focus order and activation across every control remain unverified.
- Rendered contrast and reduced motion: PASS on the scanned states. Axe's browser scan includes color contrast and reports zero critical/serious violations; reduced-motion emulation resolves transitions to zero.
- Archived screenshots: PASS for desktop states and 390×844 phone view; no tablet screenshot or live-backend screenshot is included. The complete list and fixture-backed scope are in [screenshots](screenshots/README.md).
- Authenticated provider execution: NOT_RUN.

## Browser inspection follow-up — 2026-10-23

- Rechecked the live profile empty states, persisted simulated run history/detail, and the ready 12-stage route preview in Chrome.
- Submitted a nonexistent relative project path to **Preview routes**. The dashboard displayed a blocked error state with `execution: NOT_RUN`; it did not start a workflow. Restoring the original project path cleared the stale result.
- Opened a second Chrome tab, selected the same persisted run, and confirmed its stage detail, durable event timeline, and six quality-gate verdicts matched the first viewer. This checks two-viewer projection, not live event replay or reconnect behavior.
- Keyboard Tab focus reached the Project path field and had a visible outline in the rendered page.
- Screenshots were inspected in the browser tool but were not archived from this manual pass. It did not cover live SSE reconnect or the full responsive/page-state matrix; automated rendered accessibility and responsive evidence is recorded in the Playwright section below.

## Playwright browser acceptance — 2026-10-23

Command: `cd web && npm run test:e2e` — **PASS, 5 browser tests** with Playwright 1.64.1 and Chromium 156.0.8078.4. The Vite app ran in Chromium with deterministic API fixtures; provider execution was not attempted. Axe scanned rendered run-list, run-detail, ready and blocked route-preview, profile list/readiness, empty, error, and disconnected states against WCAG 2.0/2.1 A/AA. All scans reported zero critical or serious violations, including color-contrast checks. The scan found and drove a correction to the dim stage index color; the current rule uses the accessible muted text token.

| Browser check | Result |
|---|---|
| Run-list, detail and route-preview interactions; execution labels and gate evidence | PASS |
| Empty, blocked/error and disconnected UI states | PASS |
| Profile empty states and ready profile status | PASS |
| Two independent browser pages showing the same report fixture, stage history and provider activity | PASS |
| Keyboard navigation and visible focus indicator | PASS |
| Reduced-motion media preference | PASS; transition duration resolves to zero |
| Mobile layout | PASS at 390×844 CSS px with no page-level horizontal overflow |
| 200% page-scale reflow emulation | PASS with no page-level horizontal overflow; this is a CSS scaling check, not browser zoom |
| Actual browser 200% zoom | PASS from the separately recorded live Chrome check above; viewport was 600 CSS px wide with no document overflow |
| Browser Axe scans | PASS; zero critical/serious on scanned states |

The archived screenshots and their fixture/viewport metadata are indexed in [screenshots](screenshots/README.md). Component tests continue to cover every run terminal/control state. The browser suite does not simulate provider execution or live SSE reconnect; the local Chrome inspection separately confirmed real API projections for the completed simulated run, ready/blocked dry-run previews, and matching same-run detail across two viewers.
