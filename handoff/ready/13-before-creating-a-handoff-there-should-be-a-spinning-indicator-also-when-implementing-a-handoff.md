---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3lF6I
issue: 13
title: before creating a handoff there should be a spinning indicator, also when implementing a handoff,
created_at: 2026-08-22T13:55:00.5733501Z
dirty_at_planning: true
---

# before creating a handoff there should be a spinning indicator, also when implementing a handoff,

## Summary

Reuse the existing terminal-aware spinner to add `Creating handoff...` and `Implementing handoff...` activity indicators at shared orchestration boundaries. Keep redirected output plain, prevent overlap with the polling-wait spinner and subprocess output, and cover all cleanup and ordering paths with deterministic tests while preserving the current dirty-tree work.

## Objective

Add terminal-aware spinning progress indicators while Zoro creates a handoff and while it implements a handoff, with shared behavior across manual and automatic flows and clean lifecycle handling on success, failure, and cancellation.

## Assumptions

- The issue requests activity indicators for the potentially long planning/handoff-creation operation and the implementation operation, in addition to the already implemented continuous-run waiting spinner.
- The indicators should be terminal-aware and transient, matching the existing spinner behavior: animate only on an interactive terminal and keep redirected output plain.
- “Creating handoff” begins after an item has been selected and includes repository inspection, planner invocation, rendering, and saving; preflight errors that occur before any work starts do not need animation.
- “Implementing handoff” should wrap the shared manual/automatic implementation pipeline rather than only the Codex subprocess, while stopping around any subprocess output if the current Codex or validation runner streams directly to the same terminal.
- The existing dirty changes in `README.md`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/repository/repository.go`, `internal/repository/repository_test.go`, and the untracked review handoff are unrelated in-progress work and must not be reset, overwritten, or accidentally included in this feature.

## Preparation

- Record `git status` and inspect diffs for every currently modified/untracked file before implementation; do not reset, stash, clean, or overwrite them.
- Open the full current `internal/cli/root.go` and `internal/cli/root_test.go`, which contain in-progress changes not represented in the supplied file contents, and identify the exact shared planning and implementation boundaries.
- Open `internal/cli/spinner.go` and `internal/cli/spinner_test.go` to reuse the already implemented polling-wait spinner rather than introducing a second animation implementation.
- Confirm which Cobra stream the existing spinner uses, how interactivity is detected, and whether Codex/validation subprocess output is captured or streamed to that same stream.
- Use temporary repositories for implementation-path tests because the working repository is dirty and normal implementation correctly refuses a dirty tree.
- Direct build output to a temporary location so tracked binaries such as `cmd/zoro/zoro` or `cmd/zoro/zoro.exe` are not modified.

## Implementation steps

1. Inspect the current diffs and the implementations in `internal/cli/root.go`, `internal/cli/spinner.go`, and their tests before editing, because the repository is dirty and the supplied README indicates the waiting spinner has already been added.
2. Map all planning entry points (`zoro plan`, `run --once`, and continuous automatic cycles) to identify the shared function that performs repository inspection, planner invocation, and handoff saving. Place the new creation indicator at that shared boundary so behavior cannot diverge.
3. Map manual and automatic implementation entry points to the shared implementation function, including lock ownership and preflight checks. Add the implementation indicator only after clean-tree/handoff/branch preflight succeeds and before state transitions or Codex work begins.
4. Reuse and, if necessary, minimally extend the existing spinner abstraction so callers can provide operation-specific labels such as `Creating handoff...` and `Implementing handoff...`. Preserve context-aware, synchronized, idempotent stop/clear behavior and terminal detection.
5. Introduce the smallest test seam needed to observe spinner lifecycle—such as an injected spinner factory or start/stop interface—without moving business logic into Cobra callbacks or replacing the existing implementation dependencies.
6. Wrap handoff creation with structured cleanup (`defer` or an equivalent helper) so success, planner errors, save errors, and context cancellation all stop the spinner before returning. Ensure the final `Handoff created:` message is emitted only after cleanup.
7. Wrap the shared implementation lifecycle similarly. Verify whether Codex and validation output is buffered or streamed; if streamed to the spinner's terminal, stop the spinner before streaming begins or coordinate output so frames cannot interleave, while retaining visible progress for otherwise silent work.
8. Coordinate automatic continuous mode with the existing waiting spinner: waiting animation must be stopped before `RunOnce`; the creation or implementation indicator may then run; only after the cycle returns should the waiting indicator restart.
9. Add focused spinner tests for configurable labels, non-interactive suppression, cancellation, idempotent stop, line clearing, and absence of writes after stop. Prefer injected ticks or bounded synchronization over timing-sensitive sleeps.
10. Extend CLI orchestration tests to cover manual planning, automatic planning, manual implementation, automatic implementation, success, representative failures, cancellation, and event ordering relative to normal output.
11. Update README command documentation only if the current wording mentions solely the between-cycle waiting spinner; describe the new interactive planning and implementation progress indicators without implying detailed progress percentages.
12. Review the final diff against the initial dirty state and ensure unrelated repository/commit-lifecycle changes remain intact.

## Validation steps

1. Run gofmt on changed Go files, expected to include `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/cli/spinner.go`, and `internal/cli/spinner_test.go`.
2. Run focused tests for the CLI and spinner packages.
3. Run `go test ./...`.
4. Run `go test -race ./...` and verify no spinner goroutine or output races.
5. Run `go vet ./...`.
6. Run `go build -o <temporary-directory>/zoro ./cmd/zoro` without overwriting tracked binaries.
7. In an interactive terminal, run `zoro plan` against a controlled item and verify `Creating handoff...` animates during work, clears before the result, and stops on Ctrl+C or failure.
8. In an isolated clean repository, run manual implementation and verify `Implementing handoff...` appears without corrupting Codex or validation output and clears before completion/error messages.
9. Exercise automatic planning and automatic implementation with a short polling interval; verify the waiting spinner stops before the cycle, the appropriate operation indicator runs during work, and waiting resumes afterward.
10. Redirect command output during bounded planning/implementation tests and verify no repeated frames, carriage-return animation, or ANSI clearing sequences are present.
11. Compare the final working-tree diff with the recorded initial diff to ensure unrelated modified and untracked files were preserved.

## Risks

- The repository is dirty, and several primary target files already contain uncommitted changes. Implementing without first reviewing the diff could overwrite unrelated issue work.
- Starting a spinner too early could animate while interactive selectors, repository-cleanliness errors, or lock errors are displayed; start only after preflight and selection are complete.
- If Codex or validation streams output, a continuously running implementation spinner can corrupt that output. The design must stop or suspend animation before streamed output and may resume only when safe.
- Planning and implementation are used by both direct commands and automatic cycles. Wrapping only Cobra command handlers would miss automatic execution or create duplicate spinners.
- Continuous mode already has a waiting spinner. Poor lifecycle coordination could run two spinner goroutines simultaneously or leave the waiting spinner active during cycle output.
- Every error and cancellation path must stop and join the spinner goroutine; otherwise `go test -race` may detect writes after command completion.
- Terminal detection and line clearing must remain cross-platform and must not emit control characters into CI logs or redirected output.
- Real-time animation tests can be flaky unless rendering ticks and lifecycle events are injectable or explicitly synchronized.

## Relevant files

- `internal/cli/root.go`: The repository context and prior handoffs identify this file as containing planning, automatic cycles, manual implementation, and output sequencing. — Primary integration point for starting and stopping creation and implementation indicators in shared manual/automatic orchestration.
- `internal/cli/spinner.go`: An existing terminal-aware spinner already serves continuous polling and should be reused. — Potentially add label/configuration support or a small reusable operation-spinner API; retain current waiting behavior.
- `internal/cli/root_test.go`: The requested behavior crosses command orchestration and shared automatic/manual paths. — Add lifecycle and ordering tests across plan/run/implement entry points without external services.
- `internal/cli/spinner_test.go`: Spinner correctness requires deterministic tests independent of GitHub, OpenAI, or Codex. — Add focused regression coverage for operation labels, cancellation, terminal detection, cleanup, and concurrency.
- `README.md`: The README currently documents only the continuous waiting spinner according to supplied repository metadata. — Optional concise update describing operation indicators and plain redirected output.
- `internal/codex/codex.go`: Implementation progress animation must not interleave with Codex terminal output. — No production change expected; inspect to determine whether output is captured or streamed and therefore whether spinner shutdown must occur before Codex output begins.
- `internal/validation/validation.go`: Validation commands may write output during the implementation lifecycle. — No production change expected; inspect validation output behavior for spinner coordination.

## Proposed changes

- `internal/cli/root.go`: Integrate operation-specific spinner lifecycle into the shared handoff creation and implementation paths. Ensure waiting, planning, and implementation indicators never overlap and always stop before result/error output. (Risk: This file has existing uncommitted changes and coordinates locking, GitHub transitions, Codex, validation, and output. Incorrect placement could hide preflight errors, overlap terminal output, acquire locks differently, or leave a spinner active on an early return.)
- `internal/cli/spinner.go`: Generalize the existing spinner only as needed to accept labels and support reuse for finite operations while retaining terminal gating, synchronized writes, context cancellation, and idempotent cleanup. (Risk: Changing shared spinner semantics could regress the existing continuous-run waiting indicator or introduce goroutine/output races.)
- `internal/cli/root_test.go`: Add orchestration tests using fake spinner lifecycle events for manual/automatic planning and implementation, including success, failures, cancellation, and output ordering. (Risk: The file is already modified by other work. Tests should extend current seams rather than replacing or reverting concurrent lifecycle changes.)
- `internal/cli/spinner_test.go`: Expand focused spinner tests for multiple labels, interactive and non-interactive behavior, cancellation, clearing, repeated stop calls, and no post-stop writes. (Risk: Wall-clock animation tests can be flaky; use controllable ticks or explicit synchronization wherever the current implementation permits.)
- `README.md`: Document interactive progress feedback for planning and implementation alongside the existing continuous waiting indicator, if user-visible spinner behavior is documented. (Risk: README is already modified. Make a minimal additive change and do not rewrite unrelated workflow or commit behavior.)

## Acceptance criteria

- [ ] Manual and automatic planning display an animated “Creating handoff…” indicator while repository context collection, planner execution, and handoff persistence are in progress when the spinner output stream is an interactive terminal. — Exercise both `zoro plan` and one-cycle automatic planning with injected/fake dependencies and assert the spinner starts before long-running planning work and stops after the handoff is saved.
- [ ] Manual and automatic implementation display an animated “Implementing handoff…” indicator while the shared implementation lifecycle runs. — Exercise manual implementation and automatic implementation with fakes and assert both use the same spinner-wrapped implementation path.
- [ ] Progress indicators stop and clear before success messages, errors, validation output, or other normal command output is written. — Capture ordered spinner and command-output events in tests; verify stop occurs before completion/error reporting and no spinner frames are written afterward.
- [ ] Cancellation and every failure path stop the active spinner without leaking a goroutine or leaving terminal artifacts. — Cancel contexts and inject failures during planning, GitHub transitions, Codex, and validation; assert spinner shutdown completes and run the race detector.
- [ ] Non-interactive or redirected output contains no animation frames, carriage-return updates, or ANSI clearing sequences. — Use a non-terminal spinner configuration with captured output and assert planning/implementation results remain plain and stable.
- [ ] The existing continuous `zoro run` waiting spinner remains limited to polling waits and does not overlap the new operation indicators. — Test a continuous-cycle sequence and assert the waiting spinner stops before a cycle begins, the operation spinner runs only during work, and the waiting spinner resumes only after the cycle finishes.
- [ ] Existing repository changes are preserved and the project remains formatted, tested, race-free, vetted, and buildable. — Review the final diff, then run gofmt on changed Go files, `go test ./...`, `go test -race ./...`, `go vet ./...`, and build `./cmd/zoro` to a temporary output path.
