# Plan: ERPBridge v0.5.0-alpha.4 release

## Goal

Cut a prerelease that restores the pre-`v0.5.0-alpha.3` client view: one MCP discovery entry per
tool name. Exact-revision names stay registered, callable, and digest-bindable; they are listed only
when `MCP_ADVERTISE_REVISION_ALIASES=true`.

## Decisions

- Hide exact-revision names from `tools/list` by default instead of deleting the mechanism. Digest
  binding, admission digests, immutable per-call snapshots, withdrawal commitment, and origin
  validation stay unchanged, so the ToolPlane conformance evidence still holds.
- Keep the change behind one environment variable. No store schema change, no API change, no
  migration.
- Release through the existing tag-triggered GoReleaser workflow. No second publishing path.

## Tasks

- [x] **Task 1: Hide exact-revision names by default.** Gate the discovery filter on
  `MCP_ADVERTISE_REVISION_ALIASES` (default false) while keeping registration and call handling
  intact. (**Files:** `internal/mcp/server.go`; **Verify:** `go test ./internal/mcp -run
  'TestRevisionAliases|TestHiddenRevisionAlias' -count=1`.)
- [x] **Task 2: Keep the exact-revision path covered.** Add discovery tests for the default and the
  opt-in, and make the existing exact-revision tests set the opt-in. (**Files:**
  `internal/mcp/revision_alias_discovery_test.go`, `internal/mcp/revision_binding_test.go`;
  **Verify:** `go test ./internal/mcp -count=1`.)
- [x] **Task 3: Synchronize documentation.** Update `docs/tool-schema.md`,
  `docs/toolplane-conformance.md`, `docs/environment-variables.md`, `docs/docker.md`, and
  `CHANGELOG.md`. (**Verify:** documentation names the variable and its default in every place the
  discovery behaviour is described.)
- [x] **Task 4: Quality gate.** Run the full suite and focused lint. (**Verify:** `make test`;
  `golangci-lint run ./internal/mcp/... ./services/...`.)
- [x] **Task 5: Publish.** Commit, tag `v0.5.0-alpha.4`, and push so the release workflow publishes
  the GitHub release and the `ghcr.io/nmdra/erpbridge-server:0.5.0-alpha.4` image. (**Verify:**
  workflow completes; the image digest is recorded for the deployment step.)

## Outcome

Published on 2026-10-15 as `v0.5.0-alpha.4`. Container verification found one regression: hiding
the names through the SDK tool filter also removed them from `tools/call` dispatch, so pinned names
returned `tool not found`. `v0.5.0-alpha.5` fixes this; see
[`Plan-Release-v0.5.0-alpha.5.md`](../active/Plan-Release-v0.5.0-alpha.5.md).

## Validation note

Local Docker verification with the release image is a separate step. Deploying it to the live
Container App also requires a fresh consistent database snapshot and the restore-before-start gate.
