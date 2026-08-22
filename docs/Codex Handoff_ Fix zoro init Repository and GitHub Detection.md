# Codex Handoff: Fix `zoro init` Repository and GitHub Detection

## Objective

Fix and improve the `zoro init` command so it automatically detects the current GitHub repository, validates GitHub authentication and repository access, discovers the GitHub Project configuration, and writes a valid `.zoro/config.yaml`.

A successful `zoro init` must never leave placeholder values such as:

```yaml
github:
  owner: OWNER
  repo: REPOSITORY
```

The command must either initialize Zoro with valid detected values or fail safely without creating a broken configuration.

## Problem

Current behavior can produce configuration similar to:

```yaml
github:
  owner: OWNER
  repo: REPOSITORY
```

Then:

```bash
zoro auth
```

fails with:

```text
Error: Repository not found: OWNER/REPOSITORY
```

This is incorrect because the repository owner and repository name should normally be discoverable from the current Git repository.

For example:

```bash
git remote get-url origin
```

may return:

```text
git@github.com:nenjotech/my-project.git
```

or:

```text
https://github.com/nenjotech/my-project.git
```

From either form Zoro should derive:

```yaml
github:
  owner: nenjotech
  repo: my-project
```

## Architecture Decision

Repository identity and GitHub authentication are separate concerns.

```text
Git repository
    │
    └── determines
        owner/repository

GitHub credentials
    │
    └── verify
        authenticated user
        repository access
        project access
```

Do not require an API call merely to determine `owner` and `repo`.

Use the local Git remote first.

Authentication is required to verify that the detected repository and GitHub Project are actually accessible.

## Required `zoro init` Flow

Implement initialization in this order:

```text
zoro init
    │
    ├── Verify current directory is inside a Git repository
    │
    ├── Detect Git repository root
    │
    ├── Detect Git remote
    │
    ├── Parse GitHub owner/repository
    │
    ├── Resolve GitHub credentials
    │
    ├── Verify authenticated GitHub user
    │
    ├── Verify repository access
    │
    ├── Discover/list accessible GitHub Projects
    │
    ├── Allow user to select a project
    │
    ├── Inspect project Status field
    │
    ├── Validate/map workflow statuses
    │
    └── Create .zoro/config.yaml and handoff directories
```

Do not write the final configuration until the required initialization steps have succeeded.

## Step 1: Verify Git Repository

Run the equivalent of:

```bash
git rev-parse --show-toplevel
```

If this fails, return a clear error:

```text
Error: Current directory is not a Git repository.

Initialize Git first:

  git init

or run zoro inside an existing repository.
```

Do not create `.zoro/config.yaml`.

## Step 2: Detect Repository Root

Store the repository root returned by:

```bash
git rev-parse --show-toplevel
```

All further repository operations should execute relative to this root.

Do not assume the user ran `zoro init` from the repository root.

This should work:

```text
my-project/
├── app/
│   └── services/
│       └── current-directory
├── .git/
└── ...
```

If the user runs:

```bash
cd app/services
zoro init
```

Zoro should still initialize:

```text
my-project/.zoro/
```

not:

```text
my-project/app/services/.zoro/
```

## Step 3: Detect Git Remote

Prefer:

```bash
git remote get-url origin
```

If `origin` does not exist, inspect available remotes:

```bash
git remote
```

Behavior:

1. If exactly one remote exists, allow it to be used.
2. If multiple remotes exist and `origin` does not exist, ask the user which remote to use.
3. If no remotes exist, fail with actionable guidance.

Example:

```text
Git repository detected, but no Git remote exists.

Add a GitHub remote first:

  git remote add origin <repository-url>

Then run:

  zoro init
```

## Step 4: Parse GitHub Repository

Support at minimum these remote formats:

```text
git@github.com:OWNER/REPOSITORY.git
https://github.com/OWNER/REPOSITORY.git
https://github.com/OWNER/REPOSITORY
ssh://git@github.com/OWNER/REPOSITORY.git
```

Normalize:

```text
OWNER
REPOSITORY
```

Remove:

```text
.git
```

from the repository name.

Examples:

```text
git@github.com:nenjotech/zoro-ai.git
```

becomes:

```text
owner = nenjotech
repo = zoro-ai
```

And:

```text
https://github.com/company/platform.git
```

becomes:

```text
owner = company
repo = platform
```

Do not use fragile string splitting spread across CLI code.

Create a dedicated helper, for example:

```python
class GitHubRepositoryRef(BaseModel):
    owner: str
    repo: str
    remote_name: str
    remote_url: str
```

Suggested function:

```python
def parse_github_remote(
    remote_name: str,
    remote_url: str,
) -> GitHubRepositoryRef:
    ...
```

## Non-GitHub Remote

If the detected remote is not GitHub, fail clearly.

Example:

```text
Unsupported Git remote:

  git@gitlab.com:company/project.git

zoro.ai currently requires a GitHub repository.
```

Do not silently try to convert arbitrary remotes.

## Step 5: Resolve GitHub Credentials

Use the existing token resolution behavior, but ensure it is usable during `zoro init`.

Recommended precedence:

```text
1. ZORO_GITHUB_TOKEN
2. GH_TOKEN
3. gh auth token
```

If the application already has a shared credential resolver, reuse it.

Do not duplicate token resolution logic between:

```text
zoro init
zoro auth
zoro doctor
```

Create or preserve one shared abstraction.

For example:

```python
class GitHubCredentialResolver:
    def resolve(self) -> GitHubCredentials:
        ...
```

Never write credentials into `.zoro/config.yaml`.

Never print them.

Never log them.

## Missing Authentication

If no credentials can be resolved:

```text
Repository detected:
  nenjotech/my-project

GitHub authentication was not found.

Authenticate using:

  gh auth login

or set one of:

  ZORO_GITHUB_TOKEN
  GH_TOKEN

Then run:

  zoro init
```

Do not proceed with project discovery.

Do not create a finalized config.

## Step 6: Verify Authenticated GitHub User

Use the resolved credentials to verify GitHub authentication.

Obtain useful metadata such as:

```text
login
```

Expected output:

```text
GitHub
  ✓ Authentication detected
  ✓ Logged in as nenjotech
```

If authentication is invalid:

```text
GitHub authentication failed.

The configured token is invalid, expired, or unavailable.
```

Return non-zero status.

## Step 7: Verify Repository Access

Use the detected values:

```text
owner
repo
```

to verify repository access.

For example:

```text
nenjotech/my-project
```

The verification must distinguish as much as reasonably possible between:

```text
authentication failure
repository unavailable
insufficient repository permissions
```

Be aware GitHub may return `404` for private resources when the token does not have access.

Therefore the error should not incorrectly claim that the repository definitely does not exist.

Prefer:

```text
Unable to access GitHub repository:

  nenjotech/my-project

The repository may not exist, or the current GitHub credentials may not
have permission to access it.

Verify your token repository access and try again.
```

Expected success output:

```text
Repository
  ✓ Git repository detected
  ✓ Remote: git@github.com:nenjotech/my-project.git
  ✓ Owner: nenjotech
  ✓ Repository: my-project

GitHub
  ✓ Authentication detected
  ✓ Logged in as nenjotech
  ✓ Repository access verified
```

## Step 8: GitHub Project Discovery

After repository access succeeds, discover accessible GitHub Projects relevant to the configured owner.

The MVP may list projects belonging to:

```text
repository owner
authenticated user when appropriate
organization owner when applicable
```

Do not silently pick project number `1`.

Show an interactive selector.

Example:

```text
GitHub Projects

❯ Agentic Development
  Development
  Roadmap
```

or:

```text
GitHub Projects

1. Agentic Development
2. Development
3. Roadmap

Select project: 1
```

Use the existing interactive CLI library, preferably `questionary` if already included.

If exactly one suitable project is available, it is acceptable to ask:

```text
Detected GitHub Project:

  Agentic Development (#3)

Use this project? [Y/n]
```

Do not choose it silently unless the existing CLI style explicitly supports non-interactive defaults.

## Non-Interactive Behavior

If the command already supports or will support a non-interactive mode, it must not hang waiting for selection.

Allow explicit options such as:

```bash
zoro init --project 3
```

or equivalent if consistent with the current CLI.

Do not add broad CLI complexity solely for this handoff if non-interactive support does not already exist.

## Step 9: Detect Project Status Field

After project selection, inspect the project fields.

Default configured field:

```text
Status
```

Find its single-select options.

Expected workflow:

```text
Backlog
Ready
In progress
In review
Done
```

Do not assume capitalization or spacing without checking.

The initialization command should map known values where exact matches exist.

Example:

```text
Project
  ✓ Agentic Development (#3)
  ✓ Status field detected

Workflow
  ✓ Backlog
  ✓ Ready
  ✓ In progress
  ✓ In review
  ✓ Done
```

Then write:

```yaml
statuses:
  backlog: Backlog
  ready: Ready
  implementing: In progress
  review: In review
  done: Done
```

## Missing or Different Status Values

If the field exists but required statuses differ, provide actionable feedback.

Example:

```text
Project Status field was found, but the expected review status was not.

Available values:

  Backlog
  Ready
  In Progress
  Review
  Done
```

Where practical, allow the user to map:

```text
Zoro "In review" status -> Review
```

For the initial fix, if interactive mapping is too much scope, fail clearly and tell the user what values exist.

Do not write incorrect mappings.

## Step 10: Write Configuration

Only after successful repository detection, authentication, repository verification, project selection, and minimum project validation, create:

```text
<repository-root>/.zoro/config.yaml
```

Example:

```yaml
version: 1

github:
  owner: nenjotech
  repo: my-project
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
    commands: []

handoff:
  directory: handoff

behavior:
  max_concurrent_tasks: 1
  move_to_in_progress_on_implement: true
  move_to_review_on_success: true
```

Reuse existing defaults from the application rather than introducing conflicting values if they already exist.

The critical requirement is that these are real values:

```yaml
owner: nenjotech
repo: my-project
project_number: 3
```

Never:

```yaml
owner: OWNER
repo: REPOSITORY
```

## Handoff Directories

Create under the repository root:

```text
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

if runtime state is part of the current implementation.

Ensure runtime state is ignored by Git:

```gitignore
.zoro/runtime/
```

Do not automatically ignore:

```text
.zoro/config.yaml
handoff/
```

unless the current application intentionally treats them as local-only.

The handoff files are intended to be Git-trackable.

## Existing Configuration

Handle an existing:

```text
.zoro/config.yaml
```

safely.

Do not silently overwrite user configuration.

Recommended behavior:

```text
zoro.ai is already initialized.

Configuration:
  /path/to/repo/.zoro/config.yaml

Reinitialize? [y/N]
```

Default to `No`.

If reinitialization is accepted:

- preserve user settings where reasonable
- refresh repository-derived values
- do not erase unrelated configuration unnecessarily

At minimum, never overwrite without confirmation.

## Placeholder Migration

If existing config contains:

```yaml
github:
  owner: OWNER
  repo: REPOSITORY
```

treat it as incomplete rather than valid.

`zoro init` should detect this and offer repair:

```text
Existing Zoro configuration contains placeholder repository values.

Current:
  OWNER/REPOSITORY

Detected:
  nenjotech/my-project

Repair configuration? [Y/n]
```

If confirmed, replace the placeholders.

## `zoro auth` Integration

Do not make `zoro auth` depend blindly on placeholder configuration.

Use a shared repository resolver.

When config is missing or incomplete, `zoro auth` should be able to identify the repository from Git.

Suggested repository resolution order for general commands:

```text
1. Valid explicit CLI values, if supported
2. Valid .zoro/config.yaml values
3. Current Git repository remote
4. Error
```

Never treat these as valid repository values:

```text
OWNER
REPOSITORY
owner
repository
<owner>
<repository>
empty strings
```

Use a dedicated validation helper instead of checking placeholders ad hoc across commands.

## Shared Repository Resolver

Implement or refactor toward a shared API such as:

```python
class RepositoryIdentity(BaseModel):
    root: Path
    remote_name: str
    remote_url: str
    owner: str
    repo: str


def resolve_repository_identity(
    cwd: Path,
    config: ZoroConfig | None = None,
) -> RepositoryIdentity:
    ...
```

Behavior:

```text
find git root
    ↓
read valid config values if appropriate
    ↓
inspect remote when values are missing/invalid
    ↓
parse GitHub identity
    ↓
return normalized object
```

Do not tie repository detection directly to Typer or Rich.

The core resolver must be independently testable.

## Suggested Module Responsibilities

Preserve the current project structure if it already has equivalents.

Suggested boundaries:

```text
repository.py
    Git root detection
    remote discovery
    GitHub remote parsing
    repository identity

github.py
    credentials
    auth verification
    repository verification
    project discovery
    project field discovery

config.py
    config models
    placeholder validation
    config rendering/writing

cli.py
    prompts
    Rich output
    zoro init command orchestration
```

Do not move unrelated functionality.

## Example Successful UX

Target something similar to:

```text
$ zoro init

zoro.ai initialization

Repository
  ✓ Git repository detected
  ✓ Root: /home/user/my-project
  ✓ Remote: git@github.com:nenjotech/my-project.git
  ✓ Owner: nenjotech
  ✓ Repository: my-project

GitHub
  ✓ Authentication detected
  ✓ Logged in as nenjotech
  ✓ Repository access verified

GitHub Projects

❯ Agentic Development
  Development
  Roadmap

Selected:
  Agentic Development (#3)

Project
  ✓ Status field detected

Workflow
  ✓ Backlog
  ✓ Ready
  ✓ In progress
  ✓ In review
  ✓ Done

Created:
  .zoro/config.yaml
  .zoro/runtime/
  handoff/ready/
  handoff/implementing/
  handoff/review/
  handoff/done/
  handoff/failed/

zoro.ai initialized successfully.

Next:

  zoro doctor
  zoro board
```

## Example Missing GitHub Authentication

```text
$ zoro init

zoro.ai initialization

Repository
  ✓ Git repository detected
  ✓ Owner: nenjotech
  ✓ Repository: my-project

GitHub
  ✗ Authentication not found

Authenticate with:

  gh auth login

or set:

  ZORO_GITHUB_TOKEN
  GH_TOKEN

Then run:

  zoro init
```

No broken configuration should be created.

## Example Permission Failure

```text
$ zoro init

Repository
  ✓ nenjotech/private-project

GitHub
  ✓ Authentication detected
  ✓ Logged in as nenjotech
  ✗ Repository access failed

Unable to access:

  nenjotech/private-project

The repository may not exist, or the current GitHub credentials may not
have permission to access it.

Check token repository access and try again.
```

## Example Missing Origin

```text
$ zoro init

Git repository detected.

No Git remote is configured.

Add the repository remote:

  git remote add origin git@github.com:OWNER/REPOSITORY.git

Then run:

  zoro init
```

## Example Unsupported Remote

```text
$ zoro init

Unsupported Git remote:

  git@gitlab.com:company/project.git

zoro.ai currently supports GitHub repositories only.
```

## Tests

Add focused tests for the behavior.

### Repository Detection

Test:

```text
SSH GitHub remote
HTTPS GitHub remote
HTTPS remote without .git
ssh:// GitHub remote
repository names containing hyphens
organization repositories
nested current working directory
```

Examples:

```python
@pytest.mark.parametrize(
    ("remote", "owner", "repo"),
    [
        (
            "git@github.com:nenjotech/zoro-ai.git",
            "nenjotech",
            "zoro-ai",
        ),
        (
            "https://github.com/nenjotech/zoro-ai.git",
            "nenjotech",
            "zoro-ai",
        ),
        (
            "https://github.com/acme/platform",
            "acme",
            "platform",
        ),
    ],
)
def test_parse_github_remote(remote, owner, repo):
    ...
```

### Invalid Remotes

Test:

```text
GitLab remote rejected
Bitbucket remote rejected
malformed remote rejected
empty remote rejected
```

### Git Repository

Test:

```text
not inside Git repository
inside repository root
inside nested repository directory
missing origin
single non-origin remote
multiple remotes
```

Mock subprocess operations appropriately.

Do not depend on the developer's real Git configuration.

### Placeholder Detection

Test:

```yaml
owner: OWNER
repo: REPOSITORY
```

is considered incomplete.

Also test empty values.

### Authentication

Mock:

```text
ZORO_GITHUB_TOKEN
GH_TOKEN
gh auth token
```

and verify precedence.

Test no credentials.

Test invalid credentials.

### Repository Access

Mock:

```text
success
401
403
404
network failure
```

Ensure private repository `404` is not presented as definitive nonexistence.

### Project Discovery

Mock:

```text
zero projects
one project
multiple projects
project access denied
```

### Project Status

Test:

```text
Status field exists with all required values
Status field missing
Ready missing
In review named differently
```

### Initialization Safety

Test:

```text
successful init writes config
failed auth does not write final config
failed repo validation does not write final config
existing config is not silently overwritten
placeholder config can be repaired
all paths are based on repository root
```

## Validation

Run the project's actual configured validation commands.

At minimum, if available:

```bash
uv run pytest
uv run ruff check .
```

If the project has type checking configured, run it as well.

Do not report validation as successful unless it was executed.

## Files Affected

Inspect the existing codebase first.

Likely areas:

```text
zoro/cli.py
zoro/config.py
zoro/repository.py
zoro/github.py
zoro/models.py
zoro/errors.py
tests/
README.md
```

Do not create new modules if equivalent abstractions already exist.

Do not refactor unrelated commands.

## Implementation Notes

Prefer small reusable functions.

Avoid shell invocation through:

```python
subprocess.run(..., shell=True)
```

Use argument arrays:

```python
subprocess.run(
    ["git", "remote", "get-url", "origin"],
    ...
)
```

Set:

```python
check=False
capture_output=True
text=True
```

or use the project's existing subprocess abstraction.

Normalize command failures into typed Zoro errors.

Do not leak raw access tokens through command output or exceptions.

## Acceptance Criteria

- [ ] `zoro init` detects the current Git repository root.
- [ ] Initialization works when invoked from a nested directory.
- [ ] `zoro init` reads the current Git remote.
- [ ] GitHub SSH remotes are supported.
- [ ] GitHub HTTPS remotes are supported.
- [ ] Owner is extracted automatically.
- [ ] Repository name is extracted automatically.
- [ ] `.git` is removed from repository names.
- [ ] Unsupported non-GitHub remotes fail clearly.
- [ ] Missing Git remotes fail clearly.
- [ ] GitHub credentials are resolved using the shared credential resolver.
- [ ] `ZORO_GITHUB_TOKEN` is supported.
- [ ] `GH_TOKEN` is supported.
- [ ] `gh auth token` fallback is supported.
- [ ] GitHub credentials are never written to config.
- [ ] Authentication is verified during initialization.
- [ ] Repository access is verified during initialization.
- [ ] Private repository permission failures produce useful errors.
- [ ] Accessible GitHub Projects can be discovered.
- [ ] The user can select the GitHub Project during initialization.
- [ ] The project's Status field is inspected.
- [ ] Workflow status values are validated.
- [ ] `.zoro/config.yaml` receives real `owner`, `repo`, and `project_number`.
- [ ] Successful initialization never contains `OWNER/REPOSITORY`.
- [ ] Failed initialization does not leave a misleading completed config.
- [ ] Handoff directories are created under the repository root.
- [ ] Existing valid configuration is not silently overwritten.
- [ ] Existing placeholder configuration can be repaired.
- [ ] `zoro auth` can reuse repository auto-detection.
- [ ] Repository detection logic is independent from CLI rendering.
- [ ] Important behavior has unit tests.
- [ ] External GitHub requests are mocked in tests.
- [ ] Git subprocess behavior is tested without relying on the user's environment.
- [ ] Existing tests continue to pass.
- [ ] Ruff passes if configured.

## Definition of Done

This sequence must work from an existing cloned GitHub repository:

```bash
cd my-project

gh auth login

zoro init
```

Without manually editing `.zoro/config.yaml`, Zoro should detect:

```text
owner
repository
GitHub authentication
repository access
available GitHub Projects
project Status workflow
```

and produce a configuration similar to:

```yaml
github:
  owner: nenjotech
  repo: my-project
  project_number: 3

  status_field: Status

  statuses:
    backlog: Backlog
    ready: Ready
    implementing: In progress
    review: In review
    done: Done
```

After initialization:

```bash
zoro auth
zoro doctor
zoro board
```

must operate using the initialized repository without returning:

```text
Repository not found: OWNER/REPOSITORY
```

Keep the solution focused on repository discovery, GitHub validation, project discovery, safe configuration generation, and tests. Do not refactor unrelated Zoro functionality.