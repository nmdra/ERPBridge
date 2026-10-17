# Plan: PR #20 CI build-home isolation

## Goal

Push the accepted review fixes to PR #20 and correct its failing CI check without changing runtime behavior or weakening CI. The user's request approves this bounded fix and the PR-branch push.

## Evidence

- GitHub run `39027506094`, job `frontend and console checks`, fails during `go test ./...`: `TestRealChildReadFamilies` cannot remove read-only module-cache files under its temporary HOME.
- `internal/climcp/catalog_child_test.go:219-254` changes HOME before calling `realCLI(t)`. `realCLI` runs `go build` with the current process environment at lines 44-48. Without an explicit GOPATH, the module cache falls under the temporary HOME.
- PR fix commit `c1f9661` was pushed successfully to `origin/feat/bridgectl-mcp-stdio` before this fixture change.
- No active/upcoming/stalled plan covers this CI fixture defect. Existing user-owned plan edits remain out of scope.

## Decision

Capture the Go build environment at test-package initialization and use it explicitly for the shared CLI build. This covers both the session helper and direct callers that change HOME before building, without depending on test order. Keep runtime children isolated to fixture HOME and credentials. Add a deterministic regression that observes the build environment without fetching modules or relying on filesystem permission behavior. Do not change workflow checks, dependencies, runtime server code, or production documentation.

## Task

- [x] Add the regression first, demonstrate failure, then pin the builder to the host environment. **Files:** `internal/climcp/catalog_child_test.go` and a focused new adjacent regression test. **Seam:** isolated test subprocess with a fake Go builder records build HOME; the ordinary real-child tests prove runtime HOME isolation remains intact. **Verify:** focused red/green tests; `go test -count=1 ./internal/climcp`; `go test -race -count=1 ./internal/climcp`; `make test`; scoped `golangci-lint run ./internal/climcp/...`; active LSP checks; `git diff --check`. Commit only the fixture correction, regression, and this completed plan. Push the PR branch and verify the GitHub check on the new head.

## Local verification evidence

- The regression failed before the fix: the fake builder observed fixture HOME instead of host HOME. A caller-order-only fix still failed the direct-caller case.
- The final fix pins `build.Env` to the environment captured at test-package initialization. Both session and direct-first-build regression cases pass, without module downloads. The session case also checks that the runtime child still uses a different fixture HOME.
- `go test -count=1 ./internal/climcp`, `go test -race -count=1 ./internal/climcp`, `make test`, scoped `golangci-lint run ./internal/climcp/...` (0 issues), active delta/full LSP probes (0 diagnostics), and `git diff --check` pass.
- The original CI failure repeated on `c1f9661` in run `39029834241`, confirming the separate fixture correction is required. Final GitHub acceptance is checked after pushing this commit, rather than inferred from these local gates.
- Test-only change; no product behavior, workflow, public documentation, or dependencies changed.

## Boundaries

No approval bypass, merge, force push, public-docs push, or unrelated local-file staging. CI success must be observed on the final PR head, not inferred from local tests.
