# zoro.ai

`zoro.ai` is a local-first Go CLI that turns an ordered GitHub Projects v2 board into implementation handoffs and, when approved or configured, invokes Codex to implement them. GitHub is the queue, the repository remains the source of truth, and Markdown handoffs are the auditable contract between planning and implementation.

## Requirements and installation

- Go 1.24 or newer
- Git and a GitHub token (`ZORO_GITHUB_TOKEN`, `GH_TOKEN`, or `gh auth login`)
- `OPENAI_API_KEY` for planning
- Codex CLI for implementation

Build from source with `go build -o ./cmd/zoro ./cmd/zoro`, or after a tagged public release use `go install github.com/zoro-cli/zoro.ai/cmd/zoro@latest`.

## Getting started

Run inside a Git repository whose `origin` points to GitHub:

```sh
zoro init --project 3
zoro doctor
zoro board
zoro ready
zoro plan
zoro implement
```

Initialization derives the GitHub owner and repository from `origin`, creates `.zoro/config.yaml`, and creates `handoff/{ready,implementing,review,done,failed}`. Secrets are never written to configuration. Edit the generated YAML to map the board's status names, select a model, tune context limits, validation commands, polling, branches, and automation.

## Commands

- `zoro auth` verifies the token, repository, and project.
- `zoro doctor` checks configuration, tools, credentials, project fields, and directories. A dirty tree is a warning.
- `zoro board` prints status counts; `zoro ready` prints Ready items in project connection order.
- `zoro plan [issue]` collects bounded, secret-filtered repository context and asks OpenAI for a structured plan. Zoro validates the JSON and renders deterministic Markdown in `handoff/ready`.
- `zoro implement [issue]` selects a handoff, requires a clean tree, creates a `zoro/<issue>-<slug>` branch, moves the card to In progress, runs Codex and validation, then moves the handoff/card to review. It never marks work Done.
- `zoro run --once` performs one planning cycle. `zoro run` repeats at `scheduler.interval`, shows a waiting spinner in interactive terminals, and exits on SIGINT/SIGTERM. Redirected output remains plain.
- `zoro status` reports local state. `zoro config` prints non-secret effective configuration; `zoro config path` prints its path.

Set `automation.auto_implement: true` only when unattended Codex execution is intended. A repository lock prevents overlapping operations. Planning may inspect a dirty tree and records that fact; implementation refuses to run until changes are committed, stashed manually, or removed. Zoro never auto-stashes, resets, cleans, force-pushes, or deletes branches.

Automatic cycles implement both newly planned handoffs and existing handoffs in `ready`. Handoffs already in another lifecycle state are not restarted automatically.

Repository context excludes common build/vendor directories, binaries, `.env` files, private keys, credentials, and other likely secrets. External payloads and tokens are not logged.
