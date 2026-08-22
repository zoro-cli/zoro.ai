---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3k5tY
issue: 3
title: if the config auto implement is true, trigger the implement the handoff to the source code, and update the card to In progress, if the item has already a handoff and it's not yet implemented, implement the handoff and move the item to the in progress
created_at: 2026-08-22T13:06:20.751745Z
dirty_at_planning: true
---

# if the config auto implement is true, trigger the implement the handoff to the source code, and update the card to In progress, if the item has already a handoff and it's not yet implemented, implement the handoff and move the item to the in progress

## Summary

Update the automatic cycle’s decision logic so an existing handoff is no longer always a skip. A matching handoff in `ready` should enter the same implementation workflow as a newly generated handoff whenever `auto_implement` is enabled; other lifecycle states remain idempotently skipped. Keep lock acquisition at operation boundaries and reuse the normal implementation path so the card moves to In progress before Codex, validation remains enforced, and successful work moves only to review.

## Objective

Make automatic cycles invoke the normal implementation workflow whenever `automation.auto_implement` is enabled, both immediately after creating a handoff and when the selected Ready item already has an unimplemented ready handoff, while preserving locking, repository safety, idempotency, and board-state transitions.

## Assumptions

- “Already has a handoff and it is not yet implemented” means the handoff is in the local `ready` state. Handoffs in `implementing`, `review`, or `done` must not be restarted automatically, and `failed` remains explicit-retry-only.
- The existing implementation pipeline remains responsible for clean-tree checks, branch creation, handoff transitions, GitHub status updates, Codex invocation, validation, and transition to review.
- The top Ready item remains selected according to GitHub Project ordering; this issue does not change queue ordering or process multiple cards per cycle.
- `behavior.move_to_in_progress_on_implement` and other existing behavior configuration continue to govern transitions rather than introducing a second status-update path.
- The currently untracked handoff file is unrelated work and must not be modified, deleted, or included in this change.

## Preparation

- Preserve the dirty working tree exactly as found, including `handoff/ready/2-if-theres-a-ready-item-and-the-handoff-is-not-yet-commented-to-the-item-card.md`.
- Review all callers of `handoff.Find` before changing its API, since planning, manual implementation, status reporting, or duplicate detection may depend on its current path/metadata return values.
- Identify whether automatic cycles and manual implementation currently acquire the same lock at different layers; document the ownership boundary before wiring the paths together.
- Confirm how the existing implementation code receives the GitHub project item needed for status mutation, so resuming an existing handoff does not rely only on issue-number matching.
- Confirm the clean-tree behavior for same-cycle generated handoffs. Preserve the documented safety guarantee and cover the intended automatic path without broadly ignoring unrelated dirty files.

## Implementation steps

1. Inspect `internal/cli/root.go` to map the current `run`/`run --once`, planning, locking, and implementation call paths, especially the branch that currently skips an item whenever `handoff.Find` reports an existing handoff.
2. Separate cycle decision-making from command setup: retain lock acquisition at the public operation boundary and expose an internal implementation function that assumes the caller already owns the lock. Use that function from automatic cycles and the lock-owning wrapper from `zoro implement`.
3. Classify an existing handoff by state rather than treating every match as an unconditional skip. Prefer a typed handoff lookup result or a small state helper over fragile string comparisons at call sites.
4. Restructure one-cycle orchestration so it first selects the top Ready item and looks up its handoff, then applies a deterministic decision table: implement an existing ready handoff when auto-implementation is enabled; skip known non-ready states; plan only when no handoff exists and auto-planning is enabled; and implement a newly saved handoff when auto-implementation is enabled.
5. Ensure existing ready handoffs can be consumed even when `automation.auto_plan` is false; automatic planning should gate handoff creation, not prevent implementation of an already approved handoff.
6. Pass the selected project item and handoff path into the same implementation lifecycle used by the manual command so the Ready-to-In progress GitHub update occurs before Codex and success still transitions to review rather than Done.
7. Preserve transaction-like compensation already present in the implementation path: restore a moved handoff if the initial GitHub status update fails, and surface partial synchronization failures instead of claiming success.
8. Add focused table-driven tests for newly planned handoffs, existing handoffs in every state, auto-plan/auto-implement combinations, call ordering, duplicate prevention, lock ownership, and propagated implementation failures.
9. Update the README only if current wording does not clearly state that automatic cycles also resume existing ready handoffs; avoid changing configuration defaults.

## Validation steps

1. Run `gofmt` on all changed Go files.
2. Run the focused handoff and CLI orchestration tests, including the configuration/state decision table.
3. Run `go test ./...`.
4. Run `go test -race ./...` to verify lock use and automatic/manual orchestration remain race-free.
5. Run `go vet ./...`.
6. Run `go build -o zoro ./cmd/zoro` using an output location that does not overwrite unrelated tracked or untracked artifacts.
7. In an isolated clean test repository with fake or test endpoints, enable both auto-plan and auto-implement and verify a new Ready item creates a handoff, moves to In progress, and invokes implementation in one cycle.
8. Seed an existing `handoff/ready` file, disable auto-plan, enable auto-implement, and verify `run --once` implements it without creating a duplicate.
9. Repeat with handoffs in implementing, review, done, and failed states and verify no automatic Codex invocation occurs.
10. Disable auto-implement and verify a newly planned or existing ready handoff remains ready and the GitHub card is not moved to In progress.

## Risks

- The automatic cycle may already hold the repository lock; calling the manual implementation entry point unchanged could self-deadlock or return a misleading lock error.
- A same-cycle generated handoff changes the working tree. The implementation must reconcile this with the documented clean-tree guarantee without ignoring arbitrary developer changes.
- Treating every existing handoff as eligible would rerun in-progress or completed work; eligibility must be restricted to the ready state.
- Treating `failed` as ready would introduce uncontrolled retries on every polling interval, contrary to current idempotency requirements.
- Moving GitHub status outside the existing implementation lifecycle could update the card before repository safety checks pass or could perform the mutation twice.
- Refactoring tightly coupled Cobra code for testability can expand scope; use the smallest dependency seam needed to test cycle decisions and ordering.
- The repository is currently dirty due to an unrelated untracked handoff, so implementation validation that requires a clean tree will need an isolated temporary repository or committed/stashed state managed by the developer.

## Relevant files

- `internal/cli/root.go`: The command layer contains the polling cycle and manual implementation entry point, and is the likely location of the current “existing handoff means skip” behavior. — Primary orchestration change for `run` and `run --once`; share the existing implementation lifecycle while keeping lock ownership explicit.
- `internal/handoff/handoff.go`: `Find` currently scans all state directories and returns only a path and metadata, so orchestration must reliably distinguish ready handoffs from states that must not be executed again. — Potentially add a state-aware lookup result/helper without weakening metadata-based duplicate protection.
- `internal/handoff/handoff_test.go`: Existing tests cover rendering, parsing, and duplicate rejection but not state classification across the handoff lifecycle. — Extend tests for state-aware lookup and duplicate behavior.
- `internal/cli/root_test.go`: The requested behavior crosses configuration gating, planning, handoff lookup, locking, GitHub transitions, and implementation invocation. — Add focused automatic-cycle and implementation-resume tests, creating the file if command orchestration currently has no tests.
- `internal/codex/codex.go`: Codex is the implementation endpoint, but bypassing the surrounding lifecycle would omit status, handoff, branch, and validation behavior. — No structural change expected; automatic orchestration should call this through the existing implementation lifecycle rather than invoking Codex directly.
- `internal/config/config.go`: The required switches and transition settings already exist and should not be duplicated. — No schema change expected; use the existing `AutoPlan`, `AutoImplement`, and behavior flags.
- `README.md`: It already documents automatic implementation but may not state the resume behavior explicitly. — Optional concise clarification of automatic handling for existing ready handoffs.

## Proposed changes

- `internal/cli/root.go`: Change one-cycle orchestration to distinguish an existing ready handoff from already-processing/completed states, implement ready handoffs when `auto_implement` is true, and continue from newly saved handoffs in the same cycle. Reuse a lock-aware internal implementation function rather than recursively invoking the Cobra command. (Risk: This file likely combines dependency construction and business logic; careless reuse could acquire the repository lock twice, bypass clean-tree checks, or duplicate GitHub/Codex calls.)
- `internal/handoff/handoff.go`: Add or refine handoff state lookup so callers can reliably determine whether a matching handoff is ready, implementing, review, done, or failed while retaining metadata-based duplicate detection. (Risk: Changing `Find` directly may affect existing callers. A backward-compatible helper or typed result may be safer than altering matching semantics globally.)
- `internal/handoff/handoff_test.go`: Add handoff state-classification tests, including deterministic matching and ensuring ready is distinguishable from implementing/review/done/failed. (Risk: Tests must use platform-safe paths and should not infer state with hard-coded path separators.)
- `internal/cli/root_test.go`: Add orchestration tests using fakes for GitHub, planner, handoff storage, implementation/Codex, and locking. Cover configuration combinations, existing-state behavior, operation ordering, duplicate prevention, and failure propagation. (Risk: If `root.go` currently constructs concrete external clients internally, a small dependency-injection seam may be required; avoid broad command-layer refactoring solely for tests.)
- `README.md`: Clarify that automatic mode resumes an existing ready handoff as well as implementing a handoff created in the current cycle, if this behavior is not already explicit. (Risk: Documentation must not imply automatic retries of failed or interrupted implementations, or automatic transition to Done.)

## Acceptance criteria

- [ ] With `automation.auto_implement: true`, a newly generated handoff is passed to the existing implementation pipeline during the same `run`/`run --once` cycle. — Use fake planner and implementer dependencies; assert the handoff is saved before implementation is invoked and that both occur exactly once.
- [ ] When the selected Ready project item already has a handoff in `handoff/ready`, automatic mode implements that handoff without regenerating or duplicating it. — Seed a ready handoff, run one cycle, and assert the planner and handoff writer are not called while the implementer is called once with the existing path.
- [ ] Automatic implementation uses the normal implementation lifecycle, including moving the card to the configured In progress status before Codex execution. — In an orchestration test, record calls and assert the ready-to-implementing file transition and GitHub status update occur before the Codex invocation.
- [ ] With `automation.auto_implement: false`, planning may create a handoff but automatic implementation is not invoked. — Run a cycle with automatic implementation disabled and assert the resulting handoff remains in `handoff/ready` and no implementation call occurs.
- [ ] A known handoff already in `implementing`, `review`, or `done` is not executed again; a failed handoff is not retried implicitly. — Add table-driven tests for each handoff state and assert no Codex/implementer invocation occurs.
- [ ] Automatic handling works when `auto_plan` is disabled but a ready handoff already exists and `auto_implement` is enabled. — Seed an existing ready handoff, disable automatic planning, and assert implementation still starts without calling the planner.
- [ ] The automatic cycle does not reacquire a lock it already holds, while manual implementation remains protected by the repository lock. — Test automatic and manual entry points with a fake or temporary lock and verify one acquisition per operation with no self-deadlock.
- [ ] Implementation errors, including repository safety, status transition, Codex, or validation failures, are returned and do not result in duplicate execution or a false success message. — Inject failures at the implementation boundary and assert the cycle returns an error and does not report review/success.
