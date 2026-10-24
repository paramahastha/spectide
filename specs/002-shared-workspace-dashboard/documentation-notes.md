# Private Implementation Checklist: Shared Workspace Dashboard

This file records documentation requirements for the feature. Public docs must only be updated after implementation and verification confirm the behavior.

## First-use and command help

- [ ] Explain `spectide setup`, `spectide add [PATH]`, `spectide projects`, and the explicit confirmation boundary.
- [ ] Explain registered-project requirements for `run`, remediation for unregistered paths, and retained `--db` / `--project` compatibility.
- [ ] Document default workspace locations and the `--db` override.
- [ ] Document managed `dashboard` reuse, foreign-listener diagnostics, loopback binding, and intentional isolated `serve --host/--port` use.

## API and dashboard

- [ ] Document workspace status, project readiness, absolute-path registration, project-ID selectors, pagination caps, and filtered count headers.
- [ ] Document All projects as the default view, selected project behavior, stale saved-selection fallback, readiness states, and add-project input.
- [ ] Capture verified dashboard screenshots for desktop, narrow screen, scope switcher, readiness, and add-project error/success states.

## Migration and recovery

- [ ] Document workspace identity, project/run association, treatment of unassigned legacy history, and exact-match backfill.
- [ ] Document backup-first confirmed legacy import, destination refusal rules, source retention, verification, and recovery steps.

## Verification gates

- [ ] Publish only commands, API fields, and migration behavior observed in passing checks.
- [ ] State fake-provider validation as simulated and unavailable live-provider validation as not run.
- [ ] Record outstanding cohort usability validation and 100-attempt dashboard reuse evidence before release.
