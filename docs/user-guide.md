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
spectide projects register /absolute/path/to/repository
spectide projects list
spectide init --project /absolute/path/to/repository
spectide config --file /absolute/path/to/repository/.spectide/config.json
spectide config validate --file /absolute/path/to/repository/.spectide/config.json
spectide config preview --file /absolute/path/to/repository/.spectide/config.json --prompt "inspect tests"
spectide run --project /absolute/path/to/repository "describe the task"
spectide list
spectide report RUN_ID
spectide pause RUN_ID
spectide resume RUN_ID
spectide cancel RUN_ID
```

Live provider routes currently end in a durable `BLOCKED` run unless a verified authenticated adapter is available. They are never reported as simulated success. Exit statuses are 0 success, 2 command error, 3 blocked, 4 failed, and 5 cancelled.

See [configuration](configuration.md), [provider readiness](providers.md), [recovery](recovery.md), and [API](api.md) for details.
