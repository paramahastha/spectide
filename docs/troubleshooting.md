# Troubleshooting

| Symptom | Meaning and next step |
|---|---|
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

Collect `spectide doctor`, `spectide list`, and `spectide report RUN_ID` output for local diagnosis. Review and redact repository-specific information before sharing a report.
