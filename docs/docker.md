# Docker Deployment Guide

This guide covers how to deploy and manage the ERPBridge ecosystem with Docker and Docker Compose.

## 1. Quick Start

Make sure that you have [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) installed.

```bash
# Clone the repository
git clone https://github.com/nmdra/ERPBridge.git
cd ERPBridge

# Start the local development stack safely
make dev-up

# Direct Compose use requires MOCK_ERP_CREDENTIALS_JSON or
# MOCK_ERP_CREDENTIALS_FILE to be set first.
docker compose up --build --force-recreate -d
```

The stack includes:

- **ERPBridge Server** (`:8080`): The core MCP middleware.
- **Mock ERP** (`:8081`): Simulates legacy ERP endpoints.
- **Redis** (`:6379`): Provides the exact-match cache.

## 2. Configuration

Environment variables for the server are set in the `docker-compose.yml` file.

| Variable | Description | Default (compose) |
| :--- | :--- | :--- |
| `BASE_URL` | Public URL of the MCP server. | `http://localhost:8080` |
| `ERPBRIDGE_HOST_PORT` | Host port for the ERPBridge container. | `8080` |
| `ERP_BASE_URL` | Base URL of the underlying ERP system. | `http://mock-erp:8081` |
| `MOCK_ERP_HOST_PORT` | Host port for the MockERP container. | `8081` |
| `MOCK_ERP_IMAGE` | MockERP image used by Compose. | `ghcr.io/nmdra/mockerp:0.2.1` |
| `MOCK_ERP_VERSION` | MockERP release used for OpenAPI generation. | `0.2.1` |
| `MOCK_ERP_OPENAPI_URL` | Versioned MockERP OpenAPI URL. | `https://raw.githubusercontent.com/nmdra/mockerp/v0.2.1/openapi.yaml` |
| `MOCK_ERP_DB_PATH` | SQLite database path inside MockERP. | `/data/mockerp.db` |
| `MOCK_ERP_CREDENTIALS_JSON` | JSON credential configuration for local/container use. | (required) |
| `MOCK_ERP_CREDENTIALS_FILE` | Mounted JSON credential file path. | (required alternative) |
| `REDIS_URL` | URL for the Redis cache. | `redis://redis:6379` |
| `REDIS_HOST_PORT` | Host port for Redis. | `6379` |
| `REDIS_INSIGHT_BIND_ADDRESS` | Address bound by RedisInsight. | `127.0.0.1` |
| `REDIS_INSIGHT_HOST_PORT` | Host port for RedisInsight. | `8001` |
| `DATABASE_PATH` | Path of the SQLite tool registry inside the container. | `/app/data/erpbridge.db` |
| `RATE_LIMIT_RPS` | Per-session requests per second. | `10` |
| `RATE_LIMIT_BURST` | Token bucket burst size. | `20` |
| `INSECURE_AUTH_ALLOWED_HOSTS` | Development-only exact `host:port` exceptions for credentialed HTTP calls. | `mock-erp:8081` |
| `API_AUTH_TOKEN` | Bearer token for protected admin, MCP, and direct-invoke routes. | unset |
| `PLUGIN_ENDPOINT_ALLOWLIST` | Exact `host:port` values allowed for credentialed plugin resources. | unset |
| `PLUGIN_MOCK_API_KEY` | Environment-backed API key reference used by the plugin integration fixture. | unset |
| `MOCK_PLUGIN_API_KEY` | API key accepted by the separately deployed mock plugin fixture. | unset |

For the full list of server environment variables, see the [Environment Variables Reference](./environment-variables.md).

MockERP fails closed when neither `MOCK_ERP_CREDENTIALS_JSON` nor
`MOCK_ERP_CREDENTIALS_FILE` is configured. `make dev-up` generates an ephemeral
JSON credential pair only when neither source is set. It validates the rendered
Compose configuration, force recreates the stack, and polls both service health
endpoints without writing or printing the generated values. Use the JSON
variable for local development, or mount a Docker secret and set the file path.
Do not commit credential values to this repository.

The bootstrap does not source `.env`; Compose reads that file and preserves its
quoted values. Direct `docker compose` commands do not run the bootstrap
preflight. Set a credential source first, then run `docker compose config
--quiet` and `docker compose up --build --force-recreate -d`. RedisInsight binds
to `127.0.0.1` by default. Change `REDIS_INSIGHT_BIND_ADDRESS` only when you
explicitly need another interface.

Credentialed ERP and plugin endpoints must use HTTPS. The bundled MockERP
fixture is HTTP-only, so Compose explicitly sets
`INSECURE_AUTH_ALLOWED_HOSTS=mock-erp:8081`. The opt-in plugin integration
fixture adds `mock-plugin:8080` to this exact host-and-port list and sets
`PLUGIN_ENDPOINT_ALLOWLIST=mock-plugin:8080`. These exceptions are for local
development only. Do not set a broad or production allowlist.

## 3. Tool Registry

The server keeps tool definitions in a SQLite database (`DATABASE_PATH`). The `schemas/` directory is NOT mounted into the container. `schemas/` is also not tracked by git. There is no file-system watcher.

To load tools into the registry, generate and apply the schemas from your host:

1. Register the ERP API:

   ```bash
   ./bridgectl api register --name erp --url http://localhost:8081 --module erp --description "Mock ERP"
   ```

2. Fetch the pinned OpenAPI contract, generate, and apply the tool schemas:

   ```bash
   make generate-tools
   ```

The server detects new registry entries within 10 seconds and exposes them over MCP. A restart is not necessary.

## 4. Using bridgectl with Docker

You can use the local `bridgectl` binary to interact with the server running in Docker.

1. **Build bridgectl:**

    ```bash
    go build -o bridgectl tools/bridgectl/main.go
    ```

2. **Verify Connection:**

    ```bash
    ./bridgectl tool get
    ```

3. **Generate a new tool:**

    ```bash
    MOCK_ERP_VERSION=0.2.1 make generate-tools
    ```

    The target fetches the matching tagged OpenAPI contract and applies the generated YAML.

4. **Test an API through the server:**

    ```bash
    ./bridgectl api test erp
    ```

    This is the default mode. ERPBridge resolves the API `credentialRef` from
    its environment and returns only a status summary. Use
    `./bridgectl api test erp --local` only for a legacy host-side diagnostic.

    CLI control-plane requests use the configured MCP host root. The exact
    `/mcp` or `/mcp/` suffix is removed automatically. Other non-empty paths
    fail with `CONTROL_PLANE_URL_INVALID`. `/mcp/` remains the MCP client
    transport endpoint and must not be used as a REST path.

## 5. Logs & Monitoring

- **View Container Logs:**

  ```bash
  docker compose logs -f erpbridge-server
  ```

- **Live Stream Logs via CLI:**

  ```bash
  ./bridgectl log tail
  ```

- **Metrics:**
  Prometheus metrics are available at `http://localhost:8080/metrics`.

When the container receives `SIGTERM` or `SIGINT`, ERPBridge stops accepting new
HTTP connections and gracefully shuts down the listener before the process exits.

## 6. External plugins

ERPBridge does not install, start, update, or schedule plugin code. Deploy each
plugin process separately, then apply a `Plugin` resource with its reachable
endpoint and an exact-version `PluginBinding`. See the [External Plugin
Resource Schema](./plugin-schema.md) for the manifest and `/v1/process` JSON
contract.

For a deployment that uses a published plugin image, pin the image tag in the
operator-owned deployment configuration:

```yaml
services:
  response-transformer:
    image: ghcr.io/nmdra/erpbridge-plugins/mock-plugin:0.1.0
```

The image is not part of the ERPBridge server image. The plugin receives only a
normalized result and binding configuration; it does not receive ERP
credentials, inbound request headers, or caller identity. A credentialed
plugin resource stores an environment-variable reference, not the key value:

```yaml
metadata:
  type: docker
spec:
  endpoint: http://mock-plugin:8080
  auth:
    type: api-key
    credentialRef: PLUGIN_MOCK_API_KEY
    header: X-API-Key
```

The server resolves the reference at invocation time and sends one API-key
header. The control plane must have `API_AUTH_TOKEN` enabled, and the endpoint
must match `PLUGIN_ENDPOINT_ALLOWLIST` exactly.

The repository includes an opt-in black-box test. It builds the deterministic
fixture from the sibling `../ERPBridge-Plugins` polyrepo, starts it with the
pinned MockERP image, and removes only its isolated containers and volumes:

```bash
make test-plugin-integration
```

The test uses the Compose project name `erpbridge-plugin-test` and ports
`18080`, `18081`, `18090`, and `16379` so it does not reuse the normal local
stack. It generates separate admin, ERP, and plugin credentials at runtime,
passes them only to the services that need them, and checks missing, wrong, and
correct plugin API keys at `/v1/process`. The plugin `/health` endpoint remains
unprotected for readiness checks. Do not run it when those ports are already
in use.

## 7. Connecting MCP Clients

ERPBridge supports the **Stdio** and **Streamable HTTP** transports.

### Claude Desktop (Stdio)

Claude Desktop connects to MCP servers via standard input and output. The server binary supports the `--stdio` flag.

1. **Locate Configuration:**
    - **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
    - **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

2. **Add ERPBridge Server:**
    Add the following to the `mcpServers` section. This runs the server in stdio mode inside a container.

    ```json
    {
      "mcpServers": {
        "erpbridge": {
          "command": "docker",
          "args": [
            "run",
            "-i",
            "--rm",
            "ghcr.io/nmdra/erpbridge-server:latest",
            "--stdio"
          ]
        }
      }
    }
    ```

    *Note: The tool registry lives in the SQLite database. Tools persist inside the container only while it runs. To keep tools across restarts, run the full `docker compose` stack and connect via Streamable HTTP instead.*

3. **Restart Claude:** Fully quit and restart Claude Desktop. Look for the tool icon in the chat input.

### Cursor (Streamable HTTP)

Cursor connects to remote MCP servers via HTTP. Use this method when the ERPBridge stack is already running via `docker compose up`.

1. **Make Sure the Server Is Running:**
    Verify that the stack is up and the server is reachable at `http://localhost:8080`.

2. **Configure Cursor:**
    - Open Cursor **Settings** (`Cmd+,` or `Ctrl+,`).
    - Navigate to **Features** > **MCP**.
    - Click **+ Add New MCP Server**.
    - **Name:** `ERPBridge`
    - **Type:** `streamable-http` (or `http`, depending on your Cursor version)
    - **URL:** `http://localhost:8080/mcp/`

3. **Verify:**
    You see a green status indicator. You can now use the ERP tools in Cursor Chat or Composer.

## 8. Troubleshooting

- **Connection Refused:** Make sure that `ERP_BASE_URL` in `docker-compose.yml` uses the service name `http://mock-erp:8081` instead of `localhost`. Compose pulls `ghcr.io/nmdra/mockerp:0.2.1`; override `MOCK_ERP_IMAGE` only with a compatible image.
- **Claude Stdio Timeout:** If Claude fails to connect, build the server binary first and run it directly. This shows any startup errors.
- **Schema Errors:** Validate the tool definition locally before you apply it:

  ```bash
  ./bridgectl tool validate -f schemas/erp/list_employees.json
  ```
