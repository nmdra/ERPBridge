# Connectivity & Transport Guide

ERPBridge supports multiple transport protocols. It works with modern AI agents, IDEs, and standard developer tools like Postman.

## 1. Streamable HTTP (Modern MCP)

This transport is the MCP streamable HTTP specification. It suits stateless or web-friendly environments. It is the recommended way to connect Postman and other modern MCP clients.

- **Base URL:** `http://localhost:8080/mcp/`
- **Handshake:** `POST /mcp/` with a JSON-RPC `initialize` request
- **Transport Specification:** MCP 2025-11-25. The server negotiates older supported protocol versions for compatible clients.
- **Features:**
    - Request and response via standard POST.
    - Session management via the `Mcp-Session-Id` header.
    - Browser CORS support for the `/mcp/` endpoint. Preflight requests may
      include `Content-Type`, `Mcp-Session-Id`, and `MCP-Protocol-Version`;
      responses expose `Mcp-Session-Id` so browser clients can maintain the
      stateful session.

Set `CORS_ALLOWED_ORIGINS` to a comma-separated list of browser origins. An
allowed preflight does not require a bearer token, but the following MCP
request does when `API_AUTH_TOKEN` is configured. In open mode, the server
keeps the existing wildcard CORS behavior.

The CORS policy applies to the MCP endpoint only. The management, direct
invoke, cache, logs, metrics, and health endpoints are not part of the
cross-origin browser contract.

When `API_AUTH_TOKEN` is configured, use `Authorization: Bearer <token>` for
the MCP request. API tokens require the `mcp` scope. The admin credential has
implicit access. A preflight is handled before bearer authentication so a
browser can complete its CORS negotiation without exposing credentials.

### Postman Configuration

- **Transport Type:** Streamable HTTP
- **URL:** `http://localhost:8080/mcp/`

## 2. Stdio (Local Integration)

Stdio is the preferred transport for local integrations. The client starts the ERPBridge server as a child process.

- **Best For:** Claude Desktop, Cursor, and other IDE-integrated agents running locally.
- **Usage:** Run the server with the `--stdio` flag.

```bash
erpbridge-server --stdio
```

The stdio stream reserves stdout for MCP JSON-RPC. ERPBridge writes its
startup banner and diagnostics to stderr in this mode, so agents can parse
stdout without a wrapper-specific preamble. HTTP bearer authentication does
not apply to stdio; the child process must receive any upstream ERP
credentials it needs through its environment.

See the [Agentic Tools MCP Integration Guide](./agent-integrations.md) for
Codex CLI, OpenCode, OpenClaw, and Hermes Agent configuration examples.

## 3. Direct API (Internal/CLI)

The server exposes direct HTTP endpoints for internal management, performance monitoring, and the `bridgectl` CLI. These do not require a full MCP handshake.

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/apis/erpbridge.io/v1/tools` | `GET/POST/DELETE` | Apply, list, and delete tool definitions (Control Plane). |
| `/api/tools/invoke` | `POST` | Directly invoke an MCP tool. |
| `/api/cache/stats` | `GET` | Retrieve tool cache performance metrics. |
| `/api/cache/flush` | `GET` | Flush specific or all cache entries. |
| `/api/logs/stream` | `GET` | Real-time structured log stream (SSE). |
| `/api/logs/recent` | `GET` | Fetch recent log history in JSON format. |
| `/api/auth/tokens` | `POST/GET` | Create and list API tokens. Requires the admin credential when authentication is enabled. |
| `/api/auth/tokens/{id}` | `DELETE` | Revoke an API token. Requires the admin credential when authentication is enabled. |

The registry, direct invoke, cache, and token endpoints are admin-only. Log
endpoints accept an API token with the `logs` scope, and `/metrics` accepts a
token with the `metrics` scope. `/mcp/health` is always open.

For the full endpoint reference, see the [REST API Reference](./api.md).

## 4. Protection & Limits

ERPBridge includes built-in protection for underlying ERP systems.

- **Rate Limiting:** Tool execution uses a token bucket. Authenticated HTTP
  requests are keyed by token principal; unauthenticated stateful MCP requests
  use their session, and stdio uses its process fallback.
- **Default Limits:** 10 requests per second with a burst of 20 (configurable
  via `RATE_LIMIT_RPS` and `RATE_LIMIT_BURST`).

Excess direct `/api/tools/invoke` calls return HTTP `429`, stable error
`RATE_LIMITED`, and a whole-second `Retry-After` value. Streamable HTTP MCP
`tools/call` requests remain HTTP `200` and carry `result.isError=true`.
`tools/list` is not subject to tool-execution throttling.

MCP protocol failures use JSON-RPC errors. Malformed JSON uses `-32700`,
invalid requests use `-32600`, unsupported methods use `-32601`, invalid
protocol parameters use `-32602`, and unexpected server failures use `-32603`.
Valid tool calls that fail during execution return `isError: true` instead.
These results may include the namespaced `com.erpbridge/error` metadata with a
safe error type, retryability, and a bounded `retryAfterMs` value.

ERP retries are bounded by three attempts and a 30-second overall deadline.
`Retry-After` is honored when supplied. Automatic retries are limited to
GET, HEAD, and OPTIONS so POST, PUT, PATCH, and DELETE operations are not
blindly replayed without an idempotency mechanism.

## 5. Monitoring & Health

Standard endpoints for system health and observability.

- **Health Check:** `GET /mcp/health` (Returns `{"status": "ok"}`)
- **Metrics:** `GET /metrics` (Prometheus formatted metrics)

Authenticated clients can open `/api/logs/stream` for SSE events. The server
flushes headers before the first event and frames each event as
`data: <JSON>\n\n`. The raw stream is distinct from the redacted Console BFF
projection. A slow subscriber uses a bounded 100-event channel; new events are
dropped when it is full.

## Summary Table

| Client Type | Recommended Transport | Base URL / Method |
| :--- | :--- | :--- |
| **Postman / Web** | Streamable HTTP | `http://localhost:8080/mcp/` |
| **Claude / Cursor** | Stdio | `erpbridge-server --stdio` |
| **bridgectl / Scripts** | Direct API | `http://localhost:8080/api/` |
| **Prometheus** | HTTP | `http://localhost:8080/metrics` |
