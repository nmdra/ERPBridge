# Environment Variables Reference

This page lists all environment variables read by the ERPBridge server, the `bridgectl` CLI, and the mock ERP service.

## Server Variables

The server reads these variables directly from the environment. It does not load a `.env` file. Export the variables in your shell, or set them in `docker-compose.yml`.

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `MCP_PORT` | `8080` | HTTP listen port. |
| `MCP_TRANSPORT` | (unset) | `stdio` runs the server in stdio mode. Any other value runs the HTTP server. |
| `DATABASE_PATH` | `data/erpbridge.db` | Path of the SQLite tool registry. The parent directory is created automatically. |
| `REDIS_URL` | (empty) | Redis URL (for example `redis://localhost:6379`). If empty, the server uses the bounded in-memory cache. |
| `CACHE_MEMORY_MAX_ENTRIES` | `10000` | Maximum number of entries in the in-memory cache when `REDIS_URL` is empty. `0` disables memory-cache storage. Invalid or negative values use the default. |
| `RATE_LIMIT_RPS` | `5.0` | Per-session requests per second (token bucket). |
| `RATE_LIMIT_BURST` | `10` | Token bucket burst size. |
| `BASE_URL` | `http://localhost:<MCP_PORT>` | Public URL of the server. Used for log lines and telemetry only. |
| `ERP_BASE_URL` | `http://localhost:8081` | Base URL of the underlying ERP system. Used by tool execution. |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `LOG_LEVEL_<COMPONENT>` | (unset) | Per-component log level override, for example `LOG_LEVEL_MCP`, `LOG_LEVEL_CACHE`, `LOG_LEVEL_CONNECTOR`, `LOG_LEVEL_IDP`. |
| `APP_ENV` | (unset) | `production` uses JSON log output. Any other value uses text output. |
| `LOG_TO_STDERR` | (unset) | `true` writes logs to stderr. The server sets this automatically in stdio mode. |
| `ERP_PRIMARY_KEY` | (unset) | Credential referenced by `credentialRef` in tool schemas. Resolved at tool-call time. |
| `API_AUTH_TOKEN` | (unset) | Enables HTTP bearer authentication when non-empty. This is the admin credential and is never returned by the server. |
| `API_AUTH_ADMIN_ROLES` | (unset) | Comma-separated roles assigned to the admin identity. Roles must match `[a-z][a-z0-9_-]{0,63}` and the list may contain at most 32 unique roles. |
| `CORS_ALLOWED_ORIGINS` | wildcard in open mode; disabled in auth mode when unset | Comma-separated browser origins allowed for the MCP endpoint. CORS does not apply to management or metrics routes. |

## CLI Variables (`bridgectl`)

The CLI reads these variables to override the active context.

| Variable | Purpose |
| :--- | :--- |
| `BRIDGE_CONTEXT` | Overrides the active context name. |
| `BRIDGE_SERVER` | Base URL for the cache and log endpoints. |
| `BRIDGE_MCP_SERVER` | Base URL for the tool registry API (`/apis/erpbridge.io/v1/tools`). |
| `BRIDGE_ERP_BASE` | Parsed into the context. Not used by any command. |
| `BRIDGE_AUTH_TYPE` | Parsed into the context. Not used by any command. |
| `BRIDGE_API_KEY` | Parsed into the context. Not used by any command. |
| `BRIDGE_AUTH_HEADER` | Parsed into the context. Not used by any command. |
| `BRIDGE_TOKEN` | Parsed into the context. Not used by any command. |
| `BRIDGE_USERNAME` | Parsed into the context. Not used by any command. |
| `BRIDGE_PASSWORD` | Parsed into the context. Not used by any command. |
| `BRIDGE_API_TOKEN` | Overrides the active context `api-token` for bridge HTTP requests. |

The persistent `bridgectl --token` flag has higher precedence than
`BRIDGE_API_TOKEN`, which has higher precedence than the active context's
`api-token` value. The CLI sends this value as a bearer token to the ERPBridge
server; it does not change upstream ERP authentication settings.

An active context can store the token in `~/.bridgectl/config.yaml`:

```yaml
contexts:
  local:
    server: http://localhost:8080
    api-token: erpbt_...
```

The CLI reads its defaults from `~/.bridgectl/config.yaml`. The default context uses:

| Key | Default |
| :--- | :--- |
| `server` | `http://localhost:8082` |
| `mcp-server` | `http://localhost:8080` |
| `erp-base` | `http://localhost:8081` |

> **Note:** Nothing listens on port `8082` by default. Set `BRIDGE_SERVER` or edit the config file before you use `bridgectl cache` or `bridgectl log`.

## Mock ERP Variables

| Variable | Purpose | Default |
| :--- | :--- | :--- |
| `MOCK_ERP_PORT` | Host port exposed for the MockERP container. | `8081` |
| `MOCK_ERP_IMAGE` | MockERP image used by Docker Compose. | `ghcr.io/nmdra/mockerp:0.2.1` |
| `MOCK_ERP_VERSION` | MockERP release used for OpenAPI generation. | `0.2.1` |
| `MOCK_ERP_OPENAPI_URL` | Versioned OpenAPI contract URL. | `https://raw.githubusercontent.com/nmdra/mockerp/v0.2.1/openapi.yaml` |
| `MOCK_ERP_DB_PATH` | SQLite database path inside MockERP. | `/data/mockerp.db` |
| `MOCK_ERP_CREDENTIALS_JSON` | JSON credential configuration for local/container use. Required when no secret file is set. | (required) |
| `MOCK_ERP_CREDENTIALS_FILE` | Path to a JSON credential configuration file, such as a mounted Docker secret. | (required alternative) |
| `MOCK_ERP_ENV` | Runtime environment used to gate destructive development commands. | `development` |
| `MOCK_ERP_ALLOW_RESET` | Enables the development-only database reset command when set to `true`. | `false` |
