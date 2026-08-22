---
repository: zoro-cli/zoro.ai
project_item_id: PVTI_lADOEw9idc4BhH4szg3lP8E
issue: 17
title: add a config to be compatible to gitlab
created_at: 2026-08-22T14:33:53.8605451Z
dirty_at_planning: false
---

# add a config to be compatible to gitlab

## Summary

Introduce a backward-compatible source-control/work-queue provider configuration, retain GitHub as the default, and add a dedicated GitLab Issue Board client behind a small shared service interface. Extend initialization, authentication, diagnostics, board listing, planning, implementation transitions, and handoff comment synchronization to select the configured provider without duplicating orchestration logic.

## Objective

Add a backward-compatible provider configuration and GitLab integration so Zoro can use either GitHub Projects v2 or a GitLab Issue Board as its configured work queue while preserving the existing planning, handoff, implementation, safety, and idempotency behavior.

## Assumptions

- The issue has no body or explicit acceptance criteria; the criteria in this plan are implementation checks derived from the title.
- “Compatible to GitLab” means GitLab can act as the issue-board/work-queue provider, not merely that a source repository may have a GitLab remote while GitHub Projects remains the queue.
- GitHub remains the default provider so current users and existing `.zoro/config.yaml` files are not broken.
- The first implementation should support GitLab.com and self-managed GitLab instances through a configurable base URL.
- GitLab Issue Boards use labeled lists rather than GitHub Projects v2 single-select status values; Zoro should normalize both into its existing Ready/In progress/In review lifecycle.
- GitLab merge-request creation or automatic merging is outside this issue unless the current implementation already publishes pull requests. Provider-specific publication should be handled separately or extended to merge requests when that feature lands.
- GitLab tokens will be supplied at runtime, preferably through `ZORO_GITLAB_TOKEN`, then `GITLAB_TOKEN`, with optional `glab auth token` fallback; no token belongs in YAML.
- A GitLab project may be identified by a numeric project ID or URL-encoded namespace/project path. The configuration should choose and document one canonical representation while accepting remote-derived nested namespaces.
- GitLab board and issue API details, including pagination and relative-position fields, must be confirmed against the supported API version before finalizing request shapes.

## Preparation

- Confirm the intended GitLab scope before coding from repository history or maintainers’ conventions because the issue body is empty; implement against GitLab Issue Boards unless existing code indicates the request only concerns remote parsing.
- Inspect the omitted production files and tests before choosing interface signatures; current direct dependencies in `internal/cli/root.go` and `internal/github/client.go` will determine the smallest safe seam.
- Confirm the supported GitLab REST API version and exact endpoints for project lookup, board lists/issues, list movement, issue labels, issue notes, pagination, and relative ordering.
- Decide the canonical configuration shape and migration behavior. Prefer a neutral top-level provider selector while retaining the current `github` block for backward compatibility.
- Preserve `.zoro/config.yaml` values unrelated to this feature. Treat it as repository-local effective configuration rather than the only public example.
- Use only fake tokens in tests and documentation. Keep all external service tests on `httptest.Server`; do not call real GitHub or GitLab services.

## Implementation steps

1. Inspect `internal/cli/root.go`, `internal/auth/auth.go`, `internal/github/client.go`, `internal/repository/repository.go`, and their tests to inventory every place that directly assumes GitHub configuration, GitHub client types, GitHub IDs, issue comments, or Projects v2 status updates.
2. Define a small provider-neutral work-queue interface around only the operations currently required by orchestration: verify authentication/project access, inspect workflow metadata, list ordered items, update an item’s workflow state, and ensure an issue discussion comment. Reuse the existing project-item model or move it to a neutral package; do not create abstractions for unrelated helpers.
3. Extend configuration with an explicit provider selector and a typed GitLab section. Keep legacy GitHub YAML valid by defaulting an omitted provider to GitHub. Include only non-secret GitLab values such as base URL, project path/ID, board ID, and configured workflow list/label names.
4. Refactor `Config.Validate` so common planning, scheduler, implementation, handoff, and behavior settings are always validated, while GitHub or GitLab settings are validated only for the selected provider. Reject unknown providers and malformed/non-HTTP(S) GitLab base URLs.
5. Replace positional composite literals in `config.Default` with keyed literals before extending configuration, then provide provider-specific defaults or constructors used by initialization. Ensure YAML save/load and `zoro config` remain deterministic and never contain credentials.
6. Extend remote parsing so repository metadata retains the host as well as namespace and repository. Recognize GitLab.com and self-managed GitLab HTTPS, SCP-style SSH, and `ssh://` remotes, preserving nested group/subgroup paths.
7. Update `zoro init` to select the provider from the origin host, with an explicit flag or prompt for ambiguous/self-managed hosts. For GitLab, resolve the project through the API when needed and require the board identity rather than writing placeholder values.
8. Add GitLab token resolution through the existing authentication/process seams. Implement documented precedence, optional `glab` fallback, whitespace rejection, context cancellation, and sanitized errors. Never pass tokens on command lines or expose them in verbose output.
9. Create `internal/gitlab/client.go` with an injected HTTP client and configurable API base URL. Centralize context-aware requests, authentication headers, user-agent, bounded response handling, pagination, URL escaping, non-2xx decoding, and error sanitization.
10. Implement GitLab authentication and project/board discovery. Resolve the configured project, fetch board lists, and verify that each configured lifecycle mapping exists and is usable. Return diagnostics that name missing configured values and available lists without exposing response headers.
11. Implement GitLab board item listing and normalize issues into the project-item model. Populate stable identity, issue IID, title, description, repository/project path, workflow state, and ordering metadata. Follow pagination and document a deterministic fallback if GitLab does not provide relative position for all items.
12. Implement GitLab workflow transitions using the board/list or issue-label APIs appropriate to the supported GitLab API. Preserve unrelated labels, handle list movement transaction-like failures, and make retries idempotent where the issue is already in the target state.
13. Implement GitLab issue-note synchronization using the same hidden handoff identity marker used for GitHub comments. Search all relevant paginated notes before posting so repeated cycles do not intentionally duplicate the handoff.
14. Change CLI dependency construction to instantiate the selected provider behind the neutral interface. Route `auth`, `doctor`, `board`, `ready`, `plan`, `run`, and implementation status synchronization through that interface while keeping Cobra handlers thin.
15. Audit handoff metadata and duplicate matching for GitHub-specific assumptions. Keep existing metadata readable and introduce neutral provider/item identity fields only if required; maintain compatibility with already generated GitHub handoffs.
16. Add provider-neutral orchestration tests proving the same planning and implementation lifecycle works with GitHub and GitLab fakes. Assert provider selection happens once and no command accidentally calls both providers.
17. Add focused GitLab HTTP tests for authentication, nested project paths, board/list discovery, pagination, ordering, transitions, notes, malformed responses, rate limiting, access denial, and token/error sanitization.
18. Update README configuration and workflow documentation with separate GitHub and GitLab examples, credential environment variables, self-managed host setup, list/status mapping semantics, initialization behavior, and any intentionally unsupported GitLab features.

## Validation steps

1. Run gofmt on all changed Go source and test files.
2. Run focused configuration tests for legacy GitHub YAML, explicit GitHub, explicit GitLab, unsupported providers, malformed hosts, and missing provider-specific fields.
3. Run repository tests for GitHub and GitLab remote parsing, including nested groups and self-managed hosts.
4. Run authentication tests for GitLab environment-token precedence, optional `glab` fallback, cancellation, missing credentials, and secret-safe errors.
5. Run GitLab client tests against `httptest.Server` for project lookup, board/list discovery, paginated issue listing, ordering, transitions, notes, non-2xx responses, malformed JSON, and authorization failures.
6. Run existing GitHub client tests unchanged or expanded to verify no regression after introducing the neutral service interface.
7. Run CLI tests proving omitted provider selects GitHub, explicit GitLab selects only GitLab dependencies, and provider-specific doctor/auth failures are reported clearly.
8. Run orchestration tests for GitLab-backed board, ready, plan, run-once, existing handoff detection, automatic implementation, Ready-to-In-progress, and success-to-review behavior.
9. Verify GitLab note synchronization is idempotent across paginated notes and that draft/non-issue items are handled with a clear unsupported-item message if applicable.
10. Inspect generated and printed configuration to confirm no GitHub or GitLab token is persisted or displayed.
11. Run `go test ./...`.
12. Run `go test -race ./...`.
13. Run `go vet ./...`.
14. Build `./cmd/zoro` to a temporary or otherwise untracked output path.
15. In isolated test projects, perform manual smoke tests against GitLab.com and, if available, a supported self-managed GitLab version; do not claim these passed unless they are actually executed.

## Risks

- GitLab Issue Boards are label-backed and are not semantically identical to GitHub Projects v2 single-select statuses; careless normalization can remove unrelated labels or place an issue in multiple workflow lists.
- The issue is underspecified. Full GitLab board support is a substantially larger change than adding remote parsing or a YAML key, so scope should remain focused on features the current GitHub queue already exposes.
- Backward compatibility is critical because existing configurations have no provider selector and currently require the `github` block.
- Self-managed GitLab hosts cannot always be detected from hostname alone; initialization needs an explicit provider/host override.
- GitLab subgroup paths must be URL-escaped as a whole project identifier without losing separators or allowing path traversal.
- GitLab pagination is common for projects, board issues, and notes. Ignoring pagination can break ordering, diagnostics, and duplicate-comment prevention.
- Ordering metadata may differ by GitLab version or endpoint. The implementation must document and test a deterministic fallback rather than inventing ordering semantics.
- Provider-specific numeric IDs, issue IIDs, global IDs, and repository paths can be confused. Handoff identity must use a stable, unambiguous composite.
- Authentication header and token fallback behavior differs between GitHub and GitLab. Shared logging and process errors must never expose credential values.
- Refactoring CLI code around a service interface could accidentally alter existing GitHub status transition ordering, lock ownership, or partial-failure compensation.
- GitLab rate limits and error payloads differ between hosted and self-managed versions; response handling must tolerate bounded variant payloads while preserving useful context.
- Issue-note idempotency depends on retrieving all existing notes and retaining the hidden marker; pagination or formatting changes can cause duplicates.
- Merge requests are GitLab’s counterpart to pull requests, but publication is not part of the currently documented workflow. Folding merge-request behavior into this issue would expand scope and conflict with separate pending PR work.

## Relevant files

- `internal/config/config.go`: The issue explicitly requests configuration and this file currently requires GitHub settings unconditionally. — Primary configuration schema, defaults, migration behavior, and provider-aware validation.
- `internal/config/config_test.go`: Existing tests cover validation and save/load but not provider selection or conditional fields. — Regression coverage for legacy and GitLab configurations.
- `internal/cli/root.go`: CLI orchestration currently constructs and uses external integrations and must avoid direct GitHub assumptions. — Provider selection and routing across all user-facing operations.
- `internal/cli/root_test.go`: GitLab support must be verified without calling external services. — Provider-routing, initialization, and shared-lifecycle tests.
- `internal/github/client.go`: Current project-board functionality is GitHub-specific and is the reference behavior to preserve. — GitHub implementation should satisfy the new neutral service contract with no behavior change.
- `internal/github/client_test.go`: The new provider seam must not regress organization/user project resolution, ordering, transitions, or comments. — Regression coverage for existing GitHub behavior after abstraction.
- `internal/gitlab/client.go`: GitLab requires distinct API requests and board/list semantics; it is not GitHub API-compatible. — New GitLab REST integration.
- `internal/gitlab/client_test.go`: External GitLab calls must be isolated and testable through injected HTTP dependencies. — New mocked GitLab API test suite.
- `internal/repository/repository.go`: Initialization currently derives GitHub owner/repository from origin and must recognize GitLab remotes. — Host-aware, nested-namespace remote parsing.
- `internal/repository/repository_test.go`: GitLab subgroup paths and self-managed hosts add parsing edge cases. — GitLab and GitHub remote parsing regression cases.
- `internal/auth/auth.go`: Credentials must remain runtime-only and provider-specific. — GitLab token resolution and authentication selection.
- `internal/handoff/handoff.go`: Duplicate detection and lifecycle transitions rely on stable project-item identity currently originating from GitHub. — Potential neutral provider/item identity metadata support.
- `internal/handoff/handoff_test.go`: Existing GitHub handoffs must remain discoverable while GitLab handoffs gain stable matching. — Compatibility tests for provider-neutral handoff identity.
- `README.md`: Current documentation states that GitHub is the queue and assumes a GitHub origin. — GitHub and GitLab setup, credentials, configuration, and limitations.
- `.zoro/config.yaml`: The checked-in effective configuration demonstrates the current schema, though it should not be treated as the sole example. — Optional explicit declaration of the default GitHub provider.

## Proposed changes

- `internal/config/config.go`: Add a provider selector, typed GitLab settings, backward-compatible defaults, conditional validation, and deterministic YAML persistence. Convert positional defaults to keyed literals. (Risk: A schema migration can invalidate existing configurations if omitted provider values or the current mandatory GitHub fields are handled incorrectly.)
- `internal/config/config_test.go`: Add fixtures and table-driven tests for legacy GitHub configuration, explicit providers, valid GitLab settings, invalid hosts/projects/boards/mappings, defaults, and save/load behavior. (Risk: Boolean and omitted YAML values can be indistinguishable; tests should assert effective behavior rather than YAML-node presence unless custom unmarshalling is introduced.)
- `internal/repository/repository.go`: Extend remote metadata and parsing to retain host and nested namespaces for GitLab HTTPS and SSH origins. (Risk: Changing owner/repository parsing can regress existing GitHub remotes or incorrectly truncate GitLab subgroup paths.)
- `internal/repository/repository_test.go`: Add remote parsing tests for GitHub compatibility, GitLab.com, self-managed hosts, nested groups, `.git` suffixes, ports, and malformed remotes. (Risk: URL and SCP-style syntax overlap; fixtures must cover platform-independent parsing without relying on network access.)
- `internal/auth/auth.go`: Add GitLab credential resolution and authentication checks using environment variables and optional `glab` fallback without persisting or logging tokens. (Risk: Invoking `glab` must be context-aware and must not leak token output through errors or verbose logs.)
- Introduce a minimal provider-neutral service contract and shared project-item/workflow types consumed by CLI orchestration. (Risk: An overly broad abstraction would cause unnecessary refactoring; keep it limited to operations already used by the application.)
- `internal/gitlab/client.go`: Implement a context-aware GitLab REST client for project/board discovery, ordered issue listing, workflow transitions, and idempotent issue-note synchronization. (Risk: GitLab board semantics differ from GitHub Projects v2, especially label-based lists, relative ordering, pagination, and moves between lists.)
- `internal/gitlab/client_test.go`: Add comprehensive `httptest` coverage for GitLab request construction, pagination, parsing, ordering, transitions, notes, access failures, and sanitized errors. (Risk: Tests can falsely model GitLab behavior if response fixtures do not match the supported API version; fixtures should mirror documented payloads.)
- `internal/github/client.go`: Adapt the GitHub client to the provider-neutral contract without changing existing request behavior or GitHub Projects v2 semantics. (Risk: Refactoring stable GitHub code could introduce regressions in owner resolution, ordering, status updates, or comment idempotency.)
- `internal/github/client_test.go`: Update GitHub client tests to prove behavior remains unchanged through the new interface. (Risk: Interface-only tests must not replace existing endpoint-level regression coverage.)
- `internal/cli/root.go`: Select the configured provider during command dependency construction and route auth, doctor, board, ready, planning, running, comments, and implementation transitions through the shared service. (Risk: This is the main integration point; partial routing could cause GitLab mode to invoke GitHub APIs or apply incorrect status semantics.)
- `internal/cli/root_test.go`: Add CLI/orchestration tests for default GitHub selection, explicit GitLab selection, provider-specific diagnostics, ordered Ready selection, planning, and implementation transitions. (Risk: Concrete dependency construction may require a small factory injection seam; avoid broad command-layer redesign.)
- `internal/handoff/handoff.go`: Review handoff metadata and duplicate matching so stable GitLab item identities are supported while existing GitHub handoffs remain readable. (Risk: Changing identity matching can create duplicate handoffs or make existing handoffs undiscoverable.)
- `internal/handoff/handoff_test.go`: Add compatibility tests for GitHub and GitLab handoff identities across all lifecycle directories. (Risk: Provider identity values may contain slashes or numeric IDs; they must remain metadata, not unsafe path components.)
- `internal/cli/root.go`: Update initialization to derive GitLab host/project details from origin and generate provider-appropriate configuration, with an explicit override for ambiguous hosts. (Risk: Host-based auto-detection is unreliable for self-managed installations and must permit explicit user selection.)
- `README.md`: Document provider configuration, authentication, GitLab.com/self-managed setup, board-list mapping, initialization, security, and supported feature differences. (Risk: Documentation must not imply parity for pull requests/merge requests or other features not implemented in this issue.)
- `.zoro/config.yaml`: Optionally migrate the repository-local effective config to include the explicit GitHub provider while preserving all current automation values. (Risk: This file enables unattended implementation and may be environment-specific; avoid unrelated edits and do not add credentials.)

## Acceptance criteria

- [ ] Existing configurations that contain only the current `github` section continue to load and behave as GitHub configurations without modification. — Load a legacy fixture with no provider selector and assert the effective provider is GitHub and the existing GitHub settings are preserved.
- [ ] Configuration can explicitly select GitLab and provide the GitLab host, project identity, board identity, status/list mappings, and other required non-secret settings. — Load and validate representative GitLab YAML, then verify `zoro config` prints the effective non-secret values.
- [ ] Configuration validation is provider-aware and reports actionable errors for missing or invalid GitLab settings without requiring irrelevant GitHub settings. — Add table-driven tests for missing host, project, board, status mappings, unsupported provider values, and valid GitHub/GitLab configurations.
- [ ] GitLab credentials are resolved without being stored in `.zoro/config.yaml` or printed in CLI output. — Test token precedence using environment/process fakes and inspect generated configuration and command output for absence of token values.
- [ ] Initialization recognizes supported GitLab remote URL forms and generates a valid GitLab configuration when the origin points to GitLab. — Add repository/init tests for HTTPS, SCP-style SSH, and `ssh://` GitLab remotes, including self-managed hosts and nested group paths.
- [ ] With GitLab selected, authentication and doctor checks verify the configured GitLab user, project, board, and required workflow lists/statuses rather than invoking GitHub APIs. — Use an `httptest.Server` and recording fakes to assert the GitLab endpoints are called and the GitHub client is not constructed or invoked.
- [ ] Board and ready-item operations return GitLab issues using the same internal project-item model and preserve deterministic GitLab board ordering. — Mock paginated GitLab board responses and assert status counts, issue metadata, and Ready ordering based on GitLab relative-position/connection order with a documented deterministic fallback.
- [ ] Planning, automatic cycles, and implementation can operate on GitLab-backed items without duplicating provider-specific orchestration logic. — Run orchestration tests with a fake GitLab service and assert item selection, handoff creation, duplicate detection, and implementation entry use the shared lifecycle.
- [ ] Implementation status transitions map Ready to the configured GitLab in-progress list and successful work to the configured review list, while never moving an item to Done automatically. — Mock GitLab move/update endpoints and assert transition order and that no Done transition occurs.
- [ ] Handoff synchronization posts to the corresponding GitLab issue discussion idempotently when the selected provider is GitLab. — Mock GitLab notes endpoints, seed an existing hidden identity marker, and verify retries do not create duplicate notes.
- [ ] Documentation explains GitHub and GitLab configuration, authentication, initialization, workflow mapping, self-managed host support, and platform limitations. — Review README examples and generated configuration documentation for both providers, using placeholder credentials only.
- [ ] The repository remains formatted, tested, race-free, vetted, and buildable. — Run gofmt on changed Go files, `go test ./...`, `go test -race ./...`, `go vet ./...`, and build `./cmd/zoro` to a temporary output path.
