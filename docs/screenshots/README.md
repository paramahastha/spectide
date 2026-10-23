# Browser screenshot evidence

Related acceptance evidence: [dashboard accessibility audit](../accessibility-audit.md) · [final verification report](../final-verification.md).

Status: PARTIAL — after restart on the authorized Chrome origin, run detail, the ready 12-stage route preview, and both profile empty states rendered. At 200% desktop zoom and 390×844 phone size, checks found no page-level horizontal overflow; keyboard focus was visible and the browser console was clean. Screenshots were inspected in Chrome but no screenshot files are archived here. The full browser/accessibility matrix remains open (T077/T078/T103).

The release evidence pass should save screenshots here from an authorized browser session. Capture the actual rendered dashboard at desktop, tablet, and phone widths, plus the run list and selected-run detail in representative empty, running, blocked, failed, simulated, disconnected, and reconnected states. Capture the 200% zoom and reduced-motion states where they change the rendered layout or motion.

Do not treat jsdom snapshots as browser screenshots. Record viewport, browser/version, zoom, state, and any relevant accessibility scan output beside each captured image. The owner-provided image shows the Workflow control desktop view with one completed simulated run; its source was 3024×1964 (displayed downscaled to 1971×1280). It is preliminary evidence only and is not archived in this directory. Chrome origin access to `http://127.0.0.1:4747` was authorized on 2026-10-23. The required screenshot set and remaining rendered checks are incomplete; the initial profile-page failure is fixed and rechecked live.
