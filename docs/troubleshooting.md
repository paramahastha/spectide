# Troubleshooting

| Symptom | Meaning and next step |
|---|---|
| `PROJECT_NOT_REGISTERED` | Run `spectide add /absolute/path/to/repository` from the repository or use the canonical path shown in the error, then retry the workflow. |
| `DASHBOARD_PORT_OCCUPIED` | SpecTide could not verify that `127.0.0.1:4747` belongs to the selected workspace. Keep that service untouched and use `spectide serve --port PORT` with another loopback port for an isolated dashboard. |
| `DASHBOARD_LAUNCH_IN_PROGRESS` | Another command is starting the shared dashboard. Retry `spectide dashboard` after startup settles. |
| `LEGACY_IMPORT_*` | The confirmed import failed validation or could not safely install the destination. The source is retained; inspect the reported error and backup path before retrying. Never delete the source to resolve an import failure. |
| `MISSING_CONFIGURATION` / `MISSING_ROUTE` | Run `spectide setup` from the project to create its default configuration after confirmation, then set a valid route in `.spectide/config.json`. |
| `MISSING_PROFILE` / `PROFILE_NOT_AUTHORIZED` / `CAPABILITY_NOT_VERIFIED` | Register the required provider profile for this project's canonical scope and probe it before starting a live workflow. |
| `PROJECT_PATH_UNAVAILABLE` or `PROJECT_NOT_DIRECTORY` | Use an existing local directory. Workflow start resolves symlinks before applying scope checks. |
| `PERMISSION_DENIED` / `PROFILE_NOT_ALLOWED` | The canonical project is outside a configured workspace or profile root. Review `allowed_scopes`; do not broaden it unless the project is intended to cross that boundary. |
| `PROFILE_NOT_FOUND` / `PROFILE_PROVIDER_MISMATCH` | Add a stored profile with the name and provider used by the route. |
| `SPEC_KIT_NOT_INITIALIZED` | Initialize the repository with the installed `specify` CLI and a supported agent integration before selecting live Spec-Kit stages. |
| `SPEC_KIT_STAGE_UNAVAILABLE` | The initialized integration does not provide the required stage command. Check the integration manifest and installed command files. |
| `LIVE_STAGE_EXECUTION_UNAVAILABLE` | The route passed profile checks, but live authenticated stage execution is not enabled. Inspect the persisted `BLOCKED` run report; no provider process ran. |
| `WORKSPACE_LOCKED` | Another active run owns the canonical project. Wait for it to finish or inspect its durable state before retrying. |
| `WORKSPACE_DRIFT` | An owner lease expired and full restart validation is unavailable. Review the preserved report and workspace before starting a new run. |
| `RETENTION_CAP_ACTIVE` | All detailed-history slots are active. Wait for a terminal run before retrying. |
| Provider `NOT_FOUND` or `FOUND_NOT_AUTHENTICATED` | Install the CLI or authenticate through its own supported account flow. SpecTide probes do not inspect or copy credentials. |

## Dashboard route preview

The dashboard's **All projects** scope is for observing run history. Select one registered project to use the readiness action and task draft. The route preview rechecks current server readiness on every request and reports `NOT_RUN` for both ready and blocked outcomes. A blocked result has a safe cause and a local remediation action; its support request ID is secondary and is not needed to decide what to fix. A preview does not create a run, stage, event, provider invocation, or repository transfer.

If a project changes after the dashboard loads, preview or start it again to see the current prerequisite. A rejected start refreshes the selected project's readiness; it does not imply that a run began. If the local API is disconnected, use **Retry after reconnecting** before acting on any previous readiness message. Your unsent task draft stays in the open page if the selected project becomes unavailable, and project-specific actions remain disabled until you choose an available project.

The task draft exists only in the current page session and clears when you select another project. The offline demonstration is labeled simulated and makes no API request.

Collect `spectide doctor`, `spectide list`, and `spectide report RUN_ID` output for local diagnosis. Review and redact repository-specific information before sharing a report.
