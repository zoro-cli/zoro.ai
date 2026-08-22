---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3k4Qc
issue: 1
title: add spinning icon on cli when zoro run ... to indicate its running and waiting
created_at: 2026-08-22T12:51:47.5874869Z
dirty_at_planning: false
---

# add spinning icon on cli when zoro run ... to indicate its running and waiting

## Summary

Add a small terminal-aware spinner to continuous `zoro run`, showing that Zoro is alive while waiting for the next configured polling cycle. Stop and clear it before command output, on cancellation, and on exit; keep `--once` unchanged and keep redirected output free of animation noise.

## Objective

Provide visible, non-disruptive feedback that continuous `zoro run` is active and waiting for its next polling cycle, while preserving clean output, cancellation behavior, and cross-platform CLI operation.

## Assumptions

- The requested spinner primarily represents the idle polling wait in continuous `zoro run`; `--once` has no waiting interval to represent.
- Existing cycle result messages such as `No Ready items.` and `Handoff created: ...` should remain unchanged and readable.
- Animation should be enabled only for an interactive terminal so logs, pipes, and redirected output remain machine-readable.
- A small standard-library spinner is preferable to introducing a terminal UI framework into the run loop; existing terminal-detection dependencies may be reused if needed.

## Preparation

- Inspect Cobra output stream conventions in the command layer and choose stderr for transient spinner rendering so normal stdout remains suitable for command results.
- Confirm the terminal-detection API already available through current dependencies; promote it to a direct module dependency only if the spinner imports it directly.
- Identify all return paths in continuous `run` so the spinner is always stopped before exit or before cycle output begins.

## Implementation steps

1. Add a small spinner abstraction with explicit start and stop behavior, a fixed frame sequence, a bounded refresh interval, context-aware shutdown, and terminal-aware rendering.
2. Ensure spinner rendering uses the command's error/output stream rather than hard-coded global output where practical, so tests can capture output and terminal messages remain consistent.
3. Update continuous `run` orchestration so a cycle runs, the waiting spinner starts for the configured interval, the spinner stops and clears immediately before the next cycle, and cancellation stops it before returning.
4. Keep `--once` on the existing single-cycle path without starting the polling-wait spinner.
5. Make spinner shutdown idempotent and synchronize its goroutine so no frame can be written after stop returns.
6. Add unit tests using a buffer and controllable timing/context to verify frames render, stop is idempotent, cancellation terminates rendering, and non-interactive mode avoids animation/control sequences.
7. Add or extract a testable wait helper around the continuous run loop if necessary to verify that spinner start/stop boundaries align with ticker events and cancellation without invoking GitHub or OpenAI.
8. Document the continuous command's spinner behavior briefly in the command documentation if user-visible output behavior is described there.

## Validation steps

1. Run `gofmt -w internal/cli/root.go internal/cli/spinner.go internal/cli/spinner_test.go` for the changed Go files.
2. Run `go test ./...`.
3. Run `go test -race ./...` to detect spinner lifecycle and output synchronization races.
4. Run `go vet ./...`.
5. Run `go build -o zoro ./cmd/zoro`.
6. Configure a short polling interval, run `zoro run` interactively, and verify the spinner animates only while waiting and clears before each cycle's output.
7. Interrupt `zoro run` with Ctrl+C during the wait and verify immediate, clean termination.
8. Run `zoro run --once` and verify one cycle executes without entering the waiting spinner state.
9. Redirect continuous-run output during a bounded test invocation and verify no ANSI erase sequences or repeated spinner frames appear in the captured non-terminal output.

## Risks

- Spinner frames can interleave with existing `fmt.Println` and `fmt.Fprintln` calls unless the spinner is stopped before every cycle or error message.
- A leaked ticker or rendering goroutine could cause race-test failures or write after Cobra has returned.
- ANSI carriage-return and erase sequences can pollute CI logs, redirected files, or unsupported terminals if terminal detection is omitted.
- Signal cancellation can occur concurrently with a ticker event, so cleanup must be idempotent and safe regardless of which path wins.
- Testing animation with real timers may be flaky; timing and output should be injectable or tightly bounded.
- Displaying a spinner during long planning/API work would require broader progress-output coordination; limiting this change to the explicit polling wait avoids misleading labels and output collisions.

## Relevant files

- `internal/cli/root.go`: Contains `runCmd`, its polling ticker, cycle output, and signal-aware context handling. — Modify the continuous run loop to start, stop, and clear the waiting spinner at the correct lifecycle points.
- `internal/cli/spinner.go`: Keeps transient terminal rendering separate from the already large Cobra command file and allows focused testing. — Add the reusable CLI spinner implementation.
- `internal/cli/spinner_test.go`: No current CLI tests cover terminal animation, goroutine cleanup, or non-interactive output. — Add spinner and cancellation lifecycle tests.
- `go.mod`: The module already includes terminal-related transitive dependencies, but directly imported packages must be declared appropriately. — Potentially promote a terminal-detection library from indirect to direct, depending on implementation choice.
- `go.sum`: Dependency metadata may change if a terminal-detection dependency is promoted or added. — Potential checksum adjustment following module maintenance.
- `README.md`: Documents `zoro run`, continuous polling, and signal-based exit behavior. — Optional concise user-facing note about the continuous waiting indicator.

## Proposed changes

- `internal/cli/root.go`: Integrate spinner lifecycle into continuous `zoro run`: display an animated `Waiting for next polling cycle...` status after each cycle, stop and clear it before the ticker-triggered cycle, and clean it up on context cancellation. Preserve the one-cycle behavior of `--once`. Optionally extract the polling wait into a small helper to make lifecycle tests independent of external services. (Risk: The current run loop and cycle function write directly to stdout/stderr; incorrect ordering could interleave spinner frames with cycle output or leave a goroutine active after cancellation.)
- `internal/cli/spinner.go`: Introduce a lightweight spinner implementation with terminal detection, frame timing, context/stop handling, synchronized writes, idempotent cleanup, and ANSI line clearing only for interactive terminals. (Risk: Terminal capabilities differ across Windows, Linux, and macOS; animation and line clearing must be gated to avoid corrupting redirected output.)
- `internal/cli/spinner_test.go`: Add deterministic tests for animation output, non-interactive behavior, cancellation, repeated stop calls, and absence of writes after shutdown. Where the run wait is extracted, test ticker/cancellation boundaries as well. (Risk: Real-time sleeps can make tests flaky; tests should use short bounded intervals or injected ticks rather than depending on wall-clock scheduling.)
- `go.mod`: Update module declarations only if a currently indirect terminal-detection package is imported directly by the spinner implementation. (Risk: Avoid adding a new UI dependency solely for a simple spinner; keep dependency changes minimal.)
- `go.sum`: Refresh dependency checksums only if module dependency classification or versions change. (Risk: No functional risk beyond unnecessary module churn, which should be avoided.)
- `README.md`: Mention that continuous `zoro run` shows an interactive waiting indicator and that redirected output remains plain, if command behavior documentation is updated. (Risk: Low; documentation must not imply that `--once` waits or that the spinner reports detailed operation progress it does not provide.)

## Acceptance criteria

- [ ] `zoro run` displays an animated spinner with a clear waiting message between polling cycles when stderr is an interactive terminal. — Run `zoro run` in a terminal and verify the icon animates while waiting for the configured polling interval.
- [ ] The spinner is stopped and its line is cleared before a new polling cycle writes normal command output, preventing interleaved or corrupted terminal text. — Use a short scheduler interval and verify each cycle's messages remain readable without spinner characters embedded in them.
- [ ] The spinner stops cleanly when `zoro run` exits through SIGINT, SIGTERM, command cancellation, or an error. — Interrupt continuous mode during its wait period and verify the prompt returns without a lingering spinner goroutine or partial terminal state.
- [ ] `zoro run --once` does not enter or display the between-cycle waiting state. — Run `zoro run --once` and verify it executes one cycle, exits, and does not leave an animated spinner running.
- [ ] Redirected and non-interactive output remains stable and does not contain ANSI cursor-control sequences or repeated animation frames. — Redirect `zoro run` output to a file under a short-lived context and inspect it for animation frames and escape sequences.
- [ ] Spinner lifecycle behavior is covered by deterministic unit tests and the repository continues to pass formatting, tests, race detection, vetting, and build checks. — Run `gofmt`, `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build -o zoro ./cmd/zoro`.
