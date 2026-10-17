# Plan: PR #20 review fixes

## Goal

Fix the four accepted review findings without changing ERPBridge runtime authorization or adding management operations. The user's request to fix the findings approves this scope.

## Current State

- `internal/climcp/catalog.go:1239-1259` passes confirmed tool identifiers to the existing CLI; `internal/cli/tool.go:571` interpolates them into a query. `internal/mcp/server.go:1001-1027` interprets an injected `hard=true` as permanent deletion.
- `internal/climcp/catalog.go:527-542` reads endpoint fields from a case-sensitive map, while the JSON child decoder accepts case-insensitive field names.
- `internal/climcp/catalog.go:574-578` has a credential-key list separate from `internal/logger/redact.go:111-123`, missing established sensitive spellings.
- `internal/climcp/result.go:243-246` omits empty `items` lists.
- The original management MCP plan is completed. Active, upcoming, and stalled plan inventories contain no task that covers these corrections. Existing dirty plan files are user-owned.

## Decisions

- Encode delete query values with `url.Values`; preserve literal identifiers instead of introducing an unrelated version restriction.
- Reject noncanonical case aliases along endpoint field paths before staging, so the child's case-insensitive decoder cannot bypass the checks. Test aliases and conflicting casing.
- Share the sensitive argument-key classification between manifest screening and logging, preserving existing credential spellings and schema declarations.
- Preserve `items: []` for inventories and the named-get `item` shape without adding an irrelevant collection to named results.
- Use isolated local fixtures only. No production operations, dependency changes, or runtime policy changes.

## Scope

In scope: these four fixes, focused regression tests, developer/public management MCP documentation, Unreleased changelog entries, and local acceptance checks. Out of scope: broader CLI redesign, new tools, live deployment tests, and unrelated plan edits.

## Tasks

- [x] Correct and verify all four review findings as one atomic remediation task. Independent file-owned parts may run in parallel, but final acceptance is sequential. **Files:** `internal/cli/tool.go` and its tests; `internal/climcp/catalog.go`, `result.go` and focused regression tests; `internal/logger/redact.go` and tests; `docs/bridgectl-mcp.md`; `CHANGELOG.md`; matching management MCP guide and changelog in `../erpbridge-docs`. **Seam:** failing unit tests first, compiled bridgectl child against recording HTTP fixtures for deletion, adapter validation with zero staging/execution for rejected secrets, and serialized MCP inventory results. **Verify:** focused tests, fresh race tests on changed packages, `make test`, changed-package lint, active LSP checks on changed Go paths, `git diff --check`, and public documentation build. Review the final diff independently, then make scoped Conventional Commits for product and synchronized public docs.

## Verification evidence

- Red: new adapter tests failed on endpoint aliases, shared sensitive-key spellings, schema defaults, and missing empty inventory arrays before the fixes. New CLI and compiled-child deletion tests failed on injected hard deletion and reserved-character corruption before query encoding.
- Green: `make test`; `go test -race -count=1 ./internal/cli ./internal/climcp ./internal/logger`; `golangci-lint run ./internal/cli/... ./internal/climcp/... ./internal/logger/...` (0 issues); active full LSP probe of all 8 changed Go files (0 errors); `git diff --check` in both repositories; `npm --prefix ../erpbridge-docs run build` (76 documents).
- Delta diagnostics also reported the pre-existing print-style warnings in `internal/cli/tool.go`; no new diagnostic errors were found.
- Independent read-only final-diff review returned no findings.
- Public management guide and changelog are synchronized. The user approved a local public-docs commit on main; nothing is pushed.
- No runtime authorization or server handler changes, dependency changes, or production calls were needed. Existing user-owned plan edits remain untouched.

## Acceptance

- `hard=false` cannot add a hard-delete query through name or version, and the server sees the exact confirmed identity.
- Credential-bearing endpoint aliases fail before staging or execution, including conflicting case variants.
- Established sensitive-key spellings fail while logical credential references and schema declarations remain valid.
- Empty tool/plugin/binding inventories serialize with `items: []`; named results retain only `item`.
- Tests pass, no credentials enter logs/results, and user-owned plan changes remain untouched.
