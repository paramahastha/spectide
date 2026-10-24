# Local user guide

SpecTide runs on the developer's machine. Development and CI use Go 1.27 and Node.js 26 with npm. Go builds the service and CLI; Node.js builds and tests the embedded dashboard.

## Build and run the offline demo

From the repository root:

```sh
make verify
./bin/spectide doctor
./bin/spectide demo --project ./examples/fake-provider-demo
```

The demo prints a run ID and reports `SIMULATED`. The CLI uses SpecTide's default local database unless `--db PATH` is supplied. `scripts/demo.sh` uses an isolated temporary database. The deterministic fake route does not access provider credentials.

To view the dashboard, start `./bin/spectide serve --host 127.0.0.1 --port 4747` and open `http://127.0.0.1:4747`. In another terminal, run the demo with the same `--db` path used by `serve` if you want the server and CLI to share history.

## CLI overview

Use `spectide --help` and `spectide COMMAND --help` for current flags. Common commands include:

```sh
spectide setup
spectide add /absolute/path/to/another/repository
spectide projects
spectide dashboard
spectide projects register /absolute/path/to/repository
spectide projects list
spectide init --project /absolute/path/to/repository
spectide config --file /absolute/path/to/repository/.spectide/config.json
spectide config validate --file /absolute/path/to/repository/.spectide/config.json
spectide config preview --file /absolute/path/to/repository/.spectide/config.json --prompt "inspect tests"
spectide run --project /absolute/path/to/repository "describe the task"
spectide list
spectide workflow list
spectide workflow explain
spectide report RUN_ID
spectide pause RUN_ID
spectide resume RUN_ID
spectide cancel RUN_ID
```

The default workspace is shared by projects you explicitly add. `setup` displays the current repository's canonical path and asks for confirmation before creating its `.spectide/config.json`; after confirmation it registers the project, reports readiness, and starts or verifies the dashboard. On first setup, if the former config-directory database exists and the new destination is empty, setup displays both paths and separately asks whether to import it. A confirmed import creates a private backup and retains the source; declining leaves the source untouched. `add` registers an existing directory without copying configuration or provider permissions. Run `spectide run "describe the task"` from a registered project to use that project; `--project /absolute/path` remains available for scripted or cross-directory use. The dashboard scopes history by project and lets you add a project by entering an absolute path.

`workflow list` and `workflow explain` print JSON for the canonical stage sequence and ownership/read-only boundaries. Root output controls are available before or after a subcommand: `--no-color` (the CLI currently emits no ANSI styling), `--quiet` (suppresses normal stdout), and `--verbose` (writes a command diagnostic to stderr). Errors and exit codes remain active in quiet mode. `--detach` is not supported.

Live provider routes currently end in a durable `BLOCKED` run unless a verified authenticated adapter is available. They are never reported as simulated success. Exit statuses are 0 success, 2 command error, 3 blocked, 4 failed, and 5 cancelled.

See [configuration](configuration.md), [provider readiness](providers.md), [recovery](recovery.md), and [API](api.md) for details.
