# Plan: ToolPlane Conformance Mechanisms

## Goal

Implement the minimum ERPBridge mechanisms required to validate approval-to-execution binding in the ToolPlane paper: canonical admission binding, explicit serving selection, one immutable invocation snapshot, synchronized withdrawal/dispatch commitment, final effective-origin validation, and observable reconciliation recovery.

## Decisions

- MCP `tools/list` and `tools/call` are the public execution seams.
- Dispatch commitment linearizes under the same authority synchronization as withdrawal. Withdrawal first prevents connector entry; commitment first may complete.
- Periodic rescan may remain the recovery mechanism. Persist correctness state, not test telemetry.
- Distributed revocation is out of scope.
- Every behavior change follows red-green TDD and updates developer docs, public docs, and `CHANGELOG.md`.

## Tasks

- [x] **Task 1: Canonical admission binding and conflict errors.** Bind executable resource content to an approval digest and reject changed same-version content through the control API. (**Seam:** tool apply/activation API; **Files:** `internal/mcp/tool.go`, `store.go`, API handlers, tests, docs/changelog; **Verify:** `go test ./internal/mcp -run 'Admission|Canonical|Digest|Conflict|Review' -count=1`.)
- [x] **Task 2: Serving pointers and immutable invocation snapshots.** Persist serving selection, advertise exact revision metadata, and carry one cloned resource through the MCP call. (**Seam:** MCP `tools/list`/`tools/call`; **Files:** registry/server/middleware/tool/pipeline/store, tests, docs/changelog; **Verify:** `go test -race ./internal/mcp -run 'Serving|Qualified|Digest|Snapshot|NonServing|NoFallback|Transition' -count=1`.)
- [x] **Task 3: Dispatch commitment, withdrawal, and effective origin.** Synchronize final authority with connector handoff, add acknowledged barriers and ordered sanitized events, and validate final origin. (**Seam:** queue release to connector entry; **Files:** MCP execution, connector, boundary hooks, tests, docs/changelog; **Verify:** `go test -race ./internal/mcp ./internal/connector -run 'FinalAuthority|Barrier|Withdrawal|Origin|Redirect|ZeroDispatch' -count=1`.)
- [x] **Task 4: Reconciliation recovery observability.** Retain the periodic controller if sufficient, persist only restart-critical state, and expose cycle-local status needed for recovery assertions. (**Seam:** desired-state fault and recovery to rediscovery/invocation; **Files:** store/server/status, tests, docs/changelog; **Verify:** `go test -race ./internal/mcp -run 'Reconcile|Recovery|Restart|Tombstone|NoResurrection' -count=1`.)
- [x] **Task 5: Full quality and documentation gate.** Synchronize `erpbridge-docs`, run focused lint and the full suite, and review the diff. (**Verify:** `make test`; `npm run build` in `../erpbridge-docs`; relevant lint; code review.)

## Open Questions

None. Exact API shapes may follow existing resource conventions, but the behavioral contracts above are fixed.
