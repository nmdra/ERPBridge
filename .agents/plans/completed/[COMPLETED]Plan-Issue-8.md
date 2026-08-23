# Plan: Fix cold-start MCP invocation metrics

## Goal

Ensure Prometheus exposes MCP tool invocation and duration metric families
immediately after tools are registered, with zero-valued series available
before any tool call. Preserve the existing labels and runtime increments.

## Current State

- `ToolInvocationsTotal` and `ToolLatency` are registered as `CounterVec` and
  `HistogramVec` collectors in `internal/metrics/metrics.go`, but no labeled
  child is created there.
- `MetricsMiddleware` creates the labeled children only after a call reaches
  `WithLabelValues` in `internal/mcp/middleware.go:159-160`.
- Declarative tools pass through `Server.RegisterTool` in
  `internal/mcp/server.go:456-504`; built-in tools are added directly in
  `RegisterBuiltinTools` at `internal/mcp/server.go:117-185`.
- `/metrics` serves the default Prometheus gatherer via
  `services/erpbridge-server/main.go:160`, so initialized zero-valued children
  are exported without changing the HTTP route.
- The existing metrics tests verify non-nil collectors and usage, but do not
  assert a cold-start scrape or the tool-registration path.

## Decisions

- Initialize a `SUCCESS` invocation counter child and a duration histogram
  child for each registered tool. Calling `WithLabelValues` creates a child at
  zero without incrementing it, which is the supported client behavior.
- Put initialization behind a small exported metrics-package helper so both
  declarative and built-in tool registration use the same label contract.
- Keep error-series creation lazy: an `ERROR` series should only appear after
  an error is observed, avoiding speculative label values.
- Do not add a synthetic tool label or alter metric names, labels, route
  wiring, or runtime middleware accounting.

## Scope

In scope: eager initialization for MCP tool invocation/duration series,
registration-path tests, metrics documentation, and the Unreleased changelog.

Out of scope: cache metric cardinality, Prometheus configuration, alert rules,
and changes to MCP request execution.

## Tasks

- [x] Task 1: Add a failing registration-seam test that registers a uniquely
  named tool and verifies zero-valued invocation and duration series can be
  gathered before invocation. **Seam:** `Server.RegisterTool` plus the default
  Prometheus gatherer. **Files:** `internal/mcp/server_test.go`.
  **Verify:** `GOCACHE=/tmp/erpbridge-go-cache-20260822 go test ./internal/mcp -run TestServer_RegisterToolInitializesMetrics`
- [x] Task 2: Add the shared metrics initialization helper and invoke it for
  declarative and built-in tools. **Seam:** tool registration. **Files:**
  `internal/metrics/metrics.go`, `internal/mcp/server.go`.
  **Verify:** `GOCACHE=/tmp/erpbridge-go-cache-20260822 go test ./internal/metrics ./internal/mcp`
- [x] Task 3: Document cold-start metric availability and record the fix.
  **Files:** `docs/api.md`, `CHANGELOG.md`.
  **Verify:** `rg -n "cold-start|zero|mcp_tool_invocations_total|mcp_tool_duration_seconds" docs/api.md CHANGELOG.md`
- [x] Task 4: Run repository verification and close this plan as completed.
  **Verify:** `GOCACHE=/tmp/erpbridge-go-cache-20260822 make test` and
  `golangci-lint run ./internal/metrics/... ./internal/mcp/...`.

## Verification

- A newly registered tool has a `mcp_tool_invocations_total` series with
  `cache_status="SUCCESS"` and value `0` before its first call.
- A newly registered tool has a zero-count `mcp_tool_duration_seconds` series
  before its first call.
- Existing middleware increments and error labeling remain unchanged.
- The complete Go test suite and scoped lint pass.

## Open Questions

None.
