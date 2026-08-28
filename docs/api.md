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

## External Plugin Registry

Plugin and binding resources use the admin-only Kubernetes-style control-plane
routes. Authentication follows the tool registry rules above.

| Resource | Apply/List | Delete |
| :--- | :--- | :--- |
| Plugin | `POST`/`GET /apis/erpbridge.io/v1/plugins` | `DELETE .../plugins?name=<name>&version=<version>` |
| PluginBinding | `POST`/`GET /apis/erpbridge.io/v1/pluginbindings` | `DELETE .../pluginbindings?name=<name>` |

Plugin deletion is soft by default. Use `hard=true` for permanent deletion. A
plugin with an active binding returns `409 Conflict` and cannot be hard-deleted.
Binding admission requires an active exact plugin version and active exact MCP
tool version. A `raw_response` binding additionally requires configured
`API_AUTH_TOKEN`, an authenticated admin request, an allowlisted plugin
endpoint, an active HTTP-backed tool, and an explicit object-shaped output
schema. Missing raw prerequisites prevent activation during reconciliation.
Use `name` and `version` filters for plugins, and `name`, `pluginName`,
`pluginVersion`, `toolName`, or `toolVersion` filters for bindings. Bound
response processing runs only on cache misses; the cache stores the final
transformed MCP result and never stores an error result. Plugin and binding
lifecycle changes flush affected tool cache entries.

### Plugin HTTP protocol

For an active binding, ERPBridge sends a synchronous JSON request to
`<spec.endpoint>/v1/process`. An `after_response` binding receives the
normalized result:

```json
{
  "protocolVersion": "v1",
  "invocationId": "generated-id",
  "tool": {"name": "list_employees", "version": "1.0.0"},
  "result": {"employees": []},
  "config": {"mode": "safe"}
}
```

A `raw_response` binding receives a bounded response before normalization:

```json
{
  "protocolVersion": "v1",
  "invocationId": "generated-id",
  "tool": {"name": "read-invoice-text", "version": "1.0.0"},
  "rawResponse": {
    "status": 200,
    "contentType": "image/png",
    "body": {"encoding": "base64", "value": "..."}
  },
  "config": {"mode": "ocr"}
}
```

`encoding` is `json` for one complete decoded JSON document. Empty, malformed,
and non-JSON bodies use `base64`. Raw invocations omit `result`; legacy
`after_response` invocations retain `result: null` when the result is nil.
The plugin must return `{"result": <JSON value>}`. The plugin can replace only
the body; status remains immutable. Request and response JSON are limited to
1 MiB. Redirects and retries are disabled. A timeout, non-2xx response,
malformed or oversized response, or transformed output that fails the tool
schema follows the binding policy. `continue` uses the original captured
response only when it satisfies the final schema; otherwise it returns a safe
error. Use `fail` for image conversion unless a compatible fallback is known.
Plugin URLs, payloads, credentials, and plugin error bodies are not returned to
callers.

The protocol does not include original arguments, inbound headers, caller
identity, caller tokens, or ERP credentials.


### Admission Rules

The server rejects tool definitions when:

- The tool name starts with `get-` or `post-`.
- The endpoint path contains embedded secrets (for example `token` or `key=`).
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
Authenticated direct calls share a limiter bucket by token principal.

Excess direct calls return HTTP `429` with the stable `RATE_LIMITED` error and a
whole-second `Retry-After` header. Streamable HTTP MCP `tools/call` requests
remain HTTP `200` responses with `result.isError=true`; `tools/list` is not
throttled.

This endpoint is admin-only when HTTP authentication is enabled. For a guarded
tool, pass the verified role in `X-ERPBridge-Role`; do not put `role` in the
JSON body. The header value must be present in both the caller identity and the
tool's `allowedRoles` list. A missing or invalid selector returns `403` and a
body/header collision returns `400`.

This endpoint resolves registered tools only. MCP built-ins such as `system.progress_test` are available through MCP `tools/call`, but not through this REST endpoint.

The REST endpoint returns the legacy `ToolResult` compatibility shape. MCP
clients receive the MCP result envelope, including `content`,
`structuredContent` when the developer declares an object-shaped output schema,
and `isError: true` for tool execution errors. A successful image-to-text tool
can declare `{text: string}`: MCP receives structured `{text: ...}` plus
equivalent text content containing only the extracted text. SDK clients must
preserve that envelope and must not flatten structured content.

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

OpenAPI-generated tools preserve path, query, header, and JSON body
parameters through `execution.parameterLocations`; see the [tool schema
reference](./tool-schema.md) for the legacy fallback and protected-header
rules. Generated response unwrapping is used only when the resolved top-level
response schema proves a `data` property.

`/api/logs/stream` flushes its `200` response headers before the first event.
Each event uses `data: <JSON>\n\n` framing. Closing the client request stops
subscription. Subscribers use a bounded 100-event channel; new events are
dropped for a slow subscriber when that channel is full. The raw stream is
separate from the Console BFF, which applies its own projection and redaction.

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
| `/api/info` | `GET` | Authenticated safe build and runtime metadata. |

`/api/info` returns the server version, optional commit and build date, cache
backend label, active tool count, and observation time. It does not return
credentials or ERP configuration.

MCP tool invocation and duration series are initialized when each tool is
registered, so their metric families and zero-valued samples are available on
cold-start scrapes before the first tool call. Error-status invocation series
are added when an error is observed.

## Error Responses

The server uses standard HTTP status codes. Control-plane failures return a
bounded JSON object with `error`, `message`, `suggestion`, and numeric `code`.
The `error` value is stable for automation. Common values include
`VALIDATION_FAILED`, `AUTHENTICATION_FAILED`, `AUTHORIZATION_DENIED`,
`RESOURCE_NOT_FOUND`, `UPSTREAM_UNREACHABLE`, `RATE_LIMITED`, and
`HEALTH_CHECK_FAILED`.
Messages and suggestions never include upstream bodies, credentials, auth
headers, or internal stack details.

A configured Redis backend remains the selected backend when Redis is
unreachable; the server does not silently fall back to memory in that case.
MCP tool execution errors retain the MCP result envelope and set `isError: true`.

### API probe

`POST /api/apis/test` is an authenticated administrator endpoint. It accepts
an API URL, method, authentication type, non-secret `credentialRef`, optional
`credentialSource` (`env` or `file`), and optional auth-header name. Omitted
source means `env`. The server resolves the credential in ERPBridge and returns
only HTTP status, normalized content type, latency, and success. File mode
requires `ERPBRIDGE_CREDENTIALS_DIR`, reads the reference-named file on every
probe, and fails closed without an environment fallback. It never returns an
ERP response body or upstream headers. Use `bridgectl api test` to invoke this
endpoint; use `--local` only for the explicit host-side diagnostic, which
requires the CLI process to have the same mounted directory.
