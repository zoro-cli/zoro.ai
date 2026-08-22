---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3k5Wk
issue: 2
title: if theres a Ready item, and the handoff is not yet commented to the item, it should comment the content of the handoff to the item/card
created_at: 2026-08-22T13:02:39.9993223Z
dirty_at_planning: false
---

# if theres a Ready item, and the handoff is not yet commented to the item, it should comment the content of the handoff to the item/card

## Summary

Add idempotent GitHub issue-comment synchronization to the Ready planning flow. Zoro will post the full saved handoff with a hidden identity marker, detect existing marked comments across paginated results, and reconcile historical local handoffs without invoking the planner again.

## Objective

Automatically mirror each Ready item's local handoff Markdown into the associated GitHub issue discussion exactly once, including reconciliation of handoffs that already exist locally but have not yet been commented.

## Assumptions

- The phrase “comment to the item/card” refers to creating an issue comment for project items backed by repository issues; GitHub Projects v2 cards themselves do not provide an independent general-purpose comment thread.
- The complete local handoff Markdown should be included in the comment, with a small hidden deterministic marker appended for idempotency.
- Only items in the configured Ready status are candidates for this synchronization; Zoro should not add comments while reconciling handoffs in implementing, review, done, or failed states.
- Existing handoffs should be synchronized without invoking the planner again.
- The repository operation lock should cover automatic comment reconciliation so concurrent Zoro cycles do not normally race; remote marker detection provides repeat-run idempotency.
- GitHub's issue-comment body limit must be checked before posting; oversized handoffs should produce a clear error rather than being silently truncated.

## Preparation

- Review internal/cli/root.go for the implementations of zoro plan, zoro run --once, dependency construction, Ready-item selection, and current duplicate-handoff behavior.
- Review internal/github/client.go for its shared REST/GraphQL request helpers, repository addressing, pagination conventions, error wrapping, and interfaces used by CLI or runner tests.
- Confirm whether manual zoro plan and automatic RunOnce share one planning function; place synchronization in shared business logic where possible so behavior is consistent.
- Confirm GitHub issue comment size constraints and establish a conservative byte/rune check before sending the request.
- Locate existing fake GitHub services or HTTP fixtures so the new methods can be added without broad interface churn.

## Implementation steps

1. Inspect the current planning and RunOnce paths in internal/cli/root.go and any runner/service abstractions to identify where duplicate handoffs are currently skipped and where GitHub dependencies are constructed.
2. Extend the GitHub client/service contract with context-aware operations to list issue comments and create an issue comment for the configured owner/repository and issue number.
3. Implement REST pagination for issue comments and bounded response decoding, reusing the client's shared authenticated request behavior and existing GitHub error handling.
4. Define a stable hidden marker derived from immutable handoff identity, preferably repository plus project_item_id, and helper logic to append the marker and recognize it in existing comments.
5. Add a handoff helper that reads the existing Markdown file and builds the exact comment body, validating that the item is issue-backed and that the final body fits GitHub's accepted size.
6. Introduce an orchestration helper such as EnsureHandoffComment that lists comments, returns without mutation when the marker exists, and otherwise creates one comment containing the handoff Markdown.
7. Call comment synchronization immediately after a new handoff is successfully saved.
8. Change the existing-handoff branch for a Ready item so it reads and synchronizes that handoff before returning/skipping planning, satisfying migration for handoffs created before this feature.
9. Preserve safe failure ordering: planner/save failures must never comment; comment failures must leave the local handoff available for a later retry and must not create another handoff.
10. Add GitHub HTTP tests for comment listing, pagination, creation payloads, authentication errors, GraphQL/REST error sanitization as applicable, and context cancellation.
11. Add orchestration tests covering new handoffs, historical uncommented handoffs, already-commented handoffs, malformed/unreadable handoffs, non-issue project items, oversized comments, and API failures.
12. Update user-facing workflow documentation to state that Ready handoffs are mirrored to their GitHub issue comments and that repeat cycles use a hidden marker to avoid duplicates.

## Validation steps

1. Run gofmt on every changed Go source and test file.
2. Run go test ./....
3. Run go test -race ./....
4. Run go vet ./....
5. Run go build -o zoro ./cmd/zoro.
6. With an httptest-backed GitHub client, verify GET /repos/{owner}/{repo}/issues/{issue}/comments follows pagination and POST to the same comments endpoint sends the expected Markdown and marker.
7. Run a planning-cycle test with no handoff and verify save occurs before exactly one comment creation.
8. Run a planning-cycle test with an existing uncommented handoff and verify no planner call occurs but one comment is created from the existing file.
9. Run the same cycle with an existing marker-bearing comment and verify no POST occurs.
10. Verify comment API failure leaves the handoff in handoff/ready and a subsequent successful cycle can post it without creating another handoff.
11. Verify issue-number-zero/draft items and oversized handoffs return clear errors and make no comment request.

## Risks

- GitHub issue comments have a body-size limit; large generated handoffs may fail unless checked before submission.
- A check-then-create sequence is not transactionally atomic. The repository lock protects normal local Zoro concurrency, but two independent machines could still post duplicates concurrently.
- Using issue number alone as the marker identity could collide across repositories or after project-card replacement; repository and project_item_id should be included.
- Only issue-backed project items are commentable through the Issues API; draft issues or unsupported content types need explicit handling.
- Failure after saving but before commenting leaves partial synchronization. This is intentional and must be retryable on the next cycle without replanning.
- Existing malformed handoff frontmatter may prevent reliable identity verification; errors should be surfaced rather than posting content to a potentially wrong issue.
- Listing only the first page of comments can miss an older handoff comment and create a duplicate.
- Posting raw handoff Markdown may mention repository internals already intentionally stored in the handoff; no additional secrets or API payloads should be logged during synchronization.

## Relevant files

- `internal/github/client.go`: GitHub API access belongs in the dedicated client and must reuse its authentication and HTTP safety behavior. — Add issue-comment list/create support and any minimal service-interface additions required by orchestration.
- `internal/github/client_test.go`: The feature introduces new external API behavior that must not call real GitHub in tests. — Add mocked REST tests for comment pagination, creation, failure handling, and request safety.
- `internal/handoff/handoff.go`: This package owns handoff identity, metadata parsing, rendering, lookup, and storage. — Add deterministic publication marker/body helpers and safe handoff file loading if these concerns are not already available.
- `internal/handoff/handoff_test.go`: Existing tests already exercise handoff rendering, parsing, saving, and duplicate detection. — Expand coverage for marker identity, body preservation, malformed handoffs, and size validation.
- `internal/cli/root.go`: Repository context indicates Cobra command orchestration currently lives here, including planning and automatic cycles. — Update shared plan/run orchestration so both newly created and pre-existing Ready handoffs are ensured as issue comments.
- `README.md`: The comment synchronization is visible behavior in the documented Ready-to-handoff workflow. — Add a concise workflow note about automatic handoff comments and idempotency.

## Proposed changes

- `internal/github/client.go`: Add REST issue-comment models and methods for paginated comment lookup and comment creation. Reuse authentication, timeouts, response bounds, context cancellation, and sanitized error handling. (Risk: Pagination or response handling mistakes could miss an existing marker and create duplicate comments; request errors must not expose authorization data.)
- `internal/github/client_test.go`: Add HTTP-level tests for listing comments across pages, finding marker-bearing comments, posting Markdown JSON payloads, API failures, and cancellation. (Risk: Fixtures must model GitHub pagination accurately enough to catch duplicate-protection regressions.)
- `internal/handoff/handoff.go`: Add helpers to read a handoff for publication and construct/check a stable hidden comment marker based on repository and project item identity. Validate issue association and comment size without modifying the saved handoff. (Risk: Changing rendered handoff files solely to add synchronization metadata could create unnecessary churn; marker generation should remain separate and deterministic.)
- `internal/handoff/handoff_test.go`: Extend handoff tests for deterministic marker construction, full Markdown preservation, malformed files, and maximum comment size behavior. (Risk: Tests should avoid depending on timestamps or platform path separators.)
- `internal/cli/root.go`: Integrate ensure-comment behavior into the shared planning cycle: synchronize after saving a new handoff and, critically, synchronize an existing handoff before treating a Ready item as already planned. (Risk: If synchronization is placed only after generation, historical handoffs will remain uncommented; if placed before status and issue validation, invalid requests may be attempted.)
- Add or update orchestration tests with fake planner/GitHub dependencies to prove new, existing, and already-commented handoffs behave idempotently. (Risk: The exact test file depends on whether current planning orchestration is tested in internal/cli or a runner package; avoid moving unrelated command logic as part of this issue.)
- `README.md`: Document that Ready handoffs are posted to issue comments and repeat cycles do not intentionally duplicate them. (Risk: Documentation must clarify that draft project items without issue discussions cannot receive issue comments.)

## Acceptance criteria

- [ ] When a GitHub Project item is in the configured Ready status and has a local handoff, Zoro posts the handoff Markdown as a comment on the associated GitHub issue. — Use a fake GitHub service with a Ready issue and existing handoff; run one planning cycle and assert one comment is created with the handoff content.
- [ ] A newly generated handoff is commented on its issue after the handoff is saved successfully. — Test the planning path with no prior handoff and assert the saved Markdown is passed to the GitHub comment operation.
- [ ] An existing handoff that was previously skipped as already planned is still synchronized to GitHub when no matching handoff comment exists. — Create an existing handoff fixture, return no matching issue comments, and verify the planner is not called while the existing file is read and posted.
- [ ] Zoro does not post duplicate handoff comments when the issue already contains the comment for that project item. — Return a paginated comment list containing the stable Zoro handoff marker and assert no comment creation request is made.
- [ ] Comment lookup and creation failures are surfaced and do not generate a second handoff or falsely report successful synchronization. — Inject list-comments and create-comment failures and assert the cycle returns contextual GitHub errors while the local handoff remains intact.
- [ ] Items without a commentable GitHub issue are handled explicitly rather than targeting an unrelated issue or constructing an invalid API request. — Test a Ready draft/non-issue item with issue number zero and assert a clear unsupported/not-commentable result with no comment request.
- [ ] GitHub comment requests use authenticated, context-aware API calls without logging credentials or raw authorization headers. — Use an HTTP test server to verify method, path, headers, request body, cancellation behavior, and sanitized error output.
- [ ] The implementation remains formatted, testable, race-free, vetted, and buildable. — Run gofmt on changed Go files, go test ./..., go test -race ./..., go vet ./..., and go build -o zoro ./cmd/zoro.
