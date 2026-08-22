---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3lAno
issue: 6
title: once the handoff implemented, commit and create a PR, I will merge it manually, add a config whether they want to merged the PR it automatically or not,
created_at: 2026-08-22T13:42:09.9359078Z
dirty_at_planning: true
---

# once the handoff implemented, commit and create a PR, I will merge it manually, add a config whether they want to merged the PR it automatically or not,

## Summary

Add a post-validation publication stage shared by manual and automatic implementation. It will commit the clean-baseline changes, push the Zoro branch, idempotently create a pull request, and optionally merge it through a new `automation.auto_merge` setting that defaults to false. The implementation must preserve repository safety, avoid force operations and duplicate PRs, expose partial-success failures, and keep the project item in review rather than Done.

## Objective

Extend the successful handoff implementation lifecycle so Zoro commits the validated changes, pushes the implementation branch, creates an idempotent GitHub pull request, and optionally merges that pull request when explicitly enabled in configuration.

## Assumptions

- “Once the handoff is implemented” means Codex exited successfully and all configured validation commands passed; failed implementations must not be committed or published.
- Zoro should always commit, push, and create a pull request after successful implementation; only the final merge is optional.
- The new option should be named `automation.auto_merge` and default to false so existing users retain manual merge control.
- Automatic merge means requesting an immediate normal GitHub merge after PR creation, not enabling GitHub's deferred auto-merge queue. If branch protections prevent the merge, Zoro should return a clear error and leave the PR available for manual action.
- The pull request base should be the branch that was current immediately before Zoro created the implementation branch; this value must be captured explicitly rather than guessed later.
- Branch-based implementation must be enabled for PR publication. Configuration validation or implementation preflight should reject a workflow that requires a PR while branch creation is disabled, unless the implementation introduces a separately defined head branch strategy.
- The existing shared manual/automatic implementation lifecycle in `internal/cli/root.go` should remain the single integration point so publication behavior cannot diverge between command modes.
- GitHub PR creation and merge should use the existing authenticated GitHub client rather than constructing token-bearing shell commands. Git push can continue through the repository/process abstraction without placing credentials on command lines.
- A successful auto-merge does not authorize Zoro to move the project card to Done; current human-controlled project status semantics remain unchanged.
- The current dirty files belong to issue 5 work and must be preserved; implementation and clean-tree integration tests should use temporary repositories until that work is committed or otherwise handled by the developer.

## Preparation

- Preserve the existing dirty-tree changes for issue 5: deleted `handoff/ready/...`, modified `internal/handoff/handoff.go` and `internal/handoff/handoff_test.go`, and untracked `handoff/review/...`. Do not reset, clean, stash, or fold them into this feature unintentionally.
- Open `internal/cli/root.go` and `internal/cli/root_test.go`, which were identified in the repository tree but not included in supplied file contents, to locate the shared implementation pipeline and current dependency seams.
- Open `internal/repository/repository.go` and `internal/process/process.go` to determine which Git helpers already exist and whether command results expose stdout, stderr, and exit status sufficiently for commit/push operations.
- Open `internal/github/client.go` and `internal/github/client_test.go` to reuse REST request construction, repository addressing, response limits, and error handling for pull requests.
- Confirm whether the current implementation moves the handoff to review before or after the final GitHub project status update; choose a single documented publication order and encode partial-success behavior in tests.
- Confirm that the branch name and original base branch are available throughout implementation. If only the current branch is queried after checkout, introduce an explicit captured base value.
- Decide and document the deterministic commit message and PR body format without including secrets, raw model payloads, or excessive Codex output.

## Implementation steps

1. Inspect the current implementation function in `internal/cli/root.go` and its tests to document exact ordering for lock acquisition, clean-tree verification, base/current branch lookup, handoff moves, GitHub status changes, Codex, validation, and review transition.
2. Inspect `internal/repository/repository.go`, `internal/process/process.go`, and their tests for existing Git execution seams. Add only the missing helpers for staging, detecting whether there are changes to commit, committing, and pushing the current named branch.
3. Extend `AutomationConfig` with `AutoMerge bool` using YAML key `auto_merge`; set it to false in defaults and generated config. Preserve backward compatibility so configurations that omit it decode as false.
4. Define publication inputs/results in the implementation layer, including issue metadata, base branch, head branch, commit SHA if useful, pull request number/URL, and whether it was merged. Keep this logic independent of Cobra output.
5. Capture the original current branch before creating the `zoro/<issue>-<slug>` branch and retain it as the PR base. Validate branch creation/publication prerequisites before invoking Codex where possible.
6. After Codex and validation succeed, complete the local handoff transition intended for the PR, stage the clean-baseline implementation changes with `git add -A`, verify that staged changes exist, and create a deterministic commit such as `zoro: implement #<issue> <title>`. Return a clear error if Codex reported success but produced no committable change.
7. Push the implementation branch to `origin` without force, using an explicit branch/refspec and upstream tracking as appropriate. Never overwrite an existing remote branch or force-push.
8. Extend the GitHub client with typed operations to find an existing open pull request by head/base, create a pull request, and merge a pull request. Reuse authenticated context-aware request helpers, bounded response decoding, and sanitized error handling.
9. Build the PR title from the issue title and a deterministic body referencing the issue and handoff. Search for an existing PR for the same repository/head/base before creation so a retry does not create a duplicate.
10. When `automation.auto_merge` is false, stop after PR creation/reuse and print/report the PR URL for manual merge.
11. When `automation.auto_merge` is true, invoke the merge endpoint only after obtaining the PR number. Treat an already merged PR as idempotent success, but surface closed-unmerged, conflict, required-check, or branch-protection responses clearly.
12. Integrate publication into the shared successful implementation path used by manual `zoro implement` and automatic cycles. Keep project status at In review and never transition it to Done, including after successful automatic merge.
13. Define failure behavior at every boundary: Codex/validation failures follow the existing failed-handoff path; commit failure leaves uncommitted local work with explicit recovery instructions; push failure leaves a local commit; PR failure leaves a pushed branch; merge failure leaves an open PR. Do not reset, delete, force-push, or conceal these partial states.
14. Add focused repository tests in temporary Git repositories with a bare remote to verify commit creation, changed-file inclusion, no-change handling, explicit push behavior, and push failures without contacting external services.
15. Add GitHub `httptest` coverage for PR lookup, creation request/response parsing, merge request/response parsing, authentication/error sanitization, already-existing/already-merged behavior, and branch-protection failures.
16. Extend implementation orchestration tests with a recording fake to verify call order, both manual and automatic entry points, auto-merge true/false, duplicate PR reuse, and failure short-circuiting.
17. Update README workflow/configuration examples to explain that successful implementation is committed and proposed through a PR, `automation.auto_merge` defaults to false, enabling it may still be blocked by repository rules, and Zoro never marks the project item Done.

## Validation steps

1. Run gofmt on every changed Go source and test file.
2. Run focused configuration tests for omitted, false, and true `automation.auto_merge` values.
3. Run focused repository tests using temporary repositories and a bare origin to verify staging, commit, no-change handling, explicit branch push, and non-force failure behavior.
4. Run focused GitHub client tests against `httptest.Server` for PR lookup, creation, merge, already-existing/already-merged states, and protected-branch failures.
5. Run implementation orchestration tests verifying successful ordering: Codex, validation, review transition, stage, commit, push, PR ensure, optional merge, and project review synchronization according to the chosen documented order.
6. Run failure-path tests at Codex, validation, commit, push, PR creation, merge, and final board synchronization; assert no later stage runs after a failure and partial successes are accurately reported.
7. Run both manual and automatic implementation tests to verify they share identical publication behavior.
8. Run `go test ./...`.
9. Run `go test -race ./...`.
10. Run `go vet ./...`.
11. Run `go build -o zoro ./cmd/zoro` using an output location that does not overwrite tracked or unrelated local artifacts.
12. In an isolated test repository, verify `auto_merge: false` leaves a newly created PR open and reports its URL.
13. In an isolated repository where merge is permitted, verify `auto_merge: true` merges the created PR but leaves the GitHub Project item at In review rather than Done.
14. Verify a repository with required checks or merge restrictions returns a clear merge error while preserving the open PR for manual handling.

## Risks

- The repository is currently dirty from issue 5, so the real implementation command must refuse to run until those changes are handled; tests should not weaken this safety check.
- The handoff is moved before Codex starts, making the tree intentionally dirty during implementation. Commit staging must distinguish expected Zoro/Codex changes from unrelated changes by relying on the initial clean-tree invariant.
- PR creation requires a remote branch and a known base branch. If implementation branches are disabled or the starting branch is detached, publication cannot proceed safely without explicit validation.
- Commit author identity may be absent on a user's machine. Git errors should be preserved with actionable guidance rather than silently configuring global identity.
- Push can fail because of authentication, remote collisions, or protections. Zoro must not force-push and must accurately report that a local commit remains available.
- A PR creation failure after push and a merge failure after PR creation are irreversible partial successes; destructive rollback would be unsafe, so retry/idempotency behavior is required.
- Searching for an existing PR by head/base must account for owner-qualified head names and open versus already merged/closed PRs to avoid duplicate creation.
- Immediate merge can be rejected by required checks, reviews, merge conflicts, or branch protection. Enabling the option cannot guarantee a merge and must not bypass repository rules.
- If project status moves to In review before publication finishes, a PR failure leaves board and publication state partially synchronized; if status moves afterward, a status failure follows successful publication. The chosen order and error message must make this explicit.
- Automatically merged PRs may delete or alter branches through repository policies external to Zoro; Zoro itself should not delete local or remote branches as part of this issue.
- A successful merge must not automatically mark the project card Done, preserving the existing human approval model.
- The issue asks to commit and create a PR, which changes the prior documented boundary of stopping at review; documentation and tests must make the new side effects clear.

## Relevant files

- `internal/config/config.go`: Owns the typed automation configuration and generated defaults. — Add the new boolean option, safe default, keyed default literals, and YAML persistence behavior.
- `internal/config/config_test.go`: Existing configuration tests are the natural regression suite for the new option. — Test default false, explicit true, save/load, and backward-compatible omission.
- `internal/repository/repository.go`: Git state, branches, and command execution belong in the repository abstraction rather than Cobra or GitHub code. — Add or reuse safe commit and push primitives.
- `internal/repository/repository_test.go`: Commit and push behavior can be tested without network access. — Cover Git publication helpers in isolated repositories and remotes.
- `internal/github/client.go`: The dedicated client already owns authenticated GitHub API interactions. — Add pull-request discovery, creation, and merge REST operations.
- `internal/github/client_test.go`: External GitHub interactions must remain absent from unit tests. — Mock all new GitHub PR endpoints and error cases.
- `internal/cli/root.go`: Repository context indicates manual and automatic orchestration currently converge in this command-layer implementation pipeline. — Integrate commit, push, PR, and optional merge after successful shared implementation.
- `internal/cli/root_test.go`: The behavior crosses configuration, Codex, validation, handoff state, Git, GitHub, and automatic/manual entry points. — Add implementation publication decision and ordering tests.
- `internal/process/process.go`: Shared subprocess execution should remain the single mechanism for Git commands. — Potentially extend result handling only if current process output is insufficient for reliable Git command errors.
- `internal/handoff/handoff.go`: Handoff lifecycle files are Git-trackable and their final review transition should be represented consistently in the PR. — No core change expected; ensure the review-state handoff move is included in the successful commit at the chosen ordering point.
- `README.md`: Commit/PR creation and automatic merging are visible workflow and safety behaviors. — Document the publication workflow and optional merge setting.
- `.zoro/config.yaml`: This file demonstrates the current automation schema but may also be local configuration. — Optionally include the new key with a false default if maintained as a checked-in configuration example.

## Proposed changes

- `internal/config/config.go`: Add `automation.auto_merge` to the typed configuration, default it to false, emit it in generated YAML, and cover omitted/true/false loading behavior. (Risk: Positional composite literals currently used by `Default` are fragile when fields are added; convert affected literals to keyed fields to avoid accidental value shifts.)
- `internal/config/config_test.go`: Expand configuration tests for default-safe behavior and YAML round trips of the automatic merge option. (Risk: Boolean omission and explicit false decode identically, so tests should focus on effective behavior rather than attempting to distinguish them.)
- `internal/repository/repository.go`: Add Git helpers for staging all clean-baseline changes, checking staged changes, committing with a deterministic message, and pushing a named branch to origin without force. (Risk: A broad `git add -A` is safe only because implementation starts from a verified clean tree; preserving that invariant is essential to avoid committing unrelated developer files.)
- `internal/repository/repository_test.go`: Add temporary-repository and bare-remote tests for commit and push behavior, no-change errors, branch names, and failures. (Risk: Git tests require local author identity; configure repository-local test identity and skip only when Git is genuinely unavailable.)
- `internal/github/client.go`: Add typed pull-request lookup, creation, and merge methods using GitHub REST endpoints and the existing authenticated request helper. (Risk: Incorrect head qualification, base selection, or handling of 422 responses can create duplicates or misclassify branch-protection failures.)
- `internal/github/client_test.go`: Add `httptest` coverage for PR lookup filters, create payloads, merge payloads/responses, existing PR reuse, already-merged behavior, and sanitized errors. (Risk: GitHub uses different success statuses for create and merge operations; fixtures must model these accurately.)
- `internal/cli/root.go`: Extend the shared implementation success path to capture the base branch, commit validated work, push, ensure a PR, optionally merge it, and report publication details while retaining In review status. (Risk: Incorrect ordering can leave local handoff, project status, commit, remote branch, and PR state inconsistent. Each irreversible step needs explicit partial-success errors and no destructive rollback.)
- `internal/cli/root_test.go`: Add orchestration tests for call order, manual and automatic implementation, merge enabled/disabled, existing PR reuse, and failure at commit, push, PR, and merge stages. (Risk: Concrete dependency construction may make tests difficult; add the smallest injection seam rather than broadly refactoring Cobra command code.)
- `README.md`: Document successful commit/PR publication, the `automation.auto_merge` option and default, branch-protection limitations, and the unchanged In review/Done policy. (Risk: Documentation must not imply that enabling automatic merge bypasses required checks or marks project work Done.)
- `.zoro/config.yaml`: Add `auto_merge: false` to the repository's example/effective configuration only if this tracked configuration is intentionally maintained as an example rather than local-only state. (Risk: This file currently enables automatic implementation and may be environment-specific; avoid changing unrelated values or treating it as the sole documentation source.)

## Acceptance criteria

- [ ] After Codex completes and all enabled validation commands pass, Zoro creates a Git commit containing the implementation and handoff lifecycle changes. — Use a temporary Git repository and fake command runner; verify staging and commit occur only after successful validation and that the commit message identifies the issue.
- [ ] After the successful commit, Zoro pushes the implementation branch and creates a pull request against the branch from which the Zoro branch was created. — Assert command/API call order is validate, move handoff to review, stage, commit, push, create PR; verify the PR request contains the expected base branch, head branch, issue title, and issue/handoff reference.
- [ ] The pull request is created for both manual implementation and automatic implementation because both use the same implementation pipeline. — Exercise manual and automatic orchestration with fakes and assert each reaches the shared commit/push/PR workflow exactly once.
- [ ] A typed configuration option controls whether a newly created pull request is merged automatically, and the default remains false. — Load default and explicit YAML configurations; verify omitted/false values leave the PR open and true invokes the merge operation.
- [ ] When automatic PR merging is disabled, Zoro leaves the created pull request open for manual review and merge. — Run the successful implementation path with the option false and assert PR creation occurs but no merge API call occurs.
- [ ] When automatic PR merging is enabled, Zoro requests a merge only after the pull request has been created successfully. — Run with the option true and assert one merge request follows PR creation using the returned pull request number.
- [ ] Codex or validation failure does not create a commit, push a branch, create a pull request, or attempt a merge. — Inject failures at Codex and validation boundaries and assert all Git publication operations remain uncalled.
- [ ] Commit, push, pull-request creation, and merge failures are surfaced as contextual partial-success errors and never falsely report the item as fully published or merged. — Inject a failure at each publication stage and assert the returned error identifies the completed and failed stage while later operations are not invoked.
- [ ] Repeated handling does not silently create duplicate commits or pull requests when a matching pull request already exists. — Return an existing PR for the implementation head/base pair and verify Zoro reuses it rather than creating another; if auto-merge is enabled, it may continue with the existing open PR.
- [ ] Successful publication preserves the existing lifecycle rule that Zoro does not automatically mark the project item Done. — Assert the project status is moved at most to the configured review status regardless of whether the PR is left open or merged.
- [ ] Documentation and generated configuration explain the commit, PR, and optional automatic-merge behavior. — Inspect README and config output/fixtures for the new option and its safe default.
- [ ] The repository remains formatted, tested, race-free, vetted, and buildable. — Run gofmt on changed Go files, go test ./..., go test -race ./..., go vet ./..., and go build -o zoro ./cmd/zoro.
