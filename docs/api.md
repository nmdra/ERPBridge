# REST API Reference

The ERPBridge server exposes direct HTTP endpoints. They do not require an MCP handshake. They serve the `bridgectl` CLI, scripts, and monitoring tools.

## Authentication

HTTP authentication is enabled when `API_AUTH_TOKEN` is non-empty. Send the
configured admin credential or a bearer API token in every protected request:

```http
Authorization: Bearer <token>
```

The admin credential is accepted for every route. API tokens are limited by
scope: `mcp` for `/mcp/`, `metrics` for `/metrics`, and `logs` for the log
endpoints. The registry, direct invoke, cache, and token lifecycle endpoints
require the admin credential. `/mcp/health` remains open.

When authentication is disabled, HTTP routes keep their open-mode behavior.
This does not grant a caller a role: guarded tools still deny calls without a
verified identity. Stdio has the same fail-closed behavior for guarded tools.

## Base URL

`http://localhost:8080` (or the value of `MCP_PORT`)

## Tool Registry (Control Plane)

The registry API is Kubernetes-style. It stores tool definitions in SQLite.

### List Tools

```http
GET /apis/erpbridge.io/v1/tools
```

Returns a JSON array of tool definitions.

Use the optional exact-match query parameters `name` and `version` to request a specific tool version:

```http
GET /apis/erpbridge.io/v1/tools?name=list_employees&version=1.0.0
```

### Apply a Tool

```http
POST /apis/erpbridge.io/v1/tools
Content-Type: application/json
```

Body: one JSON tool definition (kind `MCPTool`). Returns `201 Created` on success. The `bridgectl tool apply` command also accepts the YAML sequence or multi-document YAML emitted by `bridgectl tool generate` and sends each tool definition separately.

### Delete a Tool

```http
DELETE /apis/erpbridge.io/v1/tools?name=<name>&version=<version>&hard=true
```

| Query param | Description |
| :--- | :--- |
| `name` | Tool name. Required. |
| `version` | Tool version. Required. |
| `hard` | `true` removes the row from SQLite. Omitted or `false` soft-deletes the tool. |

Returns `204 No Content` on success.

### Admission Rules

The server rejects tool definitions when:

- The tool name starts with `get-` or `post-`.
- The endpoint path contains embedded secrets (for example `token ` or `key=`).
- `spec.security.allowedRoles` contains an invalid, duplicate, empty, or more
  than 32 roles.
- A guarded tool defines its own `role` input property or requires that field.

## Tool Invocation

### Invoke a Tool

```http
POST /api/tools/invoke
Content-Type: application/json
```

Body:

```json
{
  "name": "list_employees",
  "arguments": {}
}
```

The call goes through the middleware chain: rate limiting, cache, and resilience.

This endpoint is admin-only when HTTP authentication is enabled. For a guarded
tool, pass the verified role in `X-ERPBridge-Role`; do not put `role` in the
JSON body. The header value must be present in both the caller identity and the
tool's `allowedRoles` list. A missing or invalid selector returns `403` and a
body/header collision returns `400`.

This endpoint resolves registered tools only. MCP built-ins such as `system.progress_test` are available through MCP `tools/call`, but not through this REST endpoint.

The REST endpoint returns the legacy `ToolResult` compatibility shape. MCP clients receive the MCP result envelope, including `content` and any structured result fields. SDK clients must preserve that envelope; the text content can contain JSON encoded by the ERPBridge compatibility handler and must not be flattened by assuming it is the whole response.

## Cache

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/api/cache/stats` | `GET` | Cache key counts and memory usage. Works with Redis and the bounded in-memory backend. |
| `/api/cache/flush` | `GET` | Flush cache entries. Query params: `tool`, `module`, `all=true`. A module flush covers all stored versions, including inactive versions. |

## Logs

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/api/logs/recent` | `GET` | JSON array of the last 1000 log entries. |
| `/api/logs/stream` | `GET` | Server-sent events stream of log entries. |

## MCP Endpoints

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/mcp/` | `POST` | MCP JSON-RPC requests (Streamable HTTP). |
| `/mcp/` | `GET` | SSE notification stream for the session. |
| `/mcp/health` | `GET` | Returns `{"status": "ok"}`. |

For a guarded MCP tool, select a role in the generated optional
`arguments.role` field. The server checks the role against the authenticated
token and the tool allow-list, then removes the selector before sending the
arguments to the ERP. Open tools do not reserve `role`, so existing business
payloads remain unchanged. Guarded read-only cache entries are shared by
verified roles; other guarded cache entries are role-scoped.

## Token Lifecycle

The admin-only token endpoints create, list, and revoke scoped API tokens. The
raw `erpbt_` token is returned only by creation; the server stores only its
SHA-256 hash.

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/api/auth/tokens` | `POST` | Create a token. |
| `/api/auth/tokens` | `GET` | List token metadata without token values or hashes. |
| `/api/auth/tokens/{id}` | `DELETE` | Revoke a token. |

## Observability

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/metrics` | `GET` | Prometheus-formatted metrics. |

MCP tool invocation and duration series are initialized when each tool is
registered, so their metric families and zero-valued samples are available on
cold-start scrapes before the first tool call. Error-status invocation series
are added when an error is observed.

## Error Responses

The server uses standard HTTP status codes. A configured Redis backend remains the selected backend when Redis is unreachable; the server does not silently fall back to memory in that case.
