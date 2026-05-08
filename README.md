# ERP AI Middleware

Middleware for bridging legacy ERP systems with Agentic AI using the Model Context Protocol (MCP).

## Architecture

- **mock-erp/**: Temporary Python FastAPI service simulating legacy ERP modules (Finance, HR, Inventory).
- **services/erpbridge-server/**: Go-based MCP Server (HTTP/Stdio) + ERP Connector.
- **tools/bridgectl/**: Go CLI for developers and AI agents to manage APIs and tools.
- **internal/**: Shared Go libraries for configuration, protocol handling, and I/O.

## Packages

| Package | Type | Binary Name | Description |
| :--- | :--- | :--- | :--- |
| **ERPBridge Server** | Service | `erpbridge-server` | The core MCP Server. Handles ERP connections, resilience, and semantic caching. |
| **bridgectl** | CLI | `bridgectl` | Developer tool for environment management, schema validation, and real-time monitoring. |

## Key Differences

| Feature | ERPBridge Server | bridgectl |
| :--- | :--- | :--- |
| **Primary Role** | Runtime execution and protocol bridging. | Development, debugging, and management. |
| **Connectivity** | Connects to Redis and Legacy ERP APIs. | Connects to ERPBridge Server API. |
| **Lifecycle** | Long-running daemon (Docker/Kubernetes). | Short-lived command execution. |
| **Interface** | HTTP (for agents) / Stdio. | Standard Output (Table/JSON/YAML). |
| **State** | Manages semantic cache and circuit breakers. | Stateless; reads configuration from `~/.erpbridge.yaml`. |

## Stack
- **Go**: 1.26.2
- **Python**: 3.11+
- **MCP Library**: [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go)
- **Database**: Redis (for Semantic Caching)
- **Monitoring**: Prometheus (Metrics)
- **Resilience**: Sony/GoBreaker & Avast/Retry-Go

## Getting Started

### Prerequisites
- Go 1.26.2+
- Python 3.11+
- Docker & Docker Compose
- Redis (with RediSearch module)

### Running the Full Stack
```bash
docker compose up -d --build
```

### Manual Build & Run
1. **Mock ERP**:
   ```bash
   cd mock-erp
   uv run main.py
   ```

2. **ERPBridge Server**:
   ```bash
   go run services/erpbridge-server/main.go
   ```

3. **bridgectl**:
   ```bash
   go build -o bridgectl tools/bridgectl/main.go
   ./bridgectl --help
   ```

## Documentation

Explore our comprehensive documentation in the [docs/](./docs) directory:

- **[Documentation Wiki](./docs/README.md)**: Central hub for all guides.
- **[Docker Deployment Guide](./docs/docker.md)**: Setup and manage ERPBridge with Docker.
- **[Connectivity & Transport Guide](./docs/connectivity.md)**: Streamable HTTP, Stdio, and Postman setup.
- **[MCP Client Guide (Python & TypeScript)](./docs/mcp-client-guide.md)**: Build MCP clients for HTTP or Stdio.
- **[CLI Reference](./docs/cli/bridgectl.md)**: Detailed `bridgectl` command documentation.
- **[AI Agent Integration](./AGENTS.md)**: Patterns for Claude, Cursor, and more.

## Getting Started

### Prerequisites
- Go 1.26.2+
- Python 3.11+
- Docker & Docker Compose

### Running with Docker (Recommended)
The fastest way to get started is using Docker Compose. This starts the middleware, the mock ERP, Redis, and the embedding service.

```bash
docker compose up -d --build
```
See the [Docker Deployment Guide](./docs/docker.md) for detailed configuration.
