# Browser screenshot evidence

Related acceptance evidence: [dashboard accessibility audit](../accessibility-audit.md) · [final verification report](../final-verification.md).

Status: BROWSER EVIDENCE CAPTURED — Playwright screenshots below were captured from the rendered React dashboard at mocked API fixtures. They demonstrate UI states and responsive layout; they do not represent live provider execution. An additional live Chrome session checked the actual local API and is recorded in [the accessibility audit](../accessibility-audit.md). The full browser/accessibility matrix and live-provider checks remain open.

The release evidence pass should save screenshots here from an authorized browser session. Capture the actual rendered dashboard at desktop, tablet, and phone widths, plus the run list and selected-run detail in representative empty, running, blocked, failed, simulated, disconnected, and reconnected states. Capture the 200% zoom and reduced-motion states where they change the rendered layout or motion.

Do not treat jsdom snapshots as browser screenshots. The archived images were generated on 2026-10-23 by `cd web && npm run test:e2e` using Playwright 1.64.1 with Chromium 156.0.8078.4, Vite, and deterministic mocked API responses. Default desktop viewport: 1440×1000 CSS px. The phone image uses 390×844 CSS px. `workflow-200-percent-page-scale.png` applies CSS page scaling to exercise reflow and is not a substitute for browser zoom; the separate live Chrome inspection recorded actual 200% browser zoom with no document-level overflow. The E2E Axe scan covered WCAG 2.0/2.1 A/AA and reported zero critical/serious violations for each scanned state. See [the accessibility audit](../accessibility-audit.md) for scope and limitations.

| Screenshot | Rendered state |
|---|---|
| [Run history](run-history-desktop.png) | One simulated completed run, desktop |
| [Run detail](run-detail-desktop.png) | Twelve stages, one blocked stage, execution attempt, quality evidence, desktop |
| [Ready route preview](route-preview-desktop.png) | Twelve dry-run routes, `NOT RUN`, desktop |
| [Blocked route preview](route-preview-blocked-desktop.png) | Missing project path, blocked error, no run started |
| [Empty workflow](workflow-empty-desktop.png) | No persisted runs or selected run |
| [Empty profiles](profiles-empty-desktop.png) | No registered profiles |
| [Empty profile readiness](profile-readiness-empty-desktop.png) | No profiles to assess |
| [Disconnected dashboard](disconnected-desktop.png) | Local API unavailable |
| [Phone workflow](workflow-phone.png) | 390×844 CSS px viewport |
| [200% page scale](workflow-200-percent-page-scale.png) | CSS page-scale reflow emulation at desktop viewport |

The browser acceptance tests are in [`web/e2e/dashboard.spec.ts`](../../web/e2e/dashboard.spec.ts), with setup in [`web/playwright.config.ts`](../../web/playwright.config.ts). The screenshot set is fixture-backed; live account execution remains `NOT_RUN`.
