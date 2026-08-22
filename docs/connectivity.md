# Connectivity & Transport Guide

ERPBridge supports multiple transport protocols. It works with modern AI agents, IDEs, and standard developer tools like Postman.

## 1. Streamable HTTP (Modern MCP)

This transport is the MCP streamable HTTP specification. It suits stateless or web-friendly environments. It is the recommended way to connect Postman and other modern MCP clients.

- **Base URL:** `http://localhost:8080/mcp/`
- **Handshake:** `POST /mcp/initialize`
- **Transport Specification:** MCP 2025-03-26. The server negotiates up to `2025-11-25` when the client supports it.
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

For the full endpoint reference, see the [REST API Reference](./api.md).

## 4. Protection & Limits

ERPBridge includes built-in protection for underlying ERP systems.

- **Rate Limiting:** Request throttling is enforced per-session (token bucket).
- **Default Limits:** 5 requests per second with a burst of 10 (configurable via environment variables).

## 5. Monitoring & Health

Standard endpoints for system health and observability.

- **Health Check:** `GET /mcp/health` (Returns `{"status": "ok"}`)
- **Metrics:** `GET /metrics` (Prometheus formatted metrics)

## Summary Table

| Client Type | Recommended Transport | Base URL / Method |
| :--- | :--- | :--- |
| **Postman / Web** | Streamable HTTP | `http://localhost:8080/mcp/` |
| **Claude / Cursor** | Stdio | `erpbridge-server --stdio` |
| **bridgectl / Scripts** | Direct API | `http://localhost:8080/api/` |
| **Prometheus** | HTTP | `http://localhost:8080/metrics` |
