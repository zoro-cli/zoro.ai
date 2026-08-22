---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3k8hk
issue: 5
title: the filename of the generated markdown file should be max 140 characters only
created_at: 2026-08-22T13:37:43.2773735Z
dirty_at_planning: true
---

# the filename of the generated markdown file should be max 140 characters only

## Summary

Centralize a 140-character cap in handoff filename generation, dynamically reserve space for the issue prefix and `.md` extension, truncate only the normalized slug, and add boundary-focused tests. Preserve short-name behavior, historical files, branch naming, and unrelated dirty-tree changes.

## Objective

Limit every generated handoff Markdown filename to a maximum of 140 characters while preserving deterministic issue-based naming and existing behavior for filenames already within the limit.

## Assumptions

- The 140-character limit applies to the filename basename itself, not the complete directory path.
- The `.md` extension and numeric issue prefix count toward the 140-character limit.
- The existing slug normalization rules should remain unchanged; only the available slug length should be reduced dynamically.
- The limit is measured in characters rather than bytes. If slug generation is strictly ASCII, byte length and character count are equivalent; otherwise truncation should be rune-safe.
- Only newly generated Markdown handoff filenames are in scope. Existing long filenames and Git branch names should not be migrated or shortened.
- The current modifications to `README.md` and `cmd/zoro/zoro.exe` are unrelated user changes and must be preserved.

## Preparation

- Preserve the dirty working tree and do not edit, reset, stash, or rebuild over the modified `README.md` or `cmd/zoro/zoro.exe`.
- Review current filename and slug tests before changing behavior so established normalization and fallback semantics remain intact.
- Determine whether current length checks use bytes or runes and use one consistent definition matching the issue's wording of characters.
- Use a temporary directory or untracked output path for build validation to avoid modifying tracked binaries.

## Implementation steps

1. Inspect `internal/handoff/handoff.go` to locate the canonical slug and filename generation functions, existing slug length limits, empty-slug behavior, and save-path construction.
2. Trace callers from `internal/cli/root.go` to ensure manual `zoro plan`, automatic cycles, duplicate detection, and handoff persistence all obtain filenames from the same handoff helper rather than formatting names independently.
3. Introduce a named 140-character filename limit in the handoff package and calculate the slug budget from the rendered issue prefix, separator, and `.md` suffix.
4. Apply rune-safe truncation to the normalized slug when necessary, then trim separator characters introduced at the truncation boundary. Preserve current fallback behavior for an empty slug, adjusting it only if needed to keep the complete basename valid and bounded.
5. Ensure the final filename construction cannot exceed the limit even for unusually large issue numbers; return a clear handoff error if the fixed components alone cannot fit rather than producing an invalid path.
6. Expand table-driven handoff tests for unchanged short names, exact-boundary names, over-limit names, very long titles, separator-boundary truncation, empty slugs, deterministic output, and complete-basename length.
7. Update CLI-level expectations only where tests currently hardcode generated filenames; avoid broad orchestration changes if filename generation is already centralized.
8. Format changed files and execute the repository validation suite without writing build output over the already modified tracked binary.

## Validation steps

1. Run `gofmt` on changed Go source and test files.
2. Run focused tests such as `go test ./internal/handoff ./internal/cli`.
3. Run `go test ./...`.
4. Run `go test -race ./...`.
5. Run `go vet ./...`.
6. Build with output directed to a temporary or untracked path, for example `go build -o <temp-dir>/zoro ./cmd/zoro`, so the modified tracked executable is not overwritten.
7. Inspect generated basenames from short, exactly-at-limit, and over-limit titles and verify the complete basename is no more than 140 characters and still ends in `.md`.

## Risks

- Ambiguity over whether 140 characters includes `.md`; treating the complete basename as the limit is the safest interpretation of “filename.”
- A fixed slug limit is insufficient because issue numbers have variable length; the slug budget must be calculated from all fixed filename components.
- Naive byte slicing may split UTF-8 code points if Unicode characters are retained by slug generation.
- Truncating at a hyphen can leave a trailing separator unless the shortened slug is normalized again.
- Changing the general slug helper could unintentionally shorten Git branch names or other identifiers; scope the limit to Markdown filename generation unless shared behavior is explicitly intended.
- Duplicate detection based solely on filenames could behave differently for historical long handoffs, although repository guidance indicates metadata-based detection should remain authoritative.
- The repository is already dirty. Validation must not overwrite `cmd/zoro/zoro.exe` or include unrelated README/binary changes in the implementation.

## Relevant files

- `internal/handoff/handoff.go`: The repository specification assigns deterministic filename, slug generation, rendering, and storage behavior to the handoff package. — Implement the complete-basename limit in the existing slug/filename generation and handoff save-path logic.
- `internal/handoff/handoff_test.go`: The existing test suite is identified as covering deterministic filenames and slug generation, making it the primary regression location. — Add exact and over-limit filename tests while retaining existing slug and deterministic filename regressions.
- `internal/cli/root_test.go`: Planning orchestration may assert saved handoff paths and should verify the central helper is used by user-facing flows. — Potentially update or add integration assertions proving generated handoffs use the bounded filename; no change if current tests do not hardcode paths.
- `internal/cli/root.go`: Manual and automatic planning are orchestrated here according to repository context, so both paths must receive the same bounded filename behavior. — Inspect for independent filename construction; change only if a CLI path bypasses the handoff package.

## Proposed changes

- `internal/handoff/handoff.go`: Add a single maximum-filename constant and enforce a dynamic slug budget so the full generated Markdown basename, including issue prefix and extension, never exceeds 140 characters. Keep filename construction and errors deterministic. (Risk: Incorrect budget arithmetic could produce 141-character names, remove the extension, or alter short filenames. Byte slicing could also corrupt UTF-8 if Unicode survives slug normalization.)
- `internal/handoff/handoff_test.go`: Add table-driven regression and boundary coverage for full filename length, deterministic truncation, separator cleanup, empty slugs, and normal filenames. (Risk: Tests that only measure the slug would miss the issue prefix and extension; assertions must measure the complete basename.)
- `internal/cli/root_test.go`: Adjust only hardcoded filename expectations or add a planning-path regression test if CLI tests construct or assert generated handoff paths directly. (Risk: Duplicating filename logic in CLI tests would make them brittle; expected values should exercise the handoff API rather than reimplement its algorithm.)
- `internal/cli/root.go`: No production CLI change should be necessary if all save paths already use the handoff filename helper; remove any independent filename formatting discovered during caller inspection. (Risk: A secondary formatting path could bypass the new limit, especially between manual planning and automatic runner behavior.)

## Acceptance criteria

- [ ] Every newly generated handoff Markdown basename is at most 140 characters, including the issue-number prefix, separators, and `.md` extension. — Add boundary tests that count the complete basename and verify lengths of exactly 140 and greater-than-140 source titles.
- [ ] Short filenames retain the existing `<issue>-<slug>.md` naming behavior without unnecessary truncation. — Keep or add regression cases for normal titles and assert their expected filenames remain unchanged.
- [ ] Long titles are truncated deterministically by reducing only the slug portion while preserving the issue number and `.md` extension. — Generate the filename repeatedly from the same long title and assert identical output, valid prefix/suffix, and a maximum length of 140 characters.
- [ ] Truncation does not leave an invalid trailing hyphen and handles titles that produce an empty slug safely. — Add tests for punctuation-only titles, repeated separators, and titles whose truncation boundary falls beside a hyphen.
- [ ] All handoff creation paths use the bounded filename helper. — Run handoff and CLI tests covering manual planning and automatic planning, or inspect callers to confirm filename construction is centralized.
- [ ] Existing handoff files are not renamed or modified as part of this change. — Confirm the implementation only affects filename generation for newly saved handoffs.
- [ ] The code remains formatted, tested, race-free, vetted, and buildable. — Run gofmt on changed Go files, then `go test ./...`, `go test -race ./...`, `go vet ./...`, and a temporary-output build of `./cmd/zoro`.
