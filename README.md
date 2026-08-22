# zoro.ai

`zoro.ai` is a local-first Go CLI that turns a GitHub Projects v2 board or GitLab Issue Board into implementation handoffs and, when approved or configured, invokes Codex to implement them. The repository remains the source of truth, and Markdown handoffs are the auditable contract between planning and implementation.

## Requirements and installation

- Go 1.24 or newer
- Git and a provider token: GitHub uses `ZORO_GITHUB_TOKEN`, `GH_TOKEN`, or `gh auth login`; GitLab uses `ZORO_GITLAB_TOKEN`, `GITLAB_TOKEN`, or `glab auth login`
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

For a GitLab.com or self-managed GitLab origin, initialize with `zoro init --provider gitlab --board 1`. Use `--gitlab-url https://gitlab.example.com` when the API host differs from the origin. GitLab configuration contains the project path (including nested groups), board ID, and label-backed list mappings:

```yaml
provider: gitlab
gitlab:
  base_url: https://gitlab.example.com
  project: group/subgroup/project
  board_id: 1
  statuses:
    backlog: Backlog
    ready: Ready
    implementing: In progress
    review: In review
    done: Done
```

An omitted `provider` remains backward-compatible and selects GitHub. Tokens are runtime-only. GitLab workflow states correspond to board label lists; Zoro preserves the same Ready-to-In-progress-to-review lifecycle and never automatically moves work to Done. Merge-request creation and merging are not included.

## Commands

- `zoro auth` verifies the token, repository, and project.
- `zoro doctor` checks configuration, tools, credentials, project fields, and directories. A dirty tree is a warning.
- `zoro board` prints status counts; `zoro ready` prints Ready items in project connection order.
- `zoro plan [issue]` collects bounded, secret-filtered repository context and asks OpenAI for a structured plan. Zoro validates the JSON and renders deterministic Markdown in `handoff/ready`.
- `zoro implement [issue]` selects a handoff, requires a clean tree, creates a `zoro/<issue>-<slug>` branch, and commits the handoff's move into `implementing` before Codex starts. After Codex and validation succeed, it commits all implementation changes together with the handoff's move to `review`, then moves the card to In review. It never pushes, opens a pull request, merges, or marks work Done.
- `zoro run --once` performs one planning cycle. `zoro run` repeats at `scheduler.interval`, shows a waiting spinner in interactive terminals, and exits on SIGINT/SIGTERM. Redirected output remains plain.
- `zoro status` reports local state. `zoro config` prints non-secret effective configuration; `zoro config path` prints its path.

Set `automation.auto_implement: true` only when unattended Codex execution is intended. A repository lock prevents overlapping operations. Planning may inspect a dirty tree and records that fact; implementation refuses to run until changes are committed, stashed manually, or removed. Zoro never auto-stashes, resets, cleans, force-pushes, or deletes branches.

Automatic cycles implement both newly planned handoffs and existing handoffs in `ready`. Handoffs already in another lifecycle state are not restarted automatically.

Ready handoffs are also mirrored to their GitHub issue comments or GitLab issue notes. Zoro appends a hidden handoff identity marker and checks all existing comments so repeated cycles do not intentionally post duplicates. Items without an issue discussion cannot be synchronized.

Repository context excludes common build/vendor directories, binaries, `.env` files, private keys, credentials, and other likely secrets. External payloads and tokens are not logged.
