# zoro.ai

`zoro.ai` is a local-first Python CLI that turns an ordered GitHub Projects queue into structured implementation handoffs and optionally delegates those handoffs to Codex CLI. GitHub Projects is the queue, the Git repository remains the source of truth, and Markdown handoffs are Git-trackable contracts between planning and implementation.

## Requirements

- Python 3.12+
- Git and a local Git repository
- A GitHub token with access to the repository and Projects v2, or an authenticated `gh` CLI
- `OPENAI_API_KEY` for planning
- Codex CLI for implementation
- A GitHub Project single-select field (normally `Status`) with the configured workflow values

## Installation

With `uv`:

```bash
uv tool install .
```

For development:

```bash
uv sync --extra dev
uv run pytest
uv run ruff check .
```

Equivalent `pip` installation is supported:

```bash
python -m pip install -e ".[dev]"
```

## Authentication and security

Zoro resolves GitHub credentials in this order:

1. `ZORO_GITHUB_TOKEN`
2. `GH_TOKEN`
3. `gh auth token`

It never writes tokens to configuration or logs. Set `OPENAI_API_KEY` in the environment. Repository collection excludes common secret files, credentials, binary files, ignored paths, `.git`, dependencies, and build output. Only a bounded, relevant context is sent to OpenAI.

```bash
export OPENAI_API_KEY="your-key"
gh auth login
```

On PowerShell, use `$env:OPENAI_API_KEY = "your-key"`.

## Quick start

```bash
zoro init
```

Zoro detects the repository from its GitHub remote, verifies authentication and repository access, and prompts for an accessible GitHub Project. Use `zoro init --project NUMBER` to select a project non-interactively. Initialization then creates:

```text
.zoro/runtime/
handoff/ready/
handoff/implementing/
handoff/review/
handoff/done/
handoff/failed/
```

The runtime directory is ignored; handoffs are deliberately not ignored.

Verify the environment and inspect the queue:

```bash
zoro auth
zoro doctor
zoro board
zoro ready
```

Plan the first ordered Ready item or a specific issue:

```bash
zoro plan
zoro plan 142
```

Plans are requested as structured OpenAI output, validated with Pydantic, and rendered deterministically into `handoff/ready/<issue>-<slug>.md`. Existing handoffs in any lifecycle directory prevent duplicate planning.

Implement interactively or by issue number:

```bash
zoro implement
zoro implement 142
```

Implementation requires a clean working tree. Zoro creates the configured branch, moves the card to In progress, moves the handoff to `implementing`, invokes `codex exec`, and runs validation commands in order. A successful result moves to `review` and updates the card to In review. A failure moves the handoff to `failed` with failure details. Zoro never marks work Done.

## Polling and automatic mode

Run exactly one planning cycle:

```bash
zoro run --once
```

Or continuously poll at `scheduler.interval`:

```bash
zoro run
```

`run` calls the same single-cycle operation repeatedly. An OS-level lock at `.zoro/runtime/zoro.lock` prevents concurrent automatic cycles and implementations.

With the default configuration, a cycle creates a handoff and waits for a developer:

```yaml
automation:
  auto_plan: true
  auto_implement: false
```

Set `auto_implement: true` to continue into Codex and validation. Automatic implementation still refuses a dirty repository.

## Configuration

The generated `.zoro/config.yaml` is validated strictly. Important settings include:

```yaml
version: 1
github:
  owner: OWNER
  repo: REPOSITORY
  project_number: 1
  status_field: Status
  statuses:
    backlog: Backlog
    ready: Ready
    implementing: In progress
    review: In review
    done: Done
scheduler:
  interval: 1m
planning:
  model: gpt-5.6
  max_files: 30
  max_context_bytes: 300000
automation:
  auto_plan: true
  auto_implement: false
implementation:
  branch:
    enabled: true
    prefix: zoro
  validation:
    enabled: true
    commands:
      - pytest
      - ruff check .
```

Durations accept positive integer seconds, minutes, or hours such as `30s`, `5m`, and `1h`. Run `zoro config` to display validated, non-secret settings and `zoro status` for local handoff counts.

## Handoff lifecycle

```text
GitHub Ready       ↔ handoff/ready
GitHub In progress ↔ handoff/implementing
GitHub In review   ↔ handoff/review
GitHub Done        ↔ handoff/done (human-controlled in MVP)
failure            → handoff/failed
```

Humans retain the Backlog → Ready and In review → Done decisions. Zoro owns the guarded Ready → In progress → In review path.

## Development notes

The implementation is intentionally flat. CLI presentation lives in `zoro/cli.py`; orchestration is in `zoro/runner.py`; GitHub, repository analysis, OpenAI planning, deterministic rendering, and Codex execution are separate typed modules. Network and agent calls are mocked in unit tests.
