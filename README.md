# SpecTide

**Orchestrate. Implement. Converge.**

SpecTide is a local control plane for evidence-backed, profile-aware Spec-Kit workflows. It gives you a CLI and a local dashboard for setting up projects, checking coding-agent readiness, starting workflows, and reviewing durable run evidence.

Everything runs on your machine. The dashboard listens only on a loopback address, and provider credentials remain in the provider's own CLI tooling—not in SpecTide configuration.

## What you can do

- Keep several repositories in one local workspace and see them together in the dashboard.
- Use a deterministic offline demo before connecting an AI provider.
- Configure a scoped Claude Code, Codex, or OpenCode profile, then verify its local capabilities before a workflow begins.
- Inspect workflow stages, events, routing decisions, quality gates, and reports after a run.

## Requirements

To build and run the checked-in application, install **Go 1.27 or newer**. To modify, build, or test the dashboard, also install **Node.js 26** and npm.

You do not need a coding-agent account to try the offline demo.

## Install

Clone the repository and build the local binary:

```sh
git clone https://github.com/paramahastha/spectide.git
cd spectide
make build
```

The executable is available at `./bin/spectide`. Confirm that your environment is ready:

```sh
./bin/spectide doctor
```

For the full contributor check—including Go tests, dashboard tests, browser tests, and the smoke test—run `make verify`. It requires Node.js and npm.

## Quick start: try the offline demo

The demo is the fastest, safest way to see SpecTide working. It never contacts an AI provider and is clearly labelled `SIMULATED`.

1. Add the included demo repository to your local workspace:

   ```sh
   ./bin/spectide add ./examples/fake-provider-demo
   ```

2. Start the local dashboard. This command starts a verified background dashboard if one is not already running:

   ```sh
   ./bin/spectide dashboard
   ```

3. Open [http://127.0.0.1:4747](http://127.0.0.1:4747), then run the demo in a second terminal:

   ```sh
   ./bin/spectide demo --project ./examples/fake-provider-demo
   ```

4. Return to the dashboard and open the completed run. You can also inspect it in the terminal:

   ```sh
   ./bin/spectide list
   ./bin/spectide workflow list
   ./bin/spectide workflow explain
   ./bin/spectide report RUN_ID
   ```

The demo finishes with `SIMULATED run completed with fixture evidence`. This proves the local workflow, dashboard, and reporting path without requiring credentials.

## Set up your own repository

Run these commands from the root of the repository you want SpecTide to manage:

```sh
/path/to/spectide/bin/spectide setup
```

`setup` shows the canonical path and asks for confirmation before it creates a missing `.spectide/config.json`, registers the project in the shared workspace, reports its readiness, and starts or verifies the dashboard. On the first run it can also offer to import a former local database; the source is retained and an import creates a private backup.

To add another existing repository without creating or changing its configuration, use:

```sh
/path/to/spectide/bin/spectide add /absolute/path/to/another/repository
/path/to/spectide/bin/spectide projects
```

The dashboard starts at [http://127.0.0.1:4747](http://127.0.0.1:4747) and shows an all-projects view. To run a workflow for a registered project, use:

```sh
/path/to/spectide/bin/spectide run "Describe the change you want to make"
```

You can pass `--project /absolute/path/to/repository` when running from another directory or in scripts.

In the dashboard, select one registered project to see its readiness path and next action. Add a task in the single in-memory draft, choose **Preview routes**, and review the resolved stages before the separate **Start workflow** action. A preview always reports `NOT_RUN`; a blocked preview explains the safe immediate cause and links to its local recovery view without creating a run or transferring repository content. Selecting another project clears the draft. The **Offline demonstration · simulated** control opens a local example and does not call the workflow API.

## Connect a coding agent

Install and sign in to one provider first. Keep credentials in that provider's own tooling; never put API keys or tokens in `.spectide/config.json`.

| Provider | Install the provider CLI | Authenticate | SpecTide provider |
| --- | --- | --- | --- |
| Claude Code | `npm install -g @anthropic-ai/claude-code` | Run `claude` and complete its sign-in flow. | `claude` |
| Codex | `npm install -g @openai/codex@latest` | Run `codex` and complete its sign-in flow. | `codex` |
| OpenCode | `npm install -g @opencode/cli` | Run `opencode`, then use `/connect`. | `opencode` |

Provider CLIs change independently. Consult the official [Claude Code guide](https://docs.anthropic.com/en/docs/claude-code/getting-started), [Codex documentation](https://developers.openai.com/learn/codex), or [OpenCode documentation](https://dev.opencode.ai/v2/docs), then use `--help` and SpecTide's probe to verify what is installed locally.

From the target repository, register one provider profile and probe it. Replace `NAME` and `PROVIDER` with your choice from the table:

```sh
PROJECT_ROOT="$PWD"
/path/to/spectide/bin/spectide profiles add NAME \
  --provider PROVIDER \
  --executable PROVIDER \
  --scope "$PROJECT_ROOT"
/path/to/spectide/bin/spectide profiles probe NAME
```

For example, a Codex profile named `my-codex` is:

```sh
/path/to/spectide/bin/spectide profiles add my-codex \
  --provider codex \
  --executable codex \
  --scope "$PWD"
/path/to/spectide/bin/spectide profiles probe my-codex
```

The probe stores non-secret capability evidence. Review its JSON before continuing; `doctor` only finds executables and does not establish that a profile is ready.

### Choose the project's route

`setup` creates a default simulated route. To use your verified profile, edit `.spectide/config.json` and replace the default route. This example uses the Codex profile above and leaves model and effort at the verified CLI defaults:

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

Preview the route before it creates any work:

```sh
/path/to/spectide/bin/spectide config validate --file .spectide/config.json
/path/to/spectide/bin/spectide config preview --file .spectide/config.json \
  --project "$PWD" \
  --prompt "Describe the change you want to make"
```

`preview` is read-only and creates no run. Once it reports a ready route, start the workflow:

```sh
/path/to/spectide/bin/spectide run "Describe the change you want to make"
```

Live routes also require an installed, verified Spec-Kit agent integration for the repository. If it is missing or a required provider capability is unavailable, SpecTide persists a `BLOCKED` result rather than reporting a false success. OpenCode currently has no verified read-only invocation policy, so read-only stages correctly block until that support is available.

## Everyday commands

```sh
spectide projects                 # list workspace projects and readiness
spectide dashboard                # start or reuse the local dashboard
spectide config preview ...       # confirm a route without starting work
spectide run "..."                # start a configured workflow
spectide list                     # list durable runs
spectide report RUN_ID            # show evidence for one run
spectide pause RUN_ID             # pause safely at a stage boundary
spectide resume RUN_ID            # continue a paused run
spectide cancel RUN_ID            # cancel an active run
```

Use `spectide --help` or `spectide COMMAND --help` for every option. Read the [local user guide](docs/user-guide.md), [configuration reference](docs/configuration.md), [provider readiness](docs/providers.md), [recovery guidance](docs/recovery.md), and [local API contract](docs/api.md) for more detail. For local macOS/Linux release artifacts and verification, see [release preparation](docs/release-preparation.md).

## Security status

The server only accepts loopback hosts and requires a local anti-CSRF header for mutations. The current development token is intentionally local and bootstrap-only; production session handling, authorization, and real provider process execution remain tracked in `specs/001-build-spectide/tasks.md`.

## Development ledger

The source blueprint is tracked as one product feature in `specs/001-build-spectide/`. See its `spec.md`, `plan.md`, `tasks.md`, and `analysis.md` for scope and quality evidence.
