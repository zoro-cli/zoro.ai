# Codex Handoff: Build `zoro.ai` CLI Agentic Development Orchestrator in Go

## Objective

Build the first production-grade MVP of **zoro.ai** as a local-first **Go CLI** that integrates with GitHub Projects and implements an agentic software development loop.

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

The CLI should remain simple, local-first, Git-native, deterministic, secure, easy to distribute as a single binary, and easy to extend.

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

Do not add a database.

Do not add Redis, queues, workers, or other infrastructure.

---

# Technology Stack

Use:

```text
Go 1.24+
Cobra
gopkg.in/yaml.v3
github.com/openai/openai-go
github.com/gofrs/flock
github.com/charmbracelet/huh
github.com/stretchr/testify
```

Prefer the Go standard library whenever practical:

```text
net/http
encoding/json
os
os/exec
path/filepath
context
time
regexp
sync
bytes
strings
```

Use direct GitHub GraphQL and REST requests through `net/http`.

Do not introduce a large GitHub SDK unless there is a demonstrated need.

Use the local `git` CLI through `os/exec` for repository operations.

Use the local Codex CLI for implementation.

Use normal Go modules:

```bash
go mod init github.com/zoro-cli/zoro.ai
go mod tidy
```

The final CLI should compile to a standalone binary:

```bash
go build -o zoro ./cmd/zoro
```

---

# Recommended Repository Structure

Keep the implementation relatively flat and idiomatic.

```text
zoro-ai/
├── cmd/
│   └── zoro/
│       └── main.go
│
├── internal/
│   ├── cli/
│   │   ├── root.go
│   │   ├── init.go
│   │   ├── auth.go
│   │   ├── doctor.go
│   │   ├── board.go
│   │   ├── ready.go
│   │   ├── plan.go
│   │   ├── implement.go
│   │   ├── run.go
│   │   ├── status.go
│   │   └── config.go
│   │
│   ├── config/
│   │   └── config.go
│   ├── github/
│   │   └── client.go
│   ├── repository/
│   │   └── repository.go
│   ├── planner/
│   │   └── planner.go
│   ├── handoff/
│   │   └── handoff.go
│   ├── codex/
│   │   └── codex.go
│   ├── runner/
│   │   └── runner.go
│   ├── validation/
│   │   └── validation.go
│   └── app/
│       └── errors.go
│
├── tests/
│   └── fixtures/
│
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```

Avoid unnecessary architecture such as:

```text
ports/
adapters/
repositories/
usecases/
domain/
infrastructure/
```

unless a real need emerges.

The CLI command layer should stay thin. Business logic belongs under `internal/`.

---

# CLI Commands

Implement these commands for MVP:

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

Plan a specific issue or project item.

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

Implement `--once` first and use the same runner operation internally from continuous mode.

---

# Initialization

`zoro init` should create:

```text
.zoro/
├── config.yaml
└── runtime/

handoff/
├── ready/
├── implementing/
├── review/
├── done/
└── failed/
```

Add:

```text
.zoro/runtime/
```

to `.gitignore` if it is not already ignored.

Do not automatically ignore the handoff directory. Handoffs are intended to be Git-trackable.

`zoro init` should detect the current Git repository before writing config.

Resolve:

```text
owner
repository
remote URL
```

from the current repository when possible.

For example:

```bash
git remote get-url origin
```

Support common remote formats:

```text
https://github.com/OWNER/REPO.git
git@github.com:OWNER/REPO.git
ssh://git@github.com/OWNER/REPO.git
```

Do not write placeholder values such as:

```text
OWNER/REPOSITORY
```

when the current Git repository can be resolved automatically.

If GitHub authentication is required to discover the Project, fail with a clear message instead of writing an invalid configuration.

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
      - go test ./...
      - go vet ./...

handoff:
  directory: handoff

behavior:
  max_concurrent_tasks: 1
  move_to_in_progress_on_implement: true
  move_to_review_on_success: true
```

Use typed Go structs.

Example:

```go
type Config struct {
    Version        int                  `yaml:"version"`
    GitHub         GitHubConfig         `yaml:"github"`
    Scheduler      SchedulerConfig      `yaml:"scheduler"`
    Planning       PlanningConfig       `yaml:"planning"`
    Automation     AutomationConfig     `yaml:"automation"`
    Implementation ImplementationConfig `yaml:"implementation"`
    Handoff        HandoffConfig        `yaml:"handoff"`
    Behavior       BehaviorConfig       `yaml:"behavior"`
}
```

Use `time.ParseDuration` for:

```text
30s
1m
5m
15m
1h
```

Reject zero, negative, malformed, or unsafe durations.

Validate at least:

```text
version
github.owner
github.repo
github.project_number > 0
github.status_field
all required status mappings
scheduler.interval
planning.model
planning.max_files > 0
planning.max_context_bytes > 0
handoff.directory
behavior.max_concurrent_tasks
```

Do not silently accept invalid configuration.

---

# Authentication

Do not store secrets in `.zoro/config.yaml`.

Resolve the GitHub token using this precedence:

```text
1. ZORO_GITHUB_TOKEN
2. GH_TOKEN
3. gh auth token
```

Implement token resolution through a small abstraction:

```go
type TokenResolver interface {
    ResolveGitHubToken(ctx context.Context) (string, error)
}
```

The concrete implementation may simply be a function instead of an interface if no mocking need emerges.

`zoro auth` should:

1. Resolve credentials.
2. Verify GitHub authentication.
3. Verify access to the configured repository.
4. Verify access to the configured GitHub Project.
5. Return clear structured errors.

Never print the token.

Never log the token.

Avoid storing the token in long-lived global variables.

---

# `zoro doctor`

Implement diagnostic output similar to:

```text
zoro.ai doctor

Git repository       ✓
Repository clean     ✓
Git executable       ✓
GitHub CLI           ✓
GitHub auth          ✓
GitHub repository    ✓
GitHub project       ✓
Project Status field ✓
OpenAI API key       ✓
Codex CLI            ✓
Go runtime           ✓
Configuration        ✓
Handoff directories  ✓
```

Check at least:

```text
inside Git repository
git executable available
working tree status
GitHub credentials
configured repository access
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

Use exit status:

```text
0 = all required checks passed
1 = one or more required checks failed
```

Warnings should not necessarily fail the command.

---

# GitHub Client

Implement a small dedicated GitHub client.

Suggested shape:

```go
type Client struct {
    HTTPClient *http.Client
    Token      string
    GraphQLURL string
    RESTURL    string
}
```

Prefer dependency injection for the HTTP client so tests can use `httptest.Server`.

Default endpoints:

```text
https://api.github.com/graphql
https://api.github.com
```

Do not hardcode them throughout the codebase.

Create a shared request helper that adds:

```http
Authorization: Bearer <token>
Accept: application/vnd.github+json
User-Agent: zoro.ai/<version>
```

Do not log Authorization headers.

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
read item body or issue body
read item node ID
read project item ID
read status
read position/order
read repository metadata
update project status
```

Suggested domain model:

```go
type ProjectItem struct {
    ID          string
    ContentID   string
    IssueNumber int
    Title       string
    Body        string
    Status      string
    Position    int
    Repository  string
}
```

The exact GraphQL response structs should remain internal to the GitHub package.

---

# GitHub Project Owner Resolution

GitHub Projects v2 may belong to either:

```text
User
Organization
```

Do not assume the configured owner is always a GitHub user.

When resolving a project, try the correct owner type deterministically.

Recommended approach:

1. Query the repository and obtain:
   ```text
   repository.owner.__typename
   ```
2. If owner type is:
   ```text
   Organization
   ```
   use the organization's `projectV2(number:)`.
3. If owner type is:
   ```text
   User
   ```
   use the user's `projectV2(number:)`.

Do not produce GraphQL queries that simultaneously assume both a user and organization with the same login when that causes misleading errors.

Return errors such as:

```text
GitHub repository owner "zoro-cli" is an Organization, but project 1 was not found or is not accessible.
```

This specifically avoids errors such as:

```text
Could not resolve to a User with the login of 'zoro-cli'
```

when the owner is actually an organization.

---

# Project Ordering

For `Ready`, preserve GitHub Project ordering when the API exposes usable order metadata.

The top item in the visible `Ready` column should be treated as the next item to process.

Do not sort by:

```text
issue number
creation date
alphabetical title
random ordering
```

unless project ordering is unavailable.

If the Projects API response does not expose sufficient column ordering information, document the exact fallback and use a deterministic fallback.

A fallback may use returned project item connection order only if verified to be stable for the query.

Do not silently invent ordering semantics.

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

The ordering must match the deterministic Ready ordering used by the runner.

---

# Polling Model

Do not refer to the internal scheduler as cron.

The normal mode is a polling loop.

Conceptually:

```go
for {
    if err := runner.RunOnce(ctx); err != nil {
        // report error
    }

    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-time.After(interval):
    }
}
```

Prefer a `time.Ticker` for continuous mode.

Use signal-aware cancellation:

```text
SIGINT
SIGTERM
```

Use:

```go
signal.NotifyContext
```

where practical.

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

`zoro run` should repeatedly call the same `RunOnce` operation.

Do not duplicate behavior between `run` and `run --once`.

---

# Locking and Duplicate Protection

Use an OS-level repository lock:

```text
.zoro/runtime/zoro.lock
```

Use:

```text
github.com/gofrs/flock
```

Only one active automatic cycle should run against a repository at once.

Protect against:

```text
two zoro run processes
manual zoro implement while automatic implementation is active
duplicate handoffs for the same project item
duplicate Codex execution
```

Acquire locks with `TryLock` or a bounded context-aware mechanism.

Do not block forever waiting for the lock.

Return a clear error:

```text
Another Zoro operation is already active for this repository.
```

Do not implement distributed locking.

---

# Repository Safety

Before automatic or manual implementation:

```bash
git status --porcelain
```

must be clean.

If the working tree contains developer changes, do not auto-stash them.

Return an error similar to:

```text
Cannot start implementation.

Repository contains uncommitted changes:

 M app/auth.go
?? notes.txt

Commit, stash, or remove these changes first.
```

Planning may still be allowed on a dirty repository.

The generated handoff should record that the repository contained local changes at planning time.

Implementation must not overwrite developer changes.

Never run:

```text
git reset --hard
git clean -fd
git stash
git checkout -- .
git restore .
```

automatically against user changes.

---

# Repository Inspection

The planner should not receive the entire repository blindly.

Create a deterministic context collection step.

Inspect repository metadata files first:

```text
AGENTS.md
CLAUDE.md
README.md
go.mod
go.sum
pyproject.toml
package.json
Cargo.toml
Dockerfile
docker-compose.yml
docker-compose.yaml
.github/
docs/
```

Read whichever files exist.

Then use issue or project item text to identify repository keywords.

Search with `rg` if available.

Examples:

```bash
rg "refresh"
rg "access_token"
rg "cookie"
rg "RBAC"
```

Fallback to a native Go repository search if `rg` is unavailable.

The fallback should walk files with:

```go
filepath.WalkDir
```

and perform bounded text matching.

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
bin/
tmp/
binary files
```

unless explicitly required.

Honor `.gitignore` where practical.

A practical MVP approach is to use:

```bash
git ls-files
```

as the primary tracked-file list, then supplement with selected untracked metadata only when necessary.

This naturally honors Git tracking and avoids many ignored files.

---

# Secret and Sensitive File Exclusion

Explicitly exclude:

```text
.env
.env.*
*.pem
*.key
id_rsa
id_ed25519
credentials*
secrets*
*.p12
*.pfx
.npmrc
.pypirc
.netrc
```

Do not include `.git/`.

Do not include SSH keys.

Do not include GitHub tokens.

Do not include OpenAI keys.

Do not include credential files in planner context.

Centralize the exclusion logic so repository searching and context collection share the same rules.

---

# Repository Context Model

Use typed Go structs.

Example:

```go
type ContextFile struct {
    Path    string `json:"path"`
    Content string `json:"content"`
    Reason  string `json:"reason"`
}

type RepositoryContext struct {
    Root             string        `json:"root"`
    GitStatus        string        `json:"git_status"`
    Dirty            bool          `json:"dirty"`
    Instructions     []ContextFile `json:"instructions"`
    RelevantFiles    []ContextFile `json:"relevant_files"`
    TreeSummary      []string      `json:"tree_summary"`
    TotalContextSize int           `json:"total_context_size"`
}
```

Maintain deterministic file ordering.

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

Use JSON structured output.

Define Go structs similar to:

```go
type RelevantFile struct {
    Path           string  `json:"path"`
    Reason         string  `json:"reason"`
    ExpectedChange *string `json:"expected_change,omitempty"`
}

type ProposedChange struct {
    File        *string `json:"file,omitempty"`
    Description string  `json:"description"`
    Risk        *string `json:"risk,omitempty"`
}

type AcceptanceCriterion struct {
    Criterion string  `json:"criterion"`
    Validation *string `json:"validation,omitempty"`
}

type HandoffPlan struct {
    Summary             string                `json:"summary"`
    Objective           string                `json:"objective"`
    Assumptions         []string              `json:"assumptions"`
    RelevantFiles       []RelevantFile        `json:"relevant_files"`
    ProposedChanges     []ProposedChange      `json:"proposed_changes"`
    Preparation         []string              `json:"preparation"`
    ImplementationSteps []string              `json:"implementation_steps"`
    ValidationSteps     []string              `json:"validation_steps"`
    Risks               []string              `json:"risks"`
    AcceptanceCriteria  []AcceptanceCriterion `json:"acceptance_criteria"`
}
```

Use the OpenAI Go SDK structured output support where practical.

If the SDK API is unsuitable for a required feature, use direct HTTPS to the OpenAI API through `net/http`.

Do not allow arbitrary Markdown from the model to become the handoff directly.

The flow should be:

```text
OpenAI response
    ↓
JSON decode
    ↓
Go struct validation
    ↓
deterministic Markdown renderer
```

Go does not provide Pydantic-style validation natively.

Implement explicit validation methods:

```go
func (p HandoffPlan) Validate() error
```

Validate required strings, lengths, and collection consistency.

Retry malformed model output only within reasonable limits.

---

# Planner Interface

Keep planning testable.

Suggested interface:

```go
type Planner interface {
    Plan(ctx context.Context, req PlanRequest) (HandoffPlan, error)
}
```

The production implementation uses OpenAI.

Unit tests use a fake planner.

Do not call OpenAI from unit tests.

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

Slug rules should be deterministic:

```text
lowercase
letters and numbers only
spaces and underscores -> hyphens
collapse repeated hyphens
trim leading/trailing hyphens
bounded length
```

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

Use RFC3339 timestamps:

```go
time.Now().Format(time.RFC3339)
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

If none are explicitly present, state that no explicit acceptance criteria were found.

Any planner-derived checks must be clearly labeled as implementation checks, not issue acceptance criteria.

---

# Handoff Metadata Model

Suggested model:

```go
type Metadata struct {
    ZoroVersion   string
    Issue         int
    Repository    string
    ProjectItemID string
    Status        string
    GeneratedAt   time.Time
    Planner       string
    Model         string
}
```

Do not parse arbitrary YAML frontmatter with regex alone.

Use a small deterministic frontmatter parser or `yaml.v3`.

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

Implement file transitions using `os.Rename` when source and destination are on the same filesystem.

Check destination existence first.

Do not silently overwrite an existing handoff.

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

Use:

```text
github.com/charmbracelet/huh
```

or another lightweight Go terminal selector.

When selected:

1. Acquire the repository operation lock.
2. Validate repository safety.
3. Move handoff to `handoff/implementing/`.
4. Update GitHub status:
   ```text
   Ready -> In progress
   ```
5. Create a Git branch if enabled.
6. Invoke Codex.
7. Run configured validation commands.
8. On success:
   - move handoff to `handoff/review/`
   - update GitHub status to `In review`
9. On failure:
   - move handoff to `handoff/failed/`
   - retain useful failure metadata
   - do not mark as review or done

Support direct selection:

```bash
zoro implement 142
```

If multiple matching files exist for the same issue, return an ambiguity error instead of choosing randomly.

---

# Transaction-Like Implementation Ordering

GitHub state and local filesystem state cannot participate in a real transaction.

Use a deliberate ordering and compensating behavior.

Recommended implementation start order:

```text
1. acquire lock
2. validate clean Git state
3. validate handoff
4. validate target branch availability
5. move handoff ready -> implementing
6. update GitHub Ready -> In progress
7. create branch
8. invoke Codex
```

If GitHub update fails after the file move:

```text
attempt to move handoff back to ready
return an error
```

If branch creation fails:

```text
move handoff to failed or restore ready based on whether implementation actually started
include failure reason
do not continue to Codex
```

On successful Codex and validation:

```text
1. move handoff implementing -> review
2. update GitHub In progress -> In review
```

If the final GitHub update fails, do not claim full success.

Return a partial-success error explaining that local implementation succeeded but board synchronization failed.

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

Before creating:

```bash
git show-ref --verify --quiet refs/heads/<branch>
```

or equivalent.

If the target branch already exists:

```text
detect it
report it
do not silently overwrite it
```

For MVP, use the current branch as the base unless configuration explicitly introduces a base branch.

Before branch creation, verify the repository is clean.

Suggested helper methods:

```go
func CurrentBranch(ctx context.Context, root string) (string, error)
func BranchExists(ctx context.Context, root, branch string) (bool, error)
func CreateBranch(ctx context.Context, root, branch string) error
```

---

# Codex Integration

Use the installed Codex CLI.

Do not implement an internal coding agent.

Zoro provides orchestration and context.

Codex performs repository modifications.

Create a dedicated package:

```text
internal/codex
```

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

Resolve with:

```go
exec.LookPath("codex")
```

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

Suggested result:

```go
type Result struct {
    ExitCode int
    Stdout   string
    Stderr   string
    Duration time.Duration
}
```

Bound captured output to avoid unbounded memory use.

---

# Command Execution

Create a shared command runner abstraction.

Example:

```go
type CommandRunner interface {
    Run(ctx context.Context, dir string, name string, args ...string) (Result, error)
}
```

Use it for:

```text
git
gh
rg
codex
validation commands
```

This allows tests to mock process execution.

For validation commands stored as strings, avoid naïvely splitting with `strings.Fields` because quoted arguments would break.

Two acceptable MVP approaches:

1. Execute validation through the platform shell explicitly and document the behavior.
2. Change configuration to an argv representation.

Recommended cross-platform config:

```yaml
validation:
  commands:
    - name: tests
      command: go
      args: ["test", "./..."]
    - name: vet
      command: go
      args: ["vet", "./..."]
```

If compatibility with the original string format is required, implement shell execution carefully:

```text
Windows: cmd.exe /C
Unix: /bin/sh -c
```

Do not use shell execution for GitHub tokens or other secret-bearing command construction.

---

# Validation

After Codex finishes successfully, execute configured commands in order.

Recommended Go project defaults:

```yaml
implementation:
  validation:
    enabled: true
    commands:
      - go test ./...
      - go vet ./...
```

For the `zoro.ai` repository itself, also run:

```bash
gofmt -w .
go test ./...
go vet ./...
```

Do not automatically run `gofmt -w .` as a post-Codex validation command against arbitrary user repositories unless explicitly configured.

Stop on the first failing command.

Record:

```text
command
exit code
stdout summary
stderr summary
duration
```

Suggested model:

```go
type ValidationResult struct {
    Command  string
    ExitCode int
    Stdout   string
    Stderr   string
    Duration time.Duration
}
```

Do not claim success unless all required configured validation commands pass.

If no validation commands are configured, explicitly report:

```text
Validation was not performed because no commands are configured.
```

---

# Failure Handling

Use typed wrapped Go errors.

Suggested sentinel categories:

```go
var (
    ErrConfig     = errors.New("config error")
    ErrAuth       = errors.New("authentication error")
    ErrGitHub     = errors.New("github error")
    ErrProject    = errors.New("project error")
    ErrRepository = errors.New("repository error")
    ErrPlanner    = errors.New("planner error")
    ErrHandoff    = errors.New("handoff error")
    ErrCodex      = errors.New("codex error")
    ErrValidation = errors.New("validation error")
    ErrLock       = errors.New("lock error")
)
```

Wrap errors with context:

```go
return fmt.Errorf("%w: status field %q not found", ErrProject, fieldName)
```

Do not build a large error hierarchy.

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

Map categories to stable exit codes if practical.

---

# Logging and Terminal Output

Prefer concise human-readable terminal output.

Do not require a heavy logging framework.

A small logger abstraction is enough:

```go
type Logger struct {
    Verbose bool
}
```

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

Support:

```bash
zoro --verbose run
```

Never log secrets.

Avoid printing complete OpenAI or GitHub payloads in verbose mode when they may include private repository content.

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

Search state directories:

```text
handoff/ready/
handoff/implementing/
handoff/review/
handoff/done/
```

If the same item exists in any of those states, treat it as already known.

`handoff/failed` may be eligible for retry only through an explicit command later.

For MVP, do not automatically retry failed implementations indefinitely.

Do not rely only on filename matching.

Prefer parsing handoff frontmatter and matching:

```text
repository
project_item_id
issue
```

Filename matching may be used as a fast pre-filter.

---

# Runner

Keep orchestration in:

```text
internal/runner
```

Suggested dependencies:

```go
type Runner struct {
    Config     config.Config
    GitHub     GitHubService
    Repository RepositoryService
    Planner    planner.Planner
    Handoffs   HandoffService
    Implementer ImplementService
    Lock       Lock
    Logger     Logger
}
```

Avoid interfaces for every tiny helper.

Introduce an interface only where:

```text
external I/O must be mocked
multiple implementations genuinely exist
test isolation materially improves
```

Suggested primary method:

```go
func (r *Runner) RunOnce(ctx context.Context) error
```

Continuous mode belongs in a thin wrapper:

```go
func (r *Runner) Run(ctx context.Context, interval time.Duration) error
```

`Run` should call `RunOnce`.

---

# Context and Cancellation

All external I/O should accept `context.Context`.

At minimum:

```text
GitHub HTTP requests
OpenAI requests
Codex execution
git subprocesses
gh subprocesses
validation commands
continuous run loop
```

Cobra commands should derive context from:

```go
cmd.Context()
```

The root command should install signal cancellation.

---

# HTTP Safety

Use a configured HTTP client with timeouts.

Example:

```go
&http.Client{
    Timeout: 30 * time.Second,
}
```

For longer OpenAI requests, use context deadlines appropriate to the planner operation instead of an unbounded client.

Check non-2xx responses before JSON decoding.

Bound response bodies where practical.

Return GitHub errors with:

```text
HTTP status
GitHub message
request context
```

without exposing headers or tokens.

---

# `zoro status`

Display local orchestration state.

At minimum:

```text
Current repository
Configured GitHub project
Scheduler enabled
Polling interval
Automation mode
Ready handoff count
Implementing handoff count
Review handoff count
Failed handoff count
Lock state
```

Do not query unnecessary external APIs if local state is sufficient.

An optional flag may refresh GitHub status later.

---

# `zoro config`

For MVP, support:

```bash
zoro config
```

to print the effective non-secret configuration.

Do not print tokens.

Optionally support:

```bash
zoro config path
```

to print:

```text
.zoro/config.yaml
```

Do not implement a complex interactive editor for MVP.

---

# README

Document at least:

```text
What is zoro.ai
Requirements
Installation
Building from source
GitHub authentication
OpenAI configuration
Codex requirement
zoro init
config.yaml
zoro doctor
zoro board
zoro ready
zoro plan
zoro implement
zoro run
zoro run --once
automatic mode
handoff lifecycle
security considerations
```

Example build:

```bash
go build -o zoro ./cmd/zoro
```

Example installation:

```bash
go install github.com/zoro-cli/zoro.ai/cmd/zoro@latest
```

when the repository is public and versioned.

Example workflow:

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

# Cross-Platform Requirements

The CLI should work on:

```text
Windows
Linux
macOS
```

Use:

```go
filepath.Join
os.UserHomeDir
exec.LookPath
```

Do not hardcode `/` path separators.

Do not assume Bash exists on Windows.

Do not assume executable names end in `.exe`. Let `exec.LookPath` resolve them.

If validation commands use shell strings, select shell behavior by `runtime.GOOS`.

Prefer direct executable plus argv execution where possible.

---

# Testing Requirements

Use the standard `testing` package.

Use:

```text
github.com/stretchr/testify
```

only where it improves assertions or mocks.

Prefer table-driven tests.

Do not call external services from tests.

Use:

```go
httptest.NewServer
t.TempDir
```

for isolated tests.

---

# Configuration Tests

Test:

```text
valid config loads
unknown duration fails
negative or zero duration fails
invalid project number fails
missing required status mapping fails
invalid max_files fails
invalid max_context_bytes fails
```

---

# GitHub Tests

Mock HTTP using `httptest.Server`.

Test:

```text
authentication verification
repository owner type resolution
organization project resolution
user project resolution
project parsing
status field parsing
ordered Ready selection
missing status field
missing required status option
status update mutation
authentication failure
GraphQL error parsing
```

Include a regression test for:

```text
repository owner is Organization
```

so Zoro does not incorrectly query it only as a GitHub User.

---

# Repository Tests

Use temporary Git repositories.

Initialize them with:

```bash
git init
```

Test:

```text
detects Git repository
detects non-Git directory
detects dirty working tree
detects clean working tree
parses HTTPS remote
parses SSH remote
branch name generation
branch collision detection
relevant file filtering
ignored directories excluded
secret files excluded
binary files excluded
context byte limit enforced
max file count enforced
```

Skip tests requiring Git only if Git is genuinely unavailable in the test environment.

---

# Handoff Tests

Test:

```text
deterministic filename
slug generation
frontmatter generation
frontmatter parsing
required sections
plan renders correctly
acceptance criteria rendered as task items
duplicate detection by metadata
state transition move
existing destination is not overwritten
```

---

# Planner Tests

Use a fake OpenAI client or HTTP test server.

Test:

```text
valid structured output
malformed JSON
missing required fields
retry limit
context size boundary
planner does not mutate repository
```

---

# Runner Tests

Test:

```text
no Ready items
picks only top Ready item
existing handoff is skipped
auto planning disabled
auto implementation disabled
auto implementation enabled
lock prevents duplicate runs
planner failure does not create handoff
Codex failure does not move to review
validation failure does not move to review
success moves to review
GitHub transition failure is surfaced
duplicate execution is prevented
```

Mock:

```text
GitHub
OpenAI planner
Codex
command execution
```

Do not call real GitHub, OpenAI, or Codex from unit tests.

---

# Concurrency Tests

Test the repository lock.

Attempt two operations against the same lock file.

Verify that only one succeeds.

Run with:

```bash
go test -race ./...
```

The project should remain race-free.

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

Explicitly exclude files commonly containing secrets.

Avoid passing secret values as command-line arguments when an environment variable or HTTP header is available.

Do not persist raw OpenAI request payloads by default.

---

# Important Implementation Principles

Prefer the smallest correct solution.

Keep business logic independent of Cobra.

Example:

```text
internal/cli
    calls

internal/runner
    calls

internal/github
internal/planner
internal/repository
internal/handoff
internal/codex
internal/validation
```

Do not place all logic directly inside Cobra `RunE` functions.

Use structs for:

```text
configuration
project items
planner response
handoff metadata
Codex result
validation result
```

Use concurrency only where it meaningfully improves implementation.

Do not parallelize repository scanning or API work prematurely.

The MVP should be deterministic and easy to debug.

---

# Suggested Implementation Order

Implement in this sequence:

1. Go module and project skeleton.
2. Cobra root command.
3. Configuration structs, parsing, validation, and `zoro init`.
4. Git repository utilities.
5. Git remote owner/repository detection.
6. GitHub token resolution.
7. GitHub authentication verification.
8. GitHub repository owner type resolution.
9. GitHub Project querying.
10. `zoro auth`.
11. `zoro doctor`.
12. `zoro board`.
13. `zoro ready`.
14. Repository context discovery.
15. OpenAI planner.
16. Structured plan validation.
17. Handoff Markdown renderer.
18. `zoro plan`.
19. Runtime locking.
20. `zoro run --once`.
21. Polling `zoro run`.
22. Codex integration.
23. `zoro implement`.
24. Git branch handling.
25. Validation commands.
26. GitHub status synchronization.
27. `zoro status`.
28. `zoro config`.
29. Tests.
30. README and cleanup.

Do not begin with the continuous scheduler.

Prove:

```bash
zoro run --once
```

first.

---

# Acceptance Criteria

- [ ] `zoro.ai` is implemented in Go.
- [ ] The project builds as a standalone CLI binary.
- [ ] `zoro init` creates valid configuration and handoff directories.
- [ ] `zoro init` detects owner and repository from the current Git remote when possible.
- [ ] GitHub credentials are never stored in the repository.
- [ ] `zoro auth` verifies GitHub access.
- [ ] GitHub Project lookup correctly handles User and Organization owners.
- [ ] `zoro doctor` validates the environment.
- [ ] `zoro board` reads the configured GitHub Project.
- [ ] `zoro ready` returns ordered Ready items.
- [ ] Zoro selects the top Ready project item deterministically.
- [ ] Repository inspection identifies a bounded set of relevant files.
- [ ] Secret and credential files are excluded from planner context.
- [ ] OpenAI planning uses structured JSON output.
- [ ] Planner output is decoded into typed Go structs and validated.
- [ ] Handoff Markdown is rendered deterministically.
- [ ] Generated handoffs are saved under `handoff/ready/`.
- [ ] Duplicate handoffs are not generated automatically.
- [ ] `zoro run --once` performs exactly one cycle.
- [ ] `zoro run` supports configurable polling intervals.
- [ ] `zoro run` exits cleanly on SIGINT and SIGTERM.
- [ ] Only one automatic Zoro process can operate on a repository at once.
- [ ] `zoro implement` lists handoffs from `handoff/ready/`.
- [ ] A developer can select a handoff interactively.
- [ ] Automatic implementation invokes Codex when enabled.
- [ ] Manual implementation invokes Codex when selected.
- [ ] Implementation refuses to start on a dirty working tree.
- [ ] Zoro creates an isolated Git branch when configured.
- [ ] Existing branches are never overwritten silently.
- [ ] GitHub status changes from Ready to In progress when implementation starts.
- [ ] Configured validation commands run after Codex.
- [ ] Failed validation does not move the item to In review.
- [ ] Successful validation moves the handoff to `handoff/review/`.
- [ ] Successful validation updates GitHub status to In review.
- [ ] Zoro never automatically marks an item Done.
- [ ] External GitHub, OpenAI, and Codex interactions are mocked in tests.
- [ ] `gofmt` passes.
- [ ] `go test ./...` passes.
- [ ] `go test -race ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] README explains the complete workflow.

---

# Validation for the Zoro Codebase

Before considering the implementation complete, run:

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go build -o zoro ./cmd/zoro
```

Do not claim these validations passed unless they were actually executed successfully.

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
acquires repository lock
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

Keep the implementation simple, typed, testable, secure, cross-platform, deterministic, and easy for future agents to understand.
