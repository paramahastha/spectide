# Browser screenshot evidence

Status: NOT RUN — local browser-origin authorization is pending (T077/T078).

The release evidence pass should save screenshots here from an authorized browser session. Capture the actual rendered dashboard at desktop, tablet, and phone widths, plus the run list and selected-run detail in representative empty, running, blocked, failed, simulated, disconnected, and reconnected states. Capture the 200% zoom and reduced-motion states where they change the rendered layout or motion.

Do not treat jsdom snapshots as browser screenshots. Record viewport, browser/version, zoom, state, and any relevant accessibility scan output beside each captured image. No screenshot files are claimed by this checkpoint because Chrome's saved origin policy denied access to `http://127.0.0.1:4747`.
