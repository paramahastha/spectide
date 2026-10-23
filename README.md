# SpecTide

**Orchestrate. Implement. Converge.**

SpecTide is a local control plane for evidence-backed, profile-aware Spec-Kit workflows. It stores run history in SQLite and provides a CLI plus a loopback dashboard with live event replay. It can route work to Claude Code, Codex, or OpenCode after it has verified the selected local profile and its capabilities.

The built-in offline demo is deterministic and visibly **SIMULATED**. A provider that is missing, unauthenticated, or lacks a required capability produces a durable `BLOCKED` result; SpecTide never presents it as a completed live run.

## Install SpecTide

Requirements: Go 1.27, Node.js 26, and npm.

```sh
git clone https://github.com/paramahastha/spectide.git
cd spectide
make build
```

The binary is now `./bin/spectide`. Check the local environment:

```sh
./bin/spectide doctor
```

## Five-minute tutorial

This path runs entirely on your machine and needs no provider account. It is the quickest way to see the complete lifecycle and dashboard.

1. Build SpecTide, then start the local dashboard in one terminal:

   ```sh
   ./bin/spectide serve
   ```

2. Open [http://127.0.0.1:4747](http://127.0.0.1:4747).

3. In a second terminal, run the deterministic demo:

   ```sh
   ./bin/spectide demo --project ./examples/fake-provider-demo
   ```

4. Return to the dashboard. Open the completed run to inspect its stages, event stream, quality gates, routing decision, and persisted report.

5. Inspect the same evidence from the terminal:

   ```sh
   ./bin/spectide list
   ./bin/spectide report RUN_ID
   ```

The terminal should finish with `SIMULATED run completed with fixture evidence`. That label is expected: this tutorial proves the local workflow without contacting an AI provider.

## Connect a coding agent

Install and sign in to one provider first. Keep credentials in that provider's own tooling; do not place API keys or tokens in `.spectide/config.json`.

| Provider | Install the provider CLI | Authenticate | SpecTide profile |
| --- | --- | --- | --- |
| Claude Code | `npm install -g @anthropic-ai/claude-code` | Run `claude` and complete its sign-in flow. | `claude` |
| Codex | `npm install -g @openai/codex@latest` | Run `codex` and complete its sign-in flow. | `codex` |
| OpenCode | `npm install -g @opencode/cli` | Run `opencode`, then use `/connect`. | `opencode` |

The installation commands and login flows above come from the official [Claude Code guide](https://docs.anthropic.com/en/docs/claude-code/getting-started), [Codex documentation](https://developers.openai.com/learn/codex), and [OpenCode documentation](https://dev.opencode.ai/v2/docs). Provider CLIs change independently, so use `--help` and let SpecTide's probe establish what the installed version can safely do.

From the project you want SpecTide to manage, register and probe exactly one provider. Replace `NAME` and `PROVIDER` with the row you chose above. The scope is the project root and prevents that profile from being used outside this repository.

```sh
PROJECT_ROOT="$PWD"
./bin/spectide profiles add NAME \
  --provider PROVIDER \
  --executable PROVIDER \
  --scope "$PROJECT_ROOT"
./bin/spectide profiles probe NAME
```

For example, a Codex profile named `my-codex` is:

```sh
./bin/spectide profiles add my-codex \
  --provider codex \
  --executable codex \
  --scope "$PWD"
./bin/spectide profiles probe my-codex
```

`probe` saves non-secret capability evidence, including the executable identity and any safely observable authentication and execution controls. Review its JSON before proceeding. `doctor` only checks whether executables exist; it does not prove that a profile is ready.

### Configure a project route

Initialize the project's configuration, then replace its default fake route with your verified profile:

```sh
./bin/spectide init --project "$PWD"
```

Edit `.spectide/config.json` so its route names your provider and profile. This Codex example omits model and effort overrides, letting the verified CLI defaults apply:

```json
{
  "version": 1,
  "project": "/absolute/path/to/your/project",
  "routing_mode": "hybrid",
  "default_route": {
    "provider": "codex",
    "profile": "my-codex"
  },
  "allowed_scopes": ["/absolute/path/to/your/project"]
}
```

For Claude Code or OpenCode, change only `provider` and `profile` to match the registered profile. Use absolute paths in `project` and `allowed_scopes`.

Preview before starting anything:

```sh
./bin/spectide config validate --file .spectide/config.json
./bin/spectide config preview --file .spectide/config.json \
  --project "$PWD" \
  --prompt "Describe the change you want to make"
```

`preview` is read-only and creates no run. When it reports a ready route, start the workflow:

```sh
./bin/spectide run --project "$PWD" "Describe the change you want to make"
```

Live routes also require an installed, verified Spec-Kit agent integration for the repository. If that integration or a required provider capability is unavailable, the run is persisted as `BLOCKED` with the reason. Current OpenCode support has no verified read-only invocation policy, so read-only stages correctly block; use the demo or a profile whose probe satisfies the requested route until that support lands.

## Everyday workflow

1. Start `./bin/spectide serve` and use the dashboard at `127.0.0.1:4747` to follow one run.
2. Add a provider profile once per approved project scope, then run `profiles probe` whenever its executable or login state changes.
3. Use `config preview` after each route change.
4. Start work with `spectide run`, then inspect `spectide report RUN_ID` for its durable evidence.

Read the [local user guide](docs/user-guide.md), [configuration reference](docs/configuration.md), [provider readiness](docs/providers.md), [recovery guidance](docs/recovery.md), and [local API contract](docs/api.md).

For local macOS/Linux release artifact preparation, checksums, and verification steps, see [release preparation](docs/release-preparation.md). This workflow prepares files only; it does not publish them.

## Security status

The server only accepts loopback hosts and requires a local anti-CSRF header for mutations. The current development token is intentionally local and bootstrap-only; production session handling, authorization, and real provider process execution remain tracked in `specs/001-build-spectide/tasks.md`.

## Development ledger

The source blueprint is tracked as one product feature in `specs/001-build-spectide/`. See its `spec.md`, `plan.md`, `tasks.md`, and `analysis.md` for scope and quality evidence.
