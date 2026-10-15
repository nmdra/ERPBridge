# Plan: ERPBridge v0.5.0-alpha.5 release

## Goal

Correct the `v0.5.0-alpha.4` regression that made exact-revision names uncallable while they were
hidden from discovery, and publish the fix as a new prerelease.

## Cause

`mcp-go` applies registered tool filters in `filteredTools` and in `passesToolFilters`, and the call
path uses `passesToolFilters`. Hiding the `.rev_` names inside the ERPBridge tool filter therefore
also removed them from `tools/call` dispatch. Verified against the published image: a pinned call
returned `{"code":-32602,"message":"tool 'upgrade_schema_probe.rev_MS4wLjA' not found"}`.

## Decision

Keep registration and dispatch untouched. Filter only the `tools/list` result, through the SDK's
`OnAfterListTools` hook. The listing behaviour and the `MCP_ADVERTISE_REVISION_ALIASES` switch stay
as documented in `v0.5.0-alpha.4`.

## Tasks

- [x] **Task 1: Move the hiding to the listing hook.** Restore the tool filter to active-revision
  filtering only, and add `hideRevisionAliases` as an after-list hook when advertising is disabled.
  (**Files:** `internal/mcp/server.go`; **Verify:** `go test ./internal/mcp -run
  'TestRevisionAliases|TestHiddenRevisionAlias|TestUnqualifiedCall' -count=1`.)
- [x] **Task 2: Test through the protocol seam.** Drive `tools/list` and `tools/call` through
  `HandleMessage` so the tests cover the SDK filter and dispatch behaviour, not only the helper.
  (**Files:** `internal/mcp/revision_alias_discovery_test.go`; **Verify:** `go test ./internal/mcp
  -count=1`.)
- [x] **Task 3: Quality gate.** Full suite, focused lint, commit hooks. (**Verify:** `make test`;
  `golangci-lint run ./internal/mcp/... ./services/...`.)
- [x] **Task 4: Container verification.** Build the local image, run the restore-before-start
  sandbox harness, and confirm: one discovery entry per tool, no `.rev_` names listed, a pinned
  name still callable, 46 revisions and 3 token records preserved. (**Verify:** harness report.)
- [x] **Task 5: Publish and verify the digest.** Tag `v0.5.0-alpha.5`, push, wait for the release
  workflow, pull the published digest, and repeat the harness against it. (**Verify:** recorded
  digest plus a passing run.)

## Outcome

Published on 2026-10-15 and verified against the published digest
`ghcr.io/nmdra/erpbridge-server@sha256:2ae683b46d4b7cacd4e23f058cbad8768c520ae73a66619e871ed101a09f0575`.
The restore-before-start sandbox run reported 46 revisions / 45 names preserved, 45 discovery names
with no `.rev_` entry, a pinned name that still executes, three unchanged token records, one held
client token authenticated, eight invalid-input cases rejected on both transports, and no ERP call.
The local candidate build `erpbridge-local:alpha5-fix` passed the same harness first.

## Follow-up

The ACA upgrade plan then targets the verified `v0.5.0-alpha.5` digest with the existing
restore-before-start gate.
