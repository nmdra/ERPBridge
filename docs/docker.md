# Docker Deployment Guide

This guide covers how to deploy and manage the ERPBridge ecosystem with Docker and Docker Compose.

## 1. Quick Start

Make sure that you have [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) installed.

```bash
# Clone the repository
git clone https://github.com/nmdra/ERPBridge.git
cd ERPBridge

# Start the full stack
docker compose up -d --build
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
| `ERP_BASE_URL` | Base URL of the underlying ERP system. | `http://mock-erp:8081` |
| `MOCK_ERP_IMAGE` | MockERP image used by Compose. | `ghcr.io/nmdra/mockerp:0.1.1` |
| `MOCK_ERP_VERSION` | MockERP release used for OpenAPI generation. | `0.1.1` |
| `MOCK_ERP_OPENAPI_URL` | Versioned MockERP OpenAPI URL. | `https://raw.githubusercontent.com/nmdra/mockerp/v0.1.1/openapi.yaml` |
| `REDIS_URL` | URL for the Redis cache. | `redis://redis:6379` |
| `DATABASE_PATH` | Path of the SQLite tool registry inside the container. | `/app/data/erpbridge.db` |
| `RATE_LIMIT_RPS` | Per-session requests per second. | `10` |
| `RATE_LIMIT_BURST` | Token bucket burst size. | `20` |

For the full list of server environment variables, see the [Environment Variables Reference](./environment-variables.md).

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
    MOCK_ERP_VERSION=0.1.1 make generate-tools
    ```

    The target fetches the matching tagged OpenAPI contract and applies the generated YAML.

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

## 6. Connecting MCP Clients

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

## 7. Troubleshooting

- **Connection Refused:** Make sure that `ERP_BASE_URL` in `docker-compose.yml` uses the service name `http://mock-erp:8081` instead of `localhost`. Compose pulls `ghcr.io/nmdra/mockerp:0.1.1`; override `MOCK_ERP_IMAGE` only with a compatible image.
- **Claude Stdio Timeout:** If Claude fails to connect, build the server binary first and run it directly. This shows any startup errors.
- **Schema Errors:** Validate the tool definition locally before you apply it:

  ```bash
  ./bridgectl tool validate -f schemas/erp/list_employees.json
  ```
