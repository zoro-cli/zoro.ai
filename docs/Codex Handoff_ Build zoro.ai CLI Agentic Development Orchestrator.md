# Codex Handoff: Build `zoro.ai` CLI Agentic Development Orchestrator

## Objective

Build the first production-grade MVP of **zoro.ai**, a local-first Python CLI that integrates with GitHub Projects and implements an agentic software development loop.

`zoro.ai` should:

1. Authenticate to GitHub using a user-provided GitHub token or an existing `gh` CLI session.
2. Read a configured GitHub Project.
3. Understand the project workflow statuses:
   - Backlog
   - Ready
   - In progress
   - In review
   - Done
4. Poll the project periodically.
5. Select the top item in `Ready`.
6. Inspect the repository and issue context.
7. Ask an OpenAI model to create an implementation plan.
8. Render that plan into a Markdown handoff.
9. Save the handoff under `handoff/ready/`.
10. If automatic implementation is enabled, invoke Codex CLI with that handoff.
11. If automatic implementation is disabled, allow the developer to manually select and implement a ready handoff with `zoro implement`.

The CLI should remain simple, local-first, Git-native, deterministic, secure, and easy to extend.

---

# Core Product Model

Treat the system as five distinct concerns:

```text
GitHub Project
    =
work queue

Git repository
    =
source code and source of truth

Handoff Markdown
    =
contract between planner and implementer

OpenAI
    =
planning and repository analysis

Codex
    =
implementation

Zoro
    =
orchestration
```

Do not build a web application.

Do not introduce FastAPI.

Do not add a database.

Do not add Redis, Celery, queues, or other infrastructure.

---

# Technology Stack

Use:

```text
Python 3.12+
Typer
Rich
questionary
Pydantic v2
pydantic-settings
httpx
openai
PyYAML
filelock
tenacity
pytest
ruff
```

Use `uv` for dependency management if practical.

Prefer direct GitHub GraphQL/REST access using `httpx` instead of introducing a large GitHub SDK.

Use the local `git` CLI through `subprocess` for repository operations unless there is a strong reason otherwise.

Use the local Codex CLI for implementation.

---

# Recommended Repository Structure

Keep the implementation relatively flat.

```text
zoro-ai/
├── zoro/
│   ├── __init__.py
│   ├── cli.py
│   ├── config.py
│   ├── models.py
│   ├── github.py
│   ├── repository.py
│   ├── planner.py
│   ├── handoff.py
│   ├── codex.py
│   ├── runner.py
│   └── errors.py
│
├── tests/
│   ├── test_config.py
│   ├── test_github.py
│   ├── test_repository.py
│   ├── test_handoff.py
│   └── test_runner.py
│
├── pyproject.toml
├── README.md
└── LICENSE
```

Avoid unnecessary architecture such as:

```text
repositories/
services/
usecases/
domain/
infrastructure/
adapters/
ports/
```

unless a real need emerges.

---

# CLI Commands

Implement these commands for MVP.

```bash
zoro init
zoro auth
zoro doctor
zoro board
zoro ready
zoro plan
zoro implement
zoro run
zoro status
zoro config
```

Support:

```bash
zoro plan
```

Plan the top `Ready` item.

```bash
zoro plan 142
```

Plan a specific issue/project item.

```bash
zoro implement
```

Show an interactive selector containing Markdown files from:

```text
handoff/ready/
```

```bash
zoro implement 142
```

Implement the handoff associated with issue `142`.

```bash
zoro run
```

Run continuously using the configured poll interval.

```bash
zoro run --once
```

Perform exactly one polling cycle and exit.

The `--once` behavior should be implemented first and used internally by the continuous runner.

---

# Initialization

`zoro init` should create:

```text
.zoro/
└── config.yaml

handoff/
├── ready/
├── implementing/
├── review/
├── done/
└── failed/
```

Also create:

```text
.zoro/runtime/
```

for transient files such as locks.

Add the runtime directory to `.gitignore` if needed.

Example:

```text
.zoro/runtime/
```

Do not automatically ignore the handoff directory. Handoffs are intended to be Git-trackable.

---

# Configuration

Use:

```yaml
version: 1

github:
  owner: nenjotech
  repo: example-project
  project_number: 3

  status_field: Status

  statuses:
    backlog: Backlog
    ready: Ready
    implementing: In progress
    review: In review
    done: Done

scheduler:
  enabled: true
  interval: 1m

planning:
  provider: openai
  model: gpt-5.6

  max_files: 30
  max_context_bytes: 300000

automation:
  auto_plan: true
  auto_implement: false

implementation:
  provider: codex

  branch:
    enabled: true
    prefix: zoro

  validation:
    enabled: true
    commands:
      - uv run pytest
      - uv run ruff check .

handoff:
  directory: handoff

behavior:
  max_concurrent_tasks: 1
  move_to_in_progress_on_implement: true
  move_to_review_on_success: true
```

Support human-readable durations:

```text
30s
1m
5m
15m
1h
```

Validate invalid or unsafe values.

---

# Authentication

Do not store secrets in `.zoro/config.yaml`.

Resolve the GitHub token using this precedence:

```text
1. ZORO_GITHUB_TOKEN
2. GH_TOKEN
3. configured token command if implemented
4. gh auth token
```

The MVP may support only:

```text
ZORO_GITHUB_TOKEN
GH_TOKEN
gh auth token
```

That is sufficient.

`zoro auth` should:

1. Resolve credentials.
2. Verify GitHub authentication.
3. Verify access to the configured repository.
4. Verify access to the configured GitHub Project.
5. Return clear structured errors.

Never print the token.

Never log the token.

---

# `zoro doctor`

Implement a diagnostic command similar to:

```text
zoro.ai doctor

Git repository       ✓
Repository clean     ✓
GitHub CLI           ✓
GitHub auth          ✓
GitHub project       ✓
Project Status field ✓
OpenAI API key       ✓
Codex CLI            ✓
Python               ✓
```

Check at least:

```text
inside Git repository
git executable available
working tree status
GitHub credentials
GitHub project access
configured status field
required status options
OPENAI_API_KEY
Codex CLI installed
config validity
handoff directories
```

Do not require the repository to be clean merely to run `doctor`.

Report dirty state as a warning.

---

# GitHub Project Model

The configured project contains a single-select field named:

```text
Status
```

Expected values:

```text
Backlog
Ready
In progress
In review
Done
```

Do not hardcode these labels throughout the application.

Read them from configuration.

The GitHub integration must support:

```text
list project items
read item title
read issue number when available
read item body/issue body
read status
read position/order
read repository metadata
update project status
```

For `Ready`, preserve project ordering.

The top item in the visible Ready column should be treated as the next item to process.

Do not sort by:

```text
issue number
creation date
alphabetical title
random ordering
```

unless GitHub API ordering cannot be obtained.

If project ordering is technically unavailable in a particular query, document the fallback explicitly and use a deterministic fallback.

---

# Project Board Command

`zoro board` should output a concise summary:

```text
Backlog       5
Ready         3
In progress   1
In review     2
Done          20
```

Optionally show the top few items.

Do not build a terminal Kanban UI for MVP.

---

# `zoro ready`

Display ordered ready items.

Example:

```text
Ready

1. #142 Add refresh token rotation
2. #147 Add recipe validation
3. #151 Fix admin pagination
```

---

# Polling Model

Do not refer to the internal scheduler as cron.

The normal mode is a poll loop.

Conceptually:

```python
while running:
    run_once()
    sleep(interval)
```

`zoro run --once` should:

1. Acquire the Zoro lock.
2. Read the GitHub Project.
3. Get ordered `Ready` items.
4. Select the top eligible item.
5. Skip if already planned or being processed.
6. Build repository context.
7. Generate the plan.
8. Save a handoff.
9. If `auto_implement` is enabled, invoke implementation.
10. Exit.

`zoro run` should repeatedly call the equivalent operation.

Do not duplicate behavior between `run` and `run --once`.

---

# Locking and Duplicate Protection

Use an OS-level lock such as:

```text
.zoro/runtime/zoro.lock
```

Only one active automatic cycle should run against a repository at once.

Protect against:

```text
two `zoro run` processes
manual `zoro implement` while automatic implementation is active
duplicate handoffs for the same project item
duplicate Codex execution
```

Use `filelock` or equivalent.

Do not implement distributed locking.

---

# Repository Safety

Before automatic implementation:

```bash
git status --porcelain
```

must be clean.

If the working tree contains developer changes, do not auto-stash them.

Return an error similar to:

```text
Cannot start automatic implementation.

Repository contains uncommitted changes:

 M app/auth.py
?? notes.txt

Commit, stash, or remove these changes first.
```

Planning may still be allowed on a dirty repository, but the handoff should indicate that the repository contained local changes.

Implementation must not overwrite them.

---

# Repository Inspection

The planner should not receive the entire repository blindly.

Create a deterministic context collection step.

Inspect repository metadata files first:

```text
AGENTS.md
CLAUDE.md
README.md
pyproject.toml
package.json
go.mod
Cargo.toml
Dockerfile
docker-compose.yml
.github/
docs/
```

Read whichever files exist.

Then use the issue/project item text to identify repository keywords.

Search with `rg` if available.

Examples:

```bash
rg "refresh"
rg "access_token"
rg "cookie"
rg "RBAC"
```

Fallback to a Python repository search if `rg` is unavailable.

Collect a bounded set of likely relevant files.

Respect:

```yaml
planning:
  max_files: 30
  max_context_bytes: 300000
```

Never send:

```text
.git/
node_modules/
vendor/
.venv/
dist/
build/
coverage/
binary files
```

unless explicitly required.

Honor `.gitignore` where practical.

---

# Planning Agent

Use OpenAI for planning.

The planner is read-only.

It must not modify files.

Build a planning request containing:

```text
project item metadata
issue number
title
issue body
acceptance criteria if detectable
repository instructions
repository tree summary
relevant file paths
relevant file contents
test files
current git status
```

Use structured model output.

Define Pydantic models similar to:

```python
class RelevantFile(BaseModel):
    path: str
    reason: str
    expected_change: str | None = None


class ProposedChange(BaseModel):
    file: str | None = None
    description: str
    risk: str | None = None


class AcceptanceCriterion(BaseModel):
    criterion: str
    validation: str | None = None


class HandoffPlan(BaseModel):
    summary: str
    objective: str
    assumptions: list[str]
    relevant_files: list[RelevantFile]
    proposed_changes: list[ProposedChange]
    preparation: list[str]
    implementation_steps: list[str]
    validation_steps: list[str]
    risks: list[str]
    acceptance_criteria: list[AcceptanceCriterion]
```

Use OpenAI structured output where supported.

Do not rely on arbitrary Markdown from the model.

The flow should be:

```text
OpenAI response
    ↓
Pydantic validation
    ↓
deterministic Markdown renderer
```

Retry malformed model output only within reasonable limits.

---

# Handoff Markdown

Generate files using:

```text
handoff/ready/<issue>-<slug>.md
```

Example:

```text
handoff/ready/142-add-refresh-token-rotation.md
```

Use predictable slugs.

Include frontmatter:

```yaml
---
zoro_version: 0.1.0
issue: 142
repository: nenjotech/my-project
project_item_id: PVTI_xxxxx
status: ready
generated_at: 2026-08-22T16:30:17+08:00
planner: openai
model: gpt-5.6
---
```

Markdown structure:

```text
# Issue title

## Objective

## Issue Context

## Acceptance Criteria

## Repository Analysis

### Relevant Files

## Preparation

## Proposed Changes

## Implementation Plan

## Validation

## Risks

## Implementation Constraints

## Definition of Done
```

Ensure acceptance criteria are represented as Markdown task items.

Example:

```markdown
- [ ] Refresh tokens are rotated after use.
- [ ] Previous refresh tokens become invalid.
- [ ] Existing RBAC behavior remains unchanged.
- [ ] Authentication tests cover rotation.
```

Do not fabricate acceptance criteria.

If none are explicitly present, state that and derive only clearly marked implementation checks.

---

# Handoff State Machine

Use directories as state:

```text
handoff/
├── ready/
├── implementing/
├── review/
├── done/
└── failed/
```

State mapping:

```text
GitHub Ready
    ↔
handoff/ready

GitHub In progress
    ↔
handoff/implementing

GitHub In review
    ↔
handoff/review

GitHub Done
    ↔
handoff/done
```

On implementation failure:

```text
handoff/failed
```

Do not automatically mark the GitHub item `Done`.

---

# `zoro implement`

When called without an argument:

```bash
zoro implement
```

scan:

```text
handoff/ready/
```

and show an interactive selector.

Example:

```text
Select a handoff to implement:

❯ #142 Add refresh token rotation
  #147 Add recipe validation
  #151 Fix admin pagination
```

Use `questionary` or an equivalent lightweight terminal selector.

When selected:

1. Validate repository safety.
2. Move handoff to `handoff/implementing/`.
3. Update GitHub status:
   ```text
   Ready -> In progress
   ```
4. Create a Git branch if enabled.
5. Invoke Codex.
6. Run configured validation commands.
7. On success:
   - move handoff to `handoff/review/`
   - update GitHub status to `In review`
8. On failure:
   - move handoff to `handoff/failed/`
   - retain useful failure metadata
   - do not mark as review/done

Support direct selection:

```bash
zoro implement 142
```

---

# Git Branch Strategy

When enabled:

```yaml
implementation:
  branch:
    enabled: true
    prefix: zoro
```

create:

```text
zoro/<issue>-<slug>
```

Example:

```text
zoro/142-add-refresh-token-rotation
```

Do not delete existing user branches.

If the target branch already exists:

- detect it
- report it
- do not silently overwrite it

For MVP, use the current branch as the base unless configuration explicitly introduces a base branch.

Before branch creation, verify the repository is clean.

---

# Codex Integration

Use the installed Codex CLI.

Do not implement an internal coding agent.

Zoro provides orchestration and context.

Codex performs repository modifications.

Create a dedicated `codex.py` abstraction.

Responsibilities:

```text
verify Codex CLI exists
build invocation
invoke Codex
capture exit code
capture useful stdout/stderr
handle cancellation
return structured result
```

Do not assume a hardcoded Codex binary path.

Resolve from `PATH`.

Use a current supported non-interactive Codex invocation.

Do not expose secrets in the prompt or logs.

Pass the selected handoff as the primary implementation instruction.

Codex should be told:

```text
Follow repository instructions.
Implement only the requested handoff.
Inspect existing code before editing.
Do not refactor unrelated code.
Run appropriate tests.
Preserve user changes.
Report affected files and validation.
```

---

# Validation

After Codex finishes successfully, execute configured commands in order.

Example:

```yaml
validation:
  commands:
    - uv run pytest
    - uv run ruff check .
```

Stop on first failing command.

Record:

```text
command
exit code
stdout summary
stderr summary
duration
```

Do not claim success unless all required configured validation commands pass.

If no validation commands are configured, report that validation was not performed.

---

# Failure Handling

Use structured errors.

Suggested error categories:

```text
ConfigError
AuthError
GitHubError
ProjectError
RepositoryError
PlannerError
HandoffError
CodexError
ValidationError
LockError
```

CLI errors should be readable.

Example:

```text
GitHub project status field "Status" was found, but required value
"In review" does not exist.

Configured value:
  statuses.review: In review

Available values:
  Backlog
  Ready
  In progress
  Review
  Done
```

Return non-zero exit codes for failures.

---

# Logging

Prefer concise human-readable Rich output for the terminal.

Example:

```text
[16:30:00] Checking project
[16:30:01] Found 4 Ready items
[16:30:01] Selected #142 Add refresh token rotation
[16:30:02] Inspecting repository
[16:30:05] Found 8 relevant files
[16:30:06] Creating implementation plan
[16:30:17] Handoff created
             handoff/ready/142-add-refresh-token-rotation.md
```

Avoid noisy debug logs by default.

Support a verbose mode if simple to add:

```bash
zoro --verbose run
```

Never log secrets.

---

# Automatic Mode

Support:

```yaml
automation:
  auto_plan: true
  auto_implement: false
```

Behavior:

```text
Ready item
    ↓
generate handoff
    ↓
stop and wait for developer
```

With:

```yaml
automation:
  auto_plan: true
  auto_implement: true
```

continue:

```text
Ready item
    ↓
handoff generated
    ↓
move to In progress
    ↓
Codex
    ↓
validation
    ↓
move to In review
```

Do not auto-transition to `Done`.

---

# Human Approval Model

Preserve these human-controlled transitions:

```text
Backlog -> Ready
```

Human indicates that the work item is sufficiently defined.

```text
In review -> Done
```

Human accepts the implementation.

Zoro should primarily own:

```text
Ready
  ->
In progress
  ->
In review
```

---

# Idempotency

Planning the same item twice should not create duplicate handoffs by default.

Use identifying metadata such as:

```text
project_item_id
issue number
repository
```

If:

```text
handoff/ready/142-*.md
handoff/implementing/142-*.md
handoff/review/142-*.md
handoff/done/142-*.md
```

already exists, treat the item as already known.

`handoff/failed` may be eligible for retry through an explicit command later.

For MVP, do not automatically retry failed implementations indefinitely.

---

# README

Document at least:

```text
What is zoro.ai
Requirements
Installation
GitHub authentication
OpenAI configuration
Codex requirement
zoro init
config.yaml
zoro doctor
zoro board
zoro plan
zoro implement
zoro run
zoro run --once
automatic mode
handoff lifecycle
security considerations
```

Example installation:

```bash
uv tool install .
```

or equivalent.

Example:

```bash
export OPENAI_API_KEY="..."
gh auth login

zoro init
zoro doctor
zoro board
zoro run
```

Do not place real token examples in documentation.

---

# Testing Requirements

Add unit tests for important behavior.

At minimum:

## Configuration

- valid config loads
- unknown duration fails
- invalid project number fails
- missing required status mapping fails

## GitHub

Mock HTTP requests.

Test:

- project parsing
- status parsing
- ordered Ready selection
- missing status field
- status update mutation
- authentication failure

## Repository

Test:

- detects Git repository
- detects dirty working tree
- branch name generation
- relevant file filtering
- ignored directories excluded

## Handoff

Test:

- deterministic filename
- frontmatter generation
- required sections
- Pydantic planner output renders correctly
- duplicate detection

## Runner

Test:

- no Ready items
- picks only top Ready item
- existing handoff is skipped
- auto implementation disabled
- auto implementation enabled
- lock prevents duplicate runs
- failure does not move to review
- success moves to review

Mock OpenAI and Codex. Do not call external services from tests.

---

# Security Requirements

Do not:

```text
store GitHub token in repo
store OpenAI API key in repo
print secrets
send unnecessary repository files to OpenAI
include .env contents in planner context
include SSH keys
include credential files
include .git contents
auto-stash developer work
force-reset Git state
force-push
delete user branches
```

Explicitly exclude files commonly containing secrets:

```text
.env
.env.*
*.pem
*.key
id_rsa
id_ed25519
credentials*
secrets*
```

Repository instructions may override file inclusion only when safe and intentional.

---

# Important Implementation Principles

Prefer the smallest correct solution.

Keep business logic independent of Typer where practical.

Example:

```text
cli.py
  calls
runner.py
  calls
github.py / planner.py / repository.py / handoff.py / codex.py
```

Do not place all logic directly in CLI command functions.

Use typed Pydantic models for:

```text
configuration
project items
planner response
handoff metadata
Codex result
validation result
```

Use async only where it meaningfully improves implementation.

Do not convert the whole CLI to async merely because `httpx` supports it.

A synchronous MVP is acceptable and likely simpler.

---

# Suggested Implementation Order

Implement in this sequence:

1. Project skeleton and dependencies.
2. Configuration model and `zoro init`.
3. Git repository utilities.
4. GitHub authentication.
5. GitHub Project querying.
6. `zoro doctor`.
7. `zoro board`.
8. `zoro ready`.
9. Repository context discovery.
10. OpenAI planner.
11. Structured plan model.
12. Handoff Markdown renderer.
13. `zoro plan`.
14. Runtime locking.
15. `zoro run --once`.
16. Polling `zoro run`.
17. Codex integration.
18. `zoro implement`.
19. Git branch handling.
20. Validation commands.
21. GitHub status synchronization.
22. Tests.
23. README and cleanup.

Do not begin with the continuous scheduler. Prove `zoro run --once` first.

---

# Acceptance Criteria

- [ ] `zoro init` creates valid configuration and handoff directories.
- [ ] GitHub credentials are never stored in the repository.
- [ ] `zoro auth` verifies GitHub access.
- [ ] `zoro doctor` validates the environment.
- [ ] `zoro board` reads the configured GitHub Project.
- [ ] `zoro ready` returns ordered Ready items.
- [ ] Zoro selects the top Ready project item deterministically.
- [ ] Repository inspection identifies a bounded set of relevant files.
- [ ] OpenAI planning uses structured output.
- [ ] Planner output is validated through Pydantic.
- [ ] Handoff Markdown is rendered deterministically.
- [ ] Generated handoffs are saved under `handoff/ready/`.
- [ ] Duplicate handoffs are not generated automatically.
- [ ] `zoro run --once` performs exactly one cycle.
- [ ] `zoro run` supports configurable polling intervals.
- [ ] Only one automatic Zoro process can operate on a repository at once.
- [ ] `zoro implement` lists handoffs from `handoff/ready/`.
- [ ] A developer can select a handoff interactively.
- [ ] Automatic implementation invokes Codex when enabled.
- [ ] Manual implementation invokes Codex when selected.
- [ ] Implementation refuses to start on a dirty working tree.
- [ ] Zoro creates an isolated Git branch when configured.
- [ ] GitHub status changes from Ready to In progress when implementation starts.
- [ ] Configured validation commands run after Codex.
- [ ] Failed validation does not move the item to In review.
- [ ] Successful validation moves the handoff to `handoff/review/`.
- [ ] Successful validation updates GitHub status to In review.
- [ ] Zoro never automatically marks an item Done.
- [ ] Important behavior is covered by pytest.
- [ ] External GitHub, OpenAI, and Codex interactions are mocked in tests.
- [ ] `ruff` passes.
- [ ] Tests pass.
- [ ] README explains the complete workflow.

---

# Definition of Done

The MVP is complete when this flow works end-to-end:

```bash
export OPENAI_API_KEY="..."
gh auth login

zoro init
zoro doctor

zoro board
zoro ready

zoro plan
```

Result:

```text
handoff/ready/142-add-refresh-token-rotation.md
```

Then:

```bash
zoro implement
```

allows the developer to select that handoff.

Zoro then:

```text
validates clean Git state
creates a zoro/<issue>-<slug> branch
moves the GitHub card to In progress
moves handoff to handoff/implementing
invokes Codex
runs configured validations
moves handoff to handoff/review on success
moves the GitHub card to In review on success
```

Finally:

```bash
zoro run
```

must continuously perform the same planning cycle at the configured interval, with automatic Codex execution occurring only when:

```yaml
automation:
  auto_implement: true
```

Keep the implementation simple, typed, testable, secure, and easy for future agents to understand.