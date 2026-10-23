# Browser screenshot evidence

Related acceptance evidence: [dashboard accessibility audit](../accessibility-audit.md) · [final verification report](../final-verification.md).

Status: PARTIAL — one owner-provided desktop screenshot was observed in the conversation, but no release screenshot artifact is archived here; local browser-origin authorization remains blocked (T077/T078).

The release evidence pass should save screenshots here from an authorized browser session. Capture the actual rendered dashboard at desktop, tablet, and phone widths, plus the run list and selected-run detail in representative empty, running, blocked, failed, simulated, disconnected, and reconnected states. Capture the 200% zoom and reduced-motion states where they change the rendered layout or motion.

Do not treat jsdom snapshots as browser screenshots. Record viewport, browser/version, zoom, state, and any relevant accessibility scan output beside each captured image. The owner-provided image shows the Workflow control desktop view with one completed simulated run; its source was 3024×1964 (displayed downscaled to 1971×1280). It is preliminary evidence only and is not archived in this directory. The Codex browser integration still denies inspection of `http://127.0.0.1:4747`, so the required screenshot set and rendered checks remain incomplete.
