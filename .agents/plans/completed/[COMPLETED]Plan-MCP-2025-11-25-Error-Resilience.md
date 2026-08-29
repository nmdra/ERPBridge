# Plan: MCP 2025-11-25 error and resilience alignment

## Goal

Update the existing ERPBridge MCP server so protocol failures use JSON-RPC
errors while valid tool calls return sanitized `CallToolResult` execution
errors. Improve bounded downstream retries, rate-limit metadata, optional
per-tool limits, and bounded concurrency without rewriting the current
registration, middleware, connector, plugin, authentication, or logging
architecture.

## Current State

- `mcp-go v0.57.0` is pinned in `go.mod:12` and supports MCP protocol
  `2025-11-25`.
- `internal/mcp/server.go:681-764` delegates Streamable HTTP parsing and
  dispatch to `mcp-go`, but buffers every POST through an interception writer,
  preventing streaming-capable response handling.
- `internal/mcp/server.go:628-648` resolves tools and converts execution errors
  to `CallToolResult.isError`; `mcp-go` owns protocol errors in its
  `server/request_handler.go` and `server/server.go`.
- `internal/mcp/server.go:86-98` enables output-schema validation but not
  input-schema validation or panic recovery.
- `internal/mcp/middleware.go:121-148` has principal/session token-bucket
  enforcement; direct REST receives HTTP 429 at `server.go:1069-1074`, while
  MCP calls receive an error result without metadata.
- `internal/connector/client.go:203-267` retries network errors, 429, and 5xx
  three times with fixed delay/jitter, but does not honor `Retry-After` and
  replays POST/PATCH/DELETE without an idempotency guarantee.
- `internal/mcp/tool.go:328-367` decodes and returns downstream error bodies as
  tool result data, which can expose raw upstream content.
- `internal/mcp/plugin_pipeline.go:63-146` runs native/raw/after-response tool
  execution; raw plugins intentionally receive bounded raw responses.
- `internal/mcp/tool.go:72-78` exposes MCP behavior hints, but no idempotency
  storage or request-key mechanism exists.
- `internal/metrics/metrics.go:19-88` already provides Prometheus metrics for
  ERP calls, tool calls, latency, cache, credentials, server lifecycle, and
  sessions. `middleware.go:191-209` records only success/error tool status.
- Existing logging redacts arguments and credentials in
  `internal/mcp/middleware.go:151-188`, `internal/logger/redact.go`, and the
  connector logs endpoint identity/status without response bodies.

## Decisions

- Keep `mcp-go` responsible for JSON-RPC parsing, method dispatch, session
  handling, and standard protocol error codes. Enable its existing input-schema
  validation option so schema/business argument failures are tool results.
- Treat structural MCP dispatch failures (malformed JSON, unsupported method,
  invalid JSON-RPC request, and unknown tool) as SDK-owned protocol errors.
  Treat post-dispatch tool validation, business, permission, dependency, and
  rate-limit failures as sanitized `isError=true` results.
- Add a small shared `internal/faults` taxonomy carrying safe message, kind,
  retryability, retry delay, and an unexported cause. Never serialize the cause.
- Use the namespaced `_meta` key `com.erpbridge/error`; it is application
  metadata, not a claim about MCP-standard fields.
- Retry GET/HEAD/OPTIONS automatically. Do not retry POST/PATCH/DELETE unless
  an explicit idempotency mechanism is later added; this prevents duplicate
  side effects with the current architecture. Honor bounded `Retry-After` for
  retryable downstream responses.
- Add optional per-tool rate and concurrency configuration. Preserve the
  existing global principal/session limiter as the outer protection and leave
  unconfigured tools at current successful behavior.
- Remove the redundant POST response interception wrapper; `mcp-go` already
  filters tools through the configured tool filter and must retain Streamable
  HTTP streaming behavior.
- Preserve raw-response plugin input as the explicit bounded exception, but
  ensure the final MCP error for a failed downstream response is sanitized.

## Scope

In scope: MCP error classification, input validation enablement, safe execution
error results and metadata, panic sanitization, downstream error classification
and retry policy, Retry-After handling, optional per-tool rate/concurrency
limits, metrics/log distinctions, focused tests, local/public docs, and
changelog updates.

Out of scope: MCP protocol versions after 2025-11-25, new HTTP headers such as
`Mcp-Method` or `Mcp-Name`, OAuth, a new observability framework, blind
side-effect retries, persistent idempotency storage, changes to successful tool
schemas/results, and unrelated dirty-tree skill changes.

## Tasks

- [x] Task 1: Add shared invocation fault taxonomy and sanitized MCP result
  mapping. (**Seam:** `handleMCPToolCall` and `CallToolResult`; **Files:**
  `internal/faults/errors.go`, `internal/mcp/errors.go`,
  `internal/mcp/server.go`, `internal/mcp/plugin_pipeline.go`;
  **Verify:** focused MCP error tests)
- [x] Task 2: Enable SDK input validation and panic recovery, and remove the
  response-buffering wrapper while retaining tool filtering. (**Seam:**
  Streamable HTTP and `MCPServer.HandleMessage`; **Files:**
  `internal/mcp/server.go`, `internal/mcp/api_test.go`,
  `internal/mcp/server_test.go`;
  **Verify:** protocol error and tool execution tests)
- [x] Task 3: Classify downstream responses, bound retries to replay-safe
  methods, honor bounded `Retry-After`, and preserve safe error details.
  (**Seam:** `connector.Client.CallWithOptions` and `Tool.Execute`; **Files:**
  `internal/connector/client.go`, `internal/connector/resilience_test.go`,
  `internal/mcp/tool.go`, `internal/mcp/tool_test.go`;
  **Verify:** connector and tool tests)
- [x] Task 4: Add optional per-tool rate limits and bounded principal-scoped
  concurrency at the existing middleware seam. (**Seam:** tool middleware
  chain; **Files:** `internal/mcp/tool.go`, `internal/mcp/server.go`,
  `internal/mcp/middleware.go`, `internal/mcp/middleware_test.go`,
  `internal/mcp/server_test.go`;
  **Verify:** independent tool limits and concurrency tests)
- [x] Task 5: Add outcome/error metrics and preserve redacted structured logs.
  (**Seam:** tool/connector middleware; **Files:** `internal/metrics/metrics.go`,
  `internal/mcp/middleware.go`, `internal/connector/client.go`, related tests;
  **Verify:** metrics tests and focused package tests)
- [x] Task 6: Synchronize implementation docs, public docs, and changelogs.
  (**Seam:** documented MCP/error/resilience contract; **Files:**
  `docs/connectivity.md`, `docs/api.md`, `docs/environment-variables.md`,
  `CHANGELOG.md`, `/home/nimendra/Documents/Projects/erpbridge-docs/docs/erpbridge/connectivity.mdx`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/CHANGELOG.md`;
  **Verify:** documentation builds and diff checks)
- [x] Task 7: Run full verification, review assumptions, and close this plan.
  (**Seam:** repository quality gates; **Files:** plan indexes and this plan;
  **Verify:** `make test`, relevant lint/type/build checks, diagnostics, and
  plan archived with `[COMPLETED]` prefix)

## Verification

- Focused protocol tests for parse error, unsupported method, invalid request
  parameters, valid tool execution failure, sanitized internal failure, and
  namespaced retry metadata.
- Focused connector tests for bounded transient retries, no unsafe replay,
  Retry-After seconds/date handling, context deadline, and final 429 mapping.
- Focused concurrency/rate tests for principal isolation, per-tool isolation,
  capacity rejection, and release after failure.
- `go test ./internal/mcp ./internal/connector ./internal/metrics ./services/erpbridge-server`
- `make test`
- Repository lint/typecheck/build commands relevant to touched files.
- Public Docusaurus build and `git diff --check` in both repositories.
- `lens_diagnostics` delta and final full diagnostics.

## Open Questions

None. The request explicitly authorizes preserving the existing architecture and
avoiding blind retries for side-effecting tools.
