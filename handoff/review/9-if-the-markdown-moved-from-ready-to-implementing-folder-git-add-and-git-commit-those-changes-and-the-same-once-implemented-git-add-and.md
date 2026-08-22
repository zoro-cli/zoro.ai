---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3lDgo
issue: 9
title: if the markdown moved from Ready to Implementing folder, git add and git commit those changes, and the same once implemented, git add and git commit the changes every steps
created_at: 2026-08-22T13:46:16.6055202Z
dirty_at_planning: false
---

# if the markdown moved from Ready to Implementing folder, git add and git commit those changes, and the same once implemented, git add and git commit the changes every steps

## Summary

Add two deterministic Git commits to the shared implementation flow: one immediately after the handoff enters `implementing`, before Codex starts, and one after Codex and validation succeed that captures all implementation changes and the move to `review`. Implement safe staging helpers, explicit partial-failure behavior, comprehensive Git/orchestration tests, and workflow documentation.

## Objective

Make each successful implementation lifecycle auditable in Git by committing the handoff's transition into `implementing` before Codex runs, then committing the validated implementation and transition into `review` after completion.

## Assumptions

- The requested behavior applies to the implementation lifecycle, not planning: one commit records `ready -> implementing`, and another records validated source changes plus `implementing -> review`.
- The first lifecycle commit must be made on the configured Zoro implementation branch; therefore branch creation may need to occur before the first handoff move rather than following the older documented ordering.
- Implementation still starts only from a clean working tree. This invariant permits the final successful commit to stage all Codex-created source changes without capturing pre-existing developer work.
- The initial transition commit should stage only the known handoff source and destination paths so unrelated files cannot be included accidentally.
- The completion commit should be created only after Codex and all enabled validation commands succeed. Failed or unvalidated source changes must not be represented as a successful implementation commit.
- Moves to `handoff/failed` and commits of failed Codex output are not automatically included in the successful two-commit workflow. Existing failure behavior should be preserved unless inspection shows a clean, safe lifecycle-only failure commit is already expected.
- No new configuration flag is required; committing lifecycle transitions is the requested default implementation behavior.
- This issue does not include pushing branches, creating pull requests, or automatic merging. Those concerns are represented by the separate failed issue 6 handoff and should not be folded into this change.
- The issue has no body or explicit acceptance criteria, so the criteria in this plan are implementation checks derived from its title.

## Preparation

- Open `internal/cli/root.go` and `internal/cli/root_test.go`, which were listed but not supplied, and locate the shared manual/automatic implementation function and its dependency seams.
- Open `internal/repository/repository.go`, `internal/repository/repository_test.go`, and `internal/process/process.go` to identify existing Git helpers, result/error models, and fake command-runner support.
- Open `internal/handoff/handoff.go` and tests to confirm move semantics, destination collision behavior, and whether transitions expose both source and destination paths for path-scoped staging.
- Review current failure compensation around local handoff moves and GitHub status changes before changing operation order.
- Confirm how tests establish Git author name/email; production must surface normal Git identity errors rather than modifying global Git configuration.
- Preserve the currently reported clean repository state and do not introduce generated binaries into commits during implementation or validation.

## Implementation steps

1. Inspect the shared implementation pipeline in `internal/cli/root.go` and its tests to establish the current exact order of lock acquisition, clean-tree validation, handoff lookup, branch creation, local moves, GitHub status updates, Codex, validation, and success/failure handling.
2. Inspect `internal/repository/repository.go` and `internal/process/process.go` for existing command-runner behavior. Reuse existing abstractions and add only the missing context-aware helpers for path-scoped staging, staging all implementation changes, checking staged changes where needed, and committing with a supplied message.
3. Define deterministic commit messages for the two boundaries, including the issue number and lifecycle purpose. Keep message construction in business/repository orchestration rather than scattering string literals through Cobra handlers.
4. Adjust implementation-start ordering so all preflight checks and branch collision checks happen before mutation, the implementation branch is created, the handoff is moved from Ready to Implementing, and the GitHub item is synchronized to In progress according to the chosen compensating policy.
5. Immediately after the Ready-to-Implementing move and before Codex invocation, stage the handoff deletion/addition paths and create the first lifecycle commit. Verify Codex is never invoked if staging or committing this transition fails.
6. Document and implement compensation for failures before Codex starts. In particular, if GitHub synchronization or the first commit fails after a local move, restore a coherent handoff/status state without resetting unrelated work, and report any partial state that cannot be safely compensated.
7. Invoke Codex only after the first commit succeeds, ensuring Codex begins from a clean implementation branch whose history records the handoff entering the implementing state.
8. Preserve the current validation sequence. If Codex or any required validation fails, do not move the handoff to review and do not create the successful implementation commit; retain the existing failed-handoff and diagnostic behavior.
9. On successful Codex execution and validation, move the handoff from Implementing to Review, stage all source changes produced from the clean baseline together with that handoff transition, verify there is something staged, and create the second deterministic commit.
10. Synchronize the GitHub project item to In review using an ordering that accurately reports partial success. If status synchronization occurs after the local completion commit, return a partial-success error when GitHub fails rather than claiming the entire operation succeeded.
11. Ensure both manual implementation and `automation.auto_implement` call the same commit-aware implementation function, avoiding duplicate Git sequencing in command-specific paths.
12. Add repository-level Git tests in temporary repositories, configuring repository-local author identity, to cover path-scoped add, add-all, commits containing renames and new files, no-change behavior, command errors, and commit history.
13. Extend orchestration tests with recording fakes to verify the exact successful order: clean preflight, branch creation, Ready-to-Implementing move/status synchronization, first commit, Codex, validation, Implementing-to-Review move, second commit, and review status synchronization.
14. Add failure-ordering tests for branch creation, first add/commit, Codex, validation, final add/commit, and GitHub status updates. Assert that no later operation runs after a failure and that errors describe local and remote partial state.
15. Update README lifecycle and `zoro implement` documentation to state that entering implementation and completing validated implementation are separate Git commits, while Zoro still never marks the project item Done.
16. Format changed files and execute focused and full validation without overwriting tracked CLI binaries.

## Validation steps

1. Run gofmt on all changed Go source and test files.
2. Run focused repository tests covering path-scoped staging, add-all, rename commits, implementation commits, no-change behavior, and Git failures.
3. Run focused CLI orchestration tests for manual and automatic implementation success ordering.
4. Run failure-path tests for branch creation, GitHub In progress synchronization, first staging/commit, Codex, validation, review move, final staging/commit, and GitHub In review synchronization.
5. Inspect temporary-repository commit history and assert exactly two lifecycle commits are produced on success, in the expected order and with deterministic issue-identifying messages.
6. Inspect the first commit tree and verify it records only the Ready-to-Implementing handoff transition.
7. Inspect the second commit tree and verify it records Codex/validation changes and the Implementing-to-Review transition.
8. Verify Codex starts with a clean worktree after the first transition commit.
9. Verify Codex or validation failure creates no successful completion commit and never moves the handoff or project item to review.
10. Verify dirty-tree preflight still rejects implementation before branch creation, handoff movement, staging, or committing.
11. Run `go test ./...`.
12. Run `go test -race ./...`.
13. Run `go vet ./...`.
14. Run `go build -o <temporary-path>/zoro ./cmd/zoro` so tracked binaries are not modified.

## Risks

- The existing recommended lifecycle creates the branch after moving the handoff and updating GitHub. The first transition must be committed on the implementation branch, so operation ordering must change carefully.
- A Git commit can fail due to missing author identity, hooks, signing configuration, or filesystem errors. Zoro should preserve the Git error and provide coherent recovery guidance without changing global user configuration.
- If the first commit succeeds but a later remote status update fails, history cannot be safely erased. The command must report partial success and use non-destructive compensation where possible.
- If the final commit occurs before the GitHub move to In review, a GitHub failure leaves valid committed work with stale board state; if it occurs afterward, a commit failure leaves the board prematurely in review. The chosen ordering must be explicit and tested.
- Broad staging is safe only under the clean-tree invariant. Any future relaxation of that invariant could cause developer files to be committed unintentionally.
- Validation commands may modify files. Existing behavior must determine whether such generated changes belong in the completion commit; tests should document the chosen inclusion behavior.
- Codex or validation failure may leave source changes plus a failed-handoff transition in the working tree. This issue should not mislabel or commit those changes as successfully implemented.
- Hooks may mutate or reject commits, making exact post-commit cleanliness uncertain. Repository helpers should verify outcomes and return actionable errors.
- The separate issue 6 handoff proposes commit/push/PR behavior and may overlap with the final commit. This implementation should remain narrowly scoped and avoid introducing publication or merge behavior.
- Tracked binary outputs exist in the repository tree. Build validation must target a temporary or untracked path to avoid accidentally including generated executables.

## Relevant files

- `internal/cli/root.go`: Repository metadata and prior handoffs indicate implementation orchestration currently resides in this command-layer file. — Primary production change: introduce commit-aware lifecycle ordering and share it between manual and automatic implementation.
- `internal/cli/root_test.go`: The behavior crosses handoff moves, branches, GitHub status, Codex, validation, and Git commits. — Add orchestration ordering and failure-path coverage for both implementation entry points.
- `internal/repository/repository.go`: Git operations belong in the repository abstraction rather than being invoked directly from Cobra logic. — Add or expose staging and commit helpers needed by orchestration.
- `internal/repository/repository_test.go`: Temporary repositories can verify real commit contents without external services. — Add isolated Git tests for staging scope, rename commits, final implementation commits, and failures.
- `internal/process/process.go`: This is the shared subprocess execution layer used for Git commands. — Only change if command result/error behavior lacks what repository commit helpers require.
- `internal/handoff/handoff.go`: It owns Ready, Implementing, Review, and Failed filesystem transitions. — Likely no semantic change; inspect transition return values and move behavior to support precise staging and compensation.
- `internal/handoff/handoff_test.go`: The first and second commits depend on deterministic, non-overwriting handoff moves. — Potentially add tests only if transition behavior must expose paths or compensation semantics change.
- `README.md`: Committing repository changes is a visible and consequential CLI behavior. — Document automatic commit boundaries in the implementation workflow.

## Proposed changes

- `internal/cli/root.go`: Extend the shared implementation orchestration with two explicit commit boundaries: commit the Ready-to-Implementing transition before Codex, and commit validated source changes plus the Implementing-to-Review transition after validation. Reorder branch creation and state synchronization where necessary, and add contextual partial-failure handling. (Risk: Git, filesystem, and GitHub state are not transactional. Incorrect ordering can leave a committed handoff, project status, or branch inconsistent; compensation must avoid destructive reset, stash, or clean operations.)
- `internal/repository/repository.go`: Add or reuse safe Git operations for path-scoped staging, staging implementation changes, detecting staged content, and creating commits with deterministic messages through the existing process abstraction. (Risk: Using `git add -A` too early could include unrelated files. It is appropriate for the final implementation commit only because implementation enforces an initially clean tree; the first transition should be path-scoped.)
- `internal/repository/repository_test.go`: Add temporary-repository tests proving handoff renames, source edits, and untracked implementation files are committed at the intended boundaries, while no-change and Git failures are surfaced. (Risk: Git tests can depend on host identity or default branch configuration; configure identity and branch behavior locally inside each temporary repository.)
- `internal/cli/root_test.go`: Add recording-fake tests for commit call order across manual and automatic implementation, plus failures at each transition, commit, Codex, validation, and GitHub synchronization boundary. (Risk: If orchestration currently constructs concrete dependencies internally, a small injection seam may be needed; avoid a broad command-layer refactor.)
- `internal/process/process.go`: Potentially extend process result handling only if existing output and exit-code information is insufficient for actionable `git add` and `git commit` errors. (Risk: Changing the shared command abstraction can affect Git, Codex, and validation callers; keep any extension backward-compatible and narrowly scoped.)
- `README.md`: Update implementation lifecycle documentation to describe both automatic commits and clarify that only validated work is committed as completed and the item remains human-controlled at In review. (Risk: Documentation must not imply that Zoro pushes, opens a pull request, merges, or marks work Done as part of this issue.)

## Acceptance criteria

- [ ] Moving a selected handoff from `handoff/ready` to `handoff/implementing` is recorded in a Git commit before Codex begins modifying the repository. — Run the implementation orchestration with a temporary Git repository and fake Codex runner; inspect commit history and assert the first new commit contains only the Ready-to-Implementing handoff transition and precedes Codex invocation.
- [ ] After Codex succeeds and all configured validation commands pass, Zoro stages the implementation changes together with the Implementing-to-Review handoff transition and creates a second Git commit. — Use a fake Codex implementation that edits tracked and untracked files, then assert the second commit contains those changes and the handoff under `handoff/review` with no remaining staged or unstaged implementation changes.
- [ ] The lifecycle commits use deterministic, issue-identifying commit messages. — Assert orchestration tests receive the expected messages, such as a start-transition message and an implementation-complete message containing the issue number.
- [ ] Codex is not invoked if the Ready-to-Implementing transition cannot be committed. — Inject staging and commit failures and assert Codex and validation are not called, the error identifies the failed Git operation, and compensating lifecycle behavior follows the documented policy.
- [ ] The successful implementation commit is not created when Codex or validation fails. — Inject Codex and validation failures and assert no completion commit or move to `handoff/review` occurs.
- [ ] Manual `zoro implement` and automatic implementation use the same commit-aware lifecycle. — Exercise both entry points with recording fakes and assert identical transition, staging, commit, Codex, validation, and completion ordering.
- [ ] Git staging and commits do not include pre-existing developer work. — Retain the clean-tree precondition, test that implementation refuses a dirty repository, and use path-scoped staging for the initial handoff transition.
- [ ] Git command failures are contextual and do not falsely report implementation success or review readiness. — Test failures from add and commit at both commit boundaries and assert later lifecycle operations are skipped and the returned error names the failed phase.
- [ ] The documented implementation workflow explains the two commit boundaries. — Inspect README command/lifecycle documentation for the Ready-to-Implementing commit and the post-validation implementation/review commit.
- [ ] The repository remains formatted, tested, race-free, vetted, and buildable. — Run gofmt on changed Go files, `go test ./...`, `go test -race ./...`, `go vet ./...`, and build `./cmd/zoro` to an untracked or temporary output path.
