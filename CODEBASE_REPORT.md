# ERPBridge — Comprehensive Codebase Report

> **Generated:** 2026-05-05 | **Last Updated:** 2026-05-06  
> **Repository:** [nmdra/ERPBridge](https://github.com/nmdra/ERPBridge)  
> **Module:** `github.com/nimendra/ERPBridge`  
> **Go Version:** 1.26.2 | **Python Version:** 3.11+

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Architecture Diagram](#2-architecture-diagram)
3. [Repository Structure](#3-repository-structure)
4. [Technology Stack](#4-technology-stack)
5. [Component Deep-Dives](#5-component-deep-dives)
   - 5.1 [Mock ERP (Python/FastAPI)](#51-mock-erp-pythonfastapi)
   - 5.2 [Middleware Service (Go/MCP)](#52-middleware-service-gomcp)
   - 5.3 [bridgectl CLI (Go/Cobra)](#53-bridgectl-cli-gocobra)
6. [Internal Packages Reference](#6-internal-packages-reference)
   - 6.1 [internal/mcp](#61-internalmcp)
   - 6.2 [internal/cache](#62-internalcache)
   - 6.3 [internal/connector](#63-internalconnector)
   - 6.4 [internal/config](#64-internalconfig)
   - 6.5 [internal/idp](#65-internalidp)
   - 6.6 [internal/logger](#66-internallogger)
   - 6.7 [internal/output](#67-internaloutput)
   - 6.8 [internal/cli](#68-internalcli)
7. [Data Flow](#7-data-flow)
8. [Semantic Caching System](#8-semantic-caching-system)
9. [Tool Schema System](#9-tool-schema-system)
10. [Authentication Model](#10-authentication-model)
11. [Configuration System](#11-configuration-system)
12. [API Endpoints Reference](#12-api-endpoints-reference)
13. [Docker & Deployment](#13-docker--deployment)
14. [Release Pipeline (GoReleaser)](#14-release-pipeline-goreleaser)
15. [CI/CD Workflow (GitHub Actions)](#15-cicd-workflow-github-actions)
16. [Testing](#16-testing)
17. [Key Design Patterns](#17-key-design-patterns)
18. [Known Limitations & Future Work](#18-known-limitations--future-work)

---

## 1. Project Overview

**ERPBridge** is an AI middleware layer that connects legacy ERP (Enterprise Resource Planning) systems to modern Agentic AI agents via the **Model Context Protocol (MCP)**. The project solves a fundamental interoperability problem: LLMs and autonomous AI agents cannot natively speak to business systems like SAP, Oracle, or custom-built ERP services. ERPBridge translates ERP REST API calls into standardized MCP "Tools" that any MCP-compatible AI client can discover and invoke.

### Core Value Proposition

| Problem | ERPBridge Solution |
|---|---|
| LLMs don't speak REST natively | Wraps ERP calls as typed MCP Tools |
| Each ERP has unique auth/schemas | Unified connector + JSON Schema tools |
| Repeated identical queries drain ERP | Two-layer Redis semantic cache |
| Debugging AI↔ERP interactions is hard | Real-time log streaming + `bridgectl` CLI |
| Adding new ERP endpoints requires coding | Auto-generate tool schemas from OpenAPI specs |

---

## 2. Architecture Diagram

### Mermaid Diagram

```mermaid
flowchart TB
    subgraph Clients["Client Layer"]
        direction LR
        Agent["🤖 AI Agent / LLM\n(Claude, GPT, MCP Client)"]
        CLI["🖥️ bridgectl CLI\n(Developer / Operator)"]
    end

    subgraph Middleware["Middleware Service · Go · :8080"]
        direction TB
        MCP["MCP Server\n(SSE + HTTP)"]
        TR["Tool Registry\n(JSON Schemas)"]
        CM["Cache Manager\nExact + Semantic"]
        EC["ERP Connector\n(HTTP Client)"]

        MCP -->|"register / lookup"| TR
        MCP -->|"cache read/write/flush"| CM
        MCP -->|"ERP call"| EC
    end

    subgraph CacheLayer["Cache Layer"]
        Redis["Redis Stack :6379\n+ RediSearch\n+ RedisInsight :8001"]
        Embedder["HuggingFace TEI :8083\nnomic-embed-text-v1\n768-dim vectors"]
        CM --> Redis
        CM -->|"embed args"| Embedder
    end

    subgraph ERP["Mock ERP · Python/FastAPI · :8081"]
        Finance["Finance\n/api/v1/finance\ninvoices · payments"]
        HR["HR\n/api/v1/hr\nemployees · departments"]
        Inventory["Inventory\n/api/v1/inventory\nitems · stock"]
    end

    Agent -- "MCP/SSE\nGET /mcp/sse\nPOST /mcp/messages" --> MCP
    CLI -- "REST\nPOST /api/tools/invoke\nGET /api/cache/*\nGET /api/logs/*" --> MCP
    EC -- "REST HTTP\nAPI-Key / Basic / Bearer" --> Finance
    EC --> HR
    EC --> Inventory
```

### Component Interaction (Sequence)

```mermaid
sequenceDiagram
    participant A as AI Agent
    participant M as Middleware (MCP Server)
    participant R as Redis Cache
    participant E as HF Embedder
    participant ERP as Mock ERP

    A->>M: GET /mcp/sse (SSE handshake)
    M-->>A: session_id + message endpoint

    A->>M: listTools
    M-->>A: [finance.list_invoices, ...]

    A->>M: callTool(finance.list_invoices, {})
    activate M

    M->>R: GET exact:finance.list_invoices:shared:empty
    R-->>M: MISS

    M->>E: POST /embed ("search_query: {}")
    E-->>M: float32[768]

    M->>R: FT.SEARCH idx:semantic KNN 1
    R-->>M: MISS (no similar entry)

    M->>ERP: GET /api/v1/finance/invoices (X-API-Key)
    ERP-->>M: {"data":[...],"total":2}

    M->>R: SET exact:... (TTL 300s)
    M->>R: HSET sem:{uuid} (embedding + response)

    M-->>A: CallToolResult{text: JSON}
    deactivate M
```

### Cache Decision Flow

```mermaid
flowchart LR
    Start([Tool Call]) --> ExactLookup{Exact\nCache Hit?}
    ExactLookup -- HIT --> ReturnCached([Return Cached\nX-Cache-Hit: exact])
    ExactLookup -- MISS --> SemanticEnabled{Semantic\nEnabled?}
    SemanticEnabled -- No --> ERPCall
    SemanticEnabled -- Yes --> Embed[Embed args\nHF TEI]
    Embed --> KNN[KNN-1 Search\nRediSearch]
    KNN --> Threshold{similarity >=\nthreshold?}
    Threshold -- Yes --> ReturnSemantic([Return Cached\nX-Cache-Hit: semantic])
    Threshold -- No --> ERPCall[Call ERP\nconnector.Call]
    ERPCall --> Validate{OutputSchema\ndefined?}
    Validate -- Yes --> ValidateResp[Validate Response\njsonschema]
    Validate -- No --> StoreCache
    ValidateResp --> StoreCache[Store in\nExact + Semantic Cache]
    StoreCache --> AutoFlush{FlushOn\nnon-empty?}
    AutoFlush -- Yes --> Flush[AutoFlush\nrelated tools]
    AutoFlush -- No --> Return([Return Result])
    Flush --> Return
```

---

### ASCII Diagram (plaintext fallback)

```
┌─────────────────────────────────────────────────────────────────────┐
│                         AI Agent / LLM Client                       │
│                   (Claude, GPT, Custom MCP Client)                  │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │  MCP/SSE  (port 8080)
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         MIDDLEWARE (Go)                              │
│                                                                      │
│   ┌───────────────┐   ┌───────────────┐   ┌──────────────────────┐ │
│   │  MCP Server   │   │  Tool Registry│   │  Cache Manager       │ │
│   │  (SSE + HTTP) │──▶│  (JSON Schemas│   │  Exact + Semantic    │ │
│   └───────┬───────┘   └───────────────┘   └──────────┬───────────┘ │
│           │                                           │             │
│   ┌───────▼───────┐                         ┌────────▼───────────┐ │
│   │  ERP Connector│                         │  Redis Stack       │ │
│   │  (HTTP Client)│                         │  + RediSearch      │ │
│   └───────┬───────┘                         │  + HF Embedder     │ │
│           │                                 └────────────────────┘ │
└───────────┼─────────────────────────────────────────────────────────┘
            │  REST HTTP  (port 8081)
            ▼
┌───────────────────────────────────────────────────────┐
│                  MOCK ERP (Python/FastAPI)             │
│                                                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │  Finance    │  │    HR       │  │  Inventory  │  │
│  │  /invoices  │  │  /employees │  │  /items     │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  │
└───────────────────────────────────────────────────────┘

       ┌──────────────────────────────┐
       │  bridgectl (Go CLI)          │
       │  Developer/Agent CLI Tool    │
       │  context | api | tool |      │
       │  log | cache                 │
       └──────────────────────────────┘
```



## 3. Repository Structure

```
ERPBridge/
│
├── services/
│   └── middleware/
│       └── main.go             # Middleware entrypoint
│
├── tools/
│   └── bridgectl/
│       └── main.go             # CLI entrypoint
│
├── internal/                   # Shared Go libraries
│   ├── mcp/
│   │   ├── server.go           # MCP server + HTTP handlers
│   │   └── tool.go             # Tool struct + Execute() logic
│   ├── cache/
│   │   ├── manager.go          # Two-layer cache orchestration
│   │   ├── exact.go            # Exact-match Redis cache
│   │   ├── semantic.go         # Vector similarity search
│   │   ├── flush.go            # Cache invalidation
│   │   ├── embedder.go         # HuggingFace embedder client
│   │   └── manager_test.go     # Unit tests
│   ├── connector/
│   │   ├── client.go           # Outbound HTTP ERP client
│   │   └── client_test.go      # Unit tests
│   ├── config/
│   │   ├── config.go           # Multi-context YAML config
│   │   └── config_test.go      # Unit tests
│   ├── idp/
│   │   ├── registry.go         # API registration store
│   │   └── generator.go        # Tool schema generator (incl. OpenAPI)
│   ├── logger/
│   │   ├── logger.go           # Broadcast slog + SSE log streaming
│   │   ├── context.go          # Request-scoped logger context
│   │   └── sanitise.go         # Sensitive field redaction
│   ├── output/
│   │   ├── formatter.go        # Table/JSON/YAML output formatter
│   │   └── formatter_test.go   # Unit tests
│   └── cli/
│       ├── root.go             # Cobra root + global flags
│       ├── context.go          # `bridgectl context` commands
│       ├── api.go              # `bridgectl api` commands
│       ├── tool.go             # `bridgectl tool` commands
│       ├── log.go              # `bridgectl log` commands
│       └── cache.go            # `bridgectl cache` commands
│
├── mock-erp/                   # Python FastAPI mock ERP
│   ├── main.py                 # FastAPI app + router inclusion
│   ├── dependencies.py         # Auth middleware (API key, Basic, Bearer)
│   ├── requirements.txt        # Python dependencies
│   ├── Dockerfile              # Mock ERP container
│   └── routers/
│       ├── finance.py          # Invoices endpoints
│       ├── hr.py               # Employees/departments endpoints
│       └── inventory.py        # Items/stock endpoints
│
├── schemas/                    # Auto-generated MCP tool schemas (JSON)
│   └── finance/
│       ├── finance.list_invoices_api_v1_finance_invoices_get.json
│       └── finance.create_invoice_api_v1_finance_invoices_post.json
│
├── .env.example                # Environment variable template (all runtime vars)
├── .github/
│   └── workflows/
│       └── release.yml         # GoReleaser CI/CD pipeline (lint → release → Docker)
├── .goreleaser.yaml            # Multi-platform release configuration
├── docker-compose.yml          # Full stack orchestration
├── Dockerfile.middleware       # Development middleware container
├── Dockerfile.middleware.releaser # Production middleware container
├── go.mod                      # Go module definition
├── go.sum                      # Go dependency checksums
├── README.md                   # Project documentation
└── AGENTS.md                   # AI Agent integration guide
```

---

## 4. Technology Stack

### Go Layer (Middleware + CLI)

| Library | Version | Purpose |
|---|---|---|
| `mark3labs/mcp-go` | v0.51.0 | MCP protocol server (SSE transport) |
| `spf13/cobra` | v1.10.2 | CLI framework for `bridgectl` |
| `redis/go-redis/v9` | v9.19.0 | Redis client (exact + vector search) |
| `goccy/go-yaml` | v1.19.2 | YAML config parsing |
| `getkin/kin-openapi` | v0.137.0 | OpenAPI 3.x spec parsing for schema generation |
| `santhosh-tekuri/jsonschema/v6` | v6.0.2 | JSON Schema validation for tool responses |
| `google/uuid` | v1.6.0 | UUID generation for semantic cache keys |
| `log/slog` | stdlib | Structured logging (Go 1.21+) |

### Python Layer (Mock ERP)

| Library | Purpose |
|---|---|
| `fastapi` | REST API framework |
| `uvicorn` | ASGI server |
| `pydantic` | Request/response model validation |
| `python-multipart` | Form/multipart body parsing (FastAPI dependency) |

### Infrastructure

| Component | Technology | Port |
|---|---|---|
| Mock ERP | Python FastAPI | 8081 |
| Middleware (MCP Server) | Go | 8080 |
| Redis + RediSearch | redis/redis-stack:7.2.0-v9 | 6379 / 8001 (RedisInsight) |
| Text Embedder | HuggingFace TEI (`nomic-embed-text-v1`) | 8083 |

---

## 5. Component Deep-Dives

### 5.1 Mock ERP (Python/FastAPI)

**Location:** `mock-erp/`  
**Port:** `8081`

The Mock ERP simulates a real legacy ERP system with three business modules. It is intentionally simple but implements real authentication patterns.

#### Modules

| Module | Prefix | Endpoints |
|---|---|---|
| Finance | `/api/v1/finance` | `GET /invoices`, `POST /invoices`, `GET /invoices/{id}` |
| HR | `/api/v1/hr` | `GET /employees`, `GET /employees/{id}`, `GET /departments` |
| Inventory | `/api/v1/inventory` | `GET /items`, `GET /items/{id}` |

#### Authentication (`dependencies.py`)

The mock ERP supports three auth mechanisms, resolved in order:

1. **API Key** via `X-API-Key` header — maps key → role
2. **HTTP Basic Auth** — decodes Base64 credentials, maps `user:pass` → role  
3. **Bearer Token** — stub: `dev-stub-token` → `admin` role

**Predefined API Keys:**

```
finance-key-001  →  finance_viewer
finance-key-002  →  finance_editor
hr-key-001       →  hr_viewer
hr-key-002       →  hr_manager
inv-key-001      →  inv_viewer
inv-key-002      →  inv_editor
admin-key-001    →  admin
```

**Role-Based Access Control:** The `check_role()` function enforces permissions on write operations. For example, `POST /invoices` requires the `finance_editor` role. The `admin` role bypasses all checks.

#### Finance Data Model

```python
class Invoice(BaseModel):
    id: Optional[str]
    number: str          # e.g. "INV-2026-001"
    vendor_id: str
    vendor_name: str
    amount: float
    currency: str        # e.g. "LKR"
    status: str          # "pending" | "paid"
    due_date: str
    created_at: str
```

---

### 5.2 Middleware Service (Go/MCP)

**Location:** `services/middleware/main.go`  
**Port:** `8080`

The middleware is the heart of ERPBridge. It is a Go process that:
1. Initialises the structured logger
2. Optionally connects to Redis for caching
3. Creates the `connector.Client` (outbound HTTP to ERP)
4. Creates the `mcp.Server` (wraps `mark3labs/mcp-go`)
5. Walks the `schemas/` directory, deserialising every `.json` file into a `mcp.Tool` and registering it
6. Starts an `http.ServeMux` that serves both the MCP SSE endpoints and the REST management API

#### Startup Flow

```
main()
  ├── logger.Init()            → structured slog, broadcasts to SSE listeners
  ├── redis.NewClient()        → optional Redis connection
  ├── cache.NewHFEmbedder()    → optional embedding client (HuggingFace TEI)
  ├── cache.NewManager()       → two-layer cache (exact + semantic)
  ├── connector.NewClient()    → HTTP client to ERP
  ├── mcp.NewServer()          → MCP protocol wrapper
  ├── loadTools(dir)           → walk schemas/*.json → RegisterTool()
  └── http.ListenAndServe()    → serve on :8080
```

#### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `MCP_PORT` | `8080` | HTTP listen port |
| `SCHEMAS_DIR` | `schemas` | Path to JSON tool schema directory |
| `REDIS_URL` | *(empty)* | Redis connection URL — cache disabled if absent |
| `EMBEDDER_URL` | *(empty)* | HuggingFace TEI base URL — semantic cache disabled if absent |
| `BASE_URL` | `http://localhost:{MCP_PORT}` | Public URL used in SSE handshake |
| `ERP_BASE_URL` | *(empty)* | Base URL for the ERP service (used in docker-compose) |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `APP_ENV` | `development` | `production` enables JSON log format; otherwise text |

---

### 5.3 bridgectl CLI (Go/Cobra)

**Location:** `tools/bridgectl/main.go`, `internal/cli/`

`bridgectl` is the developer and agent operator CLI. It communicates with the Middleware's REST management API (`/api/*`). Built with Cobra, it has a hierarchical command structure.

#### Command Tree

```
bridgectl
├── context
│   ├── list                    # Show all saved contexts with active indicator
│   └── set [name]              # Switch active context + save config
├── api
│   ├── register                # Register a new ERP endpoint in local registry
│   ├── list                    # List all registered APIs
│   └── test [name]             # Fire a test HTTP request to the API
├── tool
│   ├── list                    # List all JSON schemas in schemas/ directory
│   ├── generate                # Generate a schema from a registered API
│   └── invoke [name] [args]    # Call a tool via /api/tools/invoke
├── log
│   ├── tail                    # SSE stream from /api/logs/stream
│   └── stats                   # Fetch recent logs + display level breakdown
└── cache
    ├── stats                   # GET /api/cache/stats
    └── flush [tool]            # Flush by tool, module, or all

Global Flags:
  -o, --output  table|json|yaml   (default: table)
  -c, --context <name>            Override active context
  -v, --verbose                   Enable debug log level
```

#### Output Formatting

Every CLI command produces output through `internal/output.Formatter`. Each response struct implements `TableRenderer` (for human-readable aligned tables) while also being JSON/YAML-serialisable for machine consumption.

---

## 6. Internal Packages Reference

### 6.1 `internal/mcp`

**Files:** `server.go`, `tool.go`

This package wraps `mark3labs/mcp-go` and provides the custom tool execution and HTTP handler logic.

#### Key Types

```go
// Tool — the full definition of an MCP Tool, loaded from a JSON schema file
type Tool struct {
    Name         string        // Unique identifier, e.g. "finance.list_invoices_..."
    Description  string        // Shown to AI agents during tool discovery
    Module       string        // ERP module grouping (finance, hr, inventory)
    InputSchema  InputSchema   // JSON Schema describing accepted arguments
    OutputSchema *any          // Optional JSON Schema for response validation
    Endpoint     *Endpoint     // HTTP endpoint config for the ERP call
    Cache        *cache.Config // Per-tool cache policy
}

// Endpoint — routing info for the ERP call
type Endpoint struct {
    Method string   // GET | POST | PATCH | DELETE
    Path   string   // Absolute URL or relative path
    Auth   AuthInfo // Auth credentials/config
}

// ToolResult — unified result envelope
type ToolResult struct {
    Result  any  // Decoded JSON from ERP
    Error   any  // Error detail if any
    IsError bool // True if ERP returned 4xx/5xx
}
```

#### Tool Execution Flow (`tool.go → Execute()`)

```
Execute(ctx, args, connector)
  ├── Build query params (GET) or JSON body (POST/PATCH)
  ├── Resolve full URL (prepend localhost:8081 for relative paths)
  ├── connector.Call(ctx, ep, queryParams, body)
  ├── Decode JSON response
  ├── If OutputSchema set → validateResponse() via jsonschema
  └── Return ToolResult{Result, IsError}
```

#### Cache Integration in `server.go`

Both `handleMCPToolCall` (SSE path) and `handleDirectInvoke` (CLI path) share an identical two-phase caching strategy:

```
READ PHASE:
  cache.Get() → exact match → return cached response (HIT)
              → semantic search → return cached response (HIT)
              → miss → proceed to ERP call

WRITE PHASE (after successful ERP call):
  cache.Set() → store in exact + semantic caches

INVALIDATION:
  if tool.Cache.FlushOn is non-empty → AutoFlush(flushOn)
```

The `X-Cache-Hit` HTTP response header is set on direct invoke cache hits.

---

### 6.2 `internal/cache`

**Files:** `manager.go`, `exact.go`, `semantic.go`, `flush.go`, `embedder.go`

The most sophisticated package in the codebase. Implements a two-layer caching strategy using Redis.

#### Cache Config (per tool, in JSON schema)

```go
type Config struct {
    Enabled           bool     // Master switch
    TTLSeconds        int      // Expiry for exact cache entries
    SemanticThreshold float32  // Cosine similarity threshold (0–1)
    IsReadOnly        bool     // true=shared across roles, false=role-isolated
    FlushOn           []string // Tool names to invalidate on successful write
}
```

#### Layer 1: Exact Cache

- **Key format:** `exact:{toolName}:{roleScope}:{argsHash}`
- **Hash:** SHA-256 of canonically sorted argument key-value pairs (truncated to 8 hex chars)
- **Storage:** Redis `SET` with TTL
- **Role scoping:** `IsReadOnly=true` uses `"shared"` scope; `false` uses the caller's role (e.g., `"finance_viewer"`)

#### Layer 2: Semantic Cache

- **Storage:** Redis Hash entries with prefix `sem:{uuid}`
- **Index:** RediSearch HNSW vector index `idx:semantic` on `args_emb` field (FLOAT32, DIM=768, COSINE distance)
- **Query:** KNN-1 search filtered by `tool` and `role` tags
- **Scoring:** `similarity = 1 - cosine_distance`. Returned only if `similarity >= SemanticThreshold`
- **Embedder:** `HFEmbedder` POSTs to `{EMBEDDER_URL}/embed` using `nomic-embed-text-v1` (768-dimensional vectors), prepending `"search_query: "` as per nomic's instruction-following spec

#### Cache Invalidation (flush.go)

| Method | Trigger | Scope |
|---|---|---|
| `FlushTool(toolName)` | Manual via `bridgectl cache flush` | Exact keys by scan pattern + semantic by FT.SEARCH tag |
| `FlushModule(module)` | Manual via `bridgectl cache flush --module` | All tools matching `module.*` pattern |
| `AutoFlush(flushOn)` | Automatic after write tool success | Each tool listed in `FlushOn` config |

---

### 6.3 `internal/connector`

**Files:** `client.go`, `client_test.go`

A minimal, dependency-free HTTP client that adds auth and structured logging to outbound ERP requests.

#### Auth Modes

| Type | Mechanism |
|---|---|
| `api-key` | Sets custom header (e.g., `X-API-Key: <value>`) |
| `basic` | `req.SetBasicAuth(username, key)` |
| `bearer` | Sets `Authorization: Bearer <token>` |

#### Request Lifecycle

```
Call(ctx, ep, queryParams, body)
  ├── Build target URL = BaseURL + Path + ?queryParams
  ├── Log request (INFO: method, path, auth_type)
  ├── Read + buffer body (for DEBUG logging)
  ├── http.NewRequestWithContext()
  ├── Apply auth header
  ├── Set Content-Type: application/json
  ├── http.Do()
  ├── Read + buffer response body (for DEBUG logging)
  ├── Log response (INFO: status, latency | WARN: if 4xx/5xx)
  └── Return *http.Response with re-wrapped body
```

Timeout is fixed at **10 seconds**.

---

### 6.4 `internal/config`

**Files:** `config.go`, `config_test.go`

Multi-context configuration, following a kubectl-style pattern.

#### Config File Location

`~/.bridgectl/config.yaml`

#### Config Structure

```yaml
current-context: local
contexts:
  local:
    server: http://localhost:8082      # bridgectl management API
    mcp-server: http://localhost:8080  # MCP SSE endpoint
    erp-base: http://localhost:8081    # Direct ERP URL
    auth:
      type: api-key
      header: X-API-Key
      key: ${BRIDGE_API_KEY}           # Supports ${VAR} expansion
  prod:
    server: https://middleware.company.com
    ...
```

#### Environment Variable Overrides (applied after file load)

| Env Var | Config Field |
|---|---|
| `BRIDGE_CONTEXT` | `current-context` |
| `BRIDGE_SERVER` | `context.server` |
| `BRIDGE_MCP_SERVER` | `context.mcp-server` |
| `BRIDGE_ERP_BASE` | `context.erp-base` |
| `BRIDGE_AUTH_TYPE` | `context.auth.type` |
| `BRIDGE_API_KEY` | `context.auth.key` |
| `BRIDGE_TOKEN` | `context.auth.token` |
| `BRIDGE_USERNAME` | `context.auth.username` |
| `BRIDGE_PASSWORD` | `context.auth.key` (basic auth password) |

The config file also supports inline `${VAR}` environment variable references, expanded via `os.ExpandEnv` before YAML parsing.

---

### 6.5 `internal/idp`

**Files:** `registry.go`, `generator.go`

"IDP" stands for **Integration Definition Provider** — this package handles registering ERP APIs and generating MCP tool schemas from them.

#### Registry (`registry.go`)

A JSON file stored at `~/.bridgectl/registry.json` that persists registered ERP API definitions:

```go
type API struct {
    ID           string    // Auto-generated: "api-{nanosecond timestamp}"
    Name         string    // Unique human name
    URL          string    // Full endpoint URL
    Method       string    // HTTP method
    AuthType     string    // api-key | basic | bearer
    AuthHeader   string
    AuthKey      string
    AuthUsername string
    AuthToken    string
    Module       string    // ERP module (finance, hr, inventory)
    Description  string
    Status       string    // "active"
    CreatedAt    time.Time
}
```

#### Generator (`generator.go`)

Two generation modes:

**1. Basic generation (`Generate`)** — Creates a minimal tool schema from an `API` struct. For `GET` endpoints, adds a default `page` integer parameter.

**2. OpenAPI generation (`GenerateFromOpenAPI`)** — Parses an OpenAPI 3.x specification (URL or file path) using `kin-openapi` and:
- Creates one `Tool` per operation (`path × method`)
- Names tools as `{module}.{operationId}` or `{module}.{method}-{sanitized-path}`
- Maps query parameters → `InputSchema.Properties`
- Maps request body JSON properties → `InputSchema.Properties`
- Infers `OutputSchema` from the `200`/`201` response schema (used for response validation)
- Saves each tool as `schemas/{module}/{toolName}.json`

---

### 6.6 `internal/logger`

**Files:** `logger.go`, `context.go`, `sanitise.go`

A structured logging subsystem built on Go's stdlib `log/slog`, extended with:

#### Broadcast Handler

Every log record is:
1. Written to stdout (text format in dev, JSON in production based on `APP_ENV`)
2. Re-serialised as JSON and broadcast to all **SSE subscriber channels** (for `bridgectl log tail`)
3. Added to a **circular buffer** of the last 1000 records (for `bridgectl log stats` and `/api/logs/recent`)

#### Per-Component Log Levels

Any logger component can have its log level overridden via `LOG_LEVEL_{COMPONENT}` environment variables (e.g., `LOG_LEVEL_CACHE=debug` without changing the global level).

#### Request-Scoped Logger

```go
// Attach logger to context
ctx = logger.WithLogger(ctx, reqLog)

// Retrieve from context (falls back to slog.Default() if not set)
log := logger.FromContext(ctx)
```

#### Sanitisation (`sanitise.go`)

Sensitive argument keys are **always redacted** in DEBUG logs:
`password`, `token`, `api_key`, `secret`, `authorization`, `ssn`, `national_id`, `bank_account`

At INFO level, only argument **keys** (not values) are logged via `logger.ArgKeys()`.
HTTP request/response bodies are truncated to 500 characters in DEBUG logs via `logger.Body()`.

---

### 6.7 `internal/output`

**Files:** `formatter.go`, `formatter_test.go`

A unified output formatting package for the CLI.

#### Format Support

| Format | Implementation |
|---|---|
| `table` | Calls `v.RenderTable(w)` if `v` implements `TableRenderer`; falls back to JSON |
| `json` | `json.NewEncoder` with 2-space indentation |
| `yaml` | `goccy/go-yaml` encoder |

#### Design Pattern

Every CLI response struct implements `TableRenderer`:

```go
type TableRenderer interface {
    RenderTable(w io.Writer) error
}
```

This allows a single `formatter.Print(resp)` call that adapts to the user's chosen output format. Tab-aligned table output uses Go's `text/tabwriter` with 3-space minimum padding.

`RawResponse` wraps `io.Reader` (e.g., HTTP response body) and lazily decodes it only when the formatter calls `RenderTable` or `MarshalJSON`/`MarshalYAML` — avoiding double-reads.

---

### 6.8 `internal/cli`

**Files:** `root.go`, `context.go`, `api.go`, `tool.go`, `log.go`, `cache.go`

The CLI package wires together all other internal packages via the Cobra command framework.

**Shared state** is managed through package-level variables initialised in `PersistentPreRunE`:
- `cfg` — loaded `config.Config`
- `formatter` — `output.Formatter` with the chosen format
- `RootLog` — structured logger instance

---

## 7. Data Flow

### AI Agent Tool Call (SSE path)

```
1. Agent connects to GET /mcp/sse
   └── SSE handshake, agent receives session ID + message endpoint

2. Agent sends listTools request
   └── mcp-go returns all registered tools with their JSON schemas

3. Agent decides to call "finance.list_invoices_..."
   └── POST /mcp/messages with CallToolRequest{name, arguments}

4. mcp.Server.handleMCPToolCall()
   ├── Validate arguments type
   ├── Attach request logger (with request_id, tool_name, role)
   ├── CACHE READ:
   │   ├── Exact key lookup in Redis → HIT? return cached JSON
   │   └── Semantic KNN search in RediSearch → HIT? return cached JSON
   ├── tool.Execute(ctx, args, connector)
   │   ├── Build HTTP request (GET with query params)
   │   ├── connector.Call() → POST to http://mock-erp:8081/api/v1/finance/invoices
   │   ├── Decode JSON response
   │   └── Optional: validateResponse() against OutputSchema
   ├── CACHE WRITE: store in exact + semantic caches
   ├── CACHE INVALIDATION: AutoFlush(flushOn) if configured
   └── Return mcp.CallToolResult{text: JSON string}

5. Agent receives JSON string result
```

### Developer Direct Invoke (CLI path)

```
bridgectl tool invoke finance.list_invoices '{}'
  └── POST http://localhost:8080/api/tools/invoke
      └── Same cache read/write/invalidation flow as SSE path
          └── Response includes X-Cache-Hit header
```

---

## 8. Semantic Caching System

This is the most technically advanced feature of ERPBridge.

### Problem It Solves

An AI agent might ask for invoices in multiple semantically equivalent ways:
- `{"query": "Q1 invoices"}`
- `{"query": "invoices from January to March"}`
- `{"query": "first quarter billing records"}`

Traditional key-value caches would miss all but the first. The semantic cache recognises these as similar and returns the cached result.

### Implementation Details

```
Query Flow:
  args (map) → JSON string → "search_query: {json}"
             → POST /embed (HuggingFace TEI)
             → float32[768] vector
             → FT.SEARCH idx:semantic KNN 1
             → cosine_distance → similarity = 1 - distance
             → if similarity >= threshold → CACHE HIT

Storage Flow:
  args → JSON → embed → float32[768]
  Redis HSET sem:{uuid}:
    tool:     "finance.list_invoices"
    role:     "shared" | "finance_viewer"
    args_raw: '{"query":"Q1 invoices"}'
    response: '{"data":[...],"total":2}'
    args_emb: <768 × 4 bytes little-endian>
    created:  1714900000
    ttl:      300
```

### Vector Index Structure

```
FT.CREATE idx:semantic ON HASH PREFIX 1 "sem:"
SCHEMA
  tool    TAG
  role    TAG
  args_emb VECTOR HNSW 6 TYPE FLOAT32 DIM 768 DISTANCE_METRIC COSINE
```

### Role Scoping

| `IsReadOnly` | Behaviour |
|---|---|
| `true` | All roles share the same cache under key scope `"shared"` |
| `false` | Each role has isolated cache entries (e.g., `"finance_viewer"`) |

This ensures that a `finance_editor` doesn't serve cached results to a `finance_viewer` if the data differs per role.

---

## 9. Tool Schema System

### JSON Schema Format

Tool schemas are self-contained JSON files stored in `schemas/{module}/{toolName}.json`:

```json
{
  "name": "finance.list_invoices_api_v1_finance_invoices_get",
  "description": "List Invoices",
  "module": "finance",
  "inputSchema": {
    "type": "object",
    "properties": {
      "page": { "type": "integer", "description": "Page number", "default": 1 }
    }
  },
  "outputSchema": {},
  "endpoint": {
    "method": "GET",
    "path": "/api/v1/finance/invoices",
    "auth": {
      "type": "api-key",
      "header": "X-API-Key",
      "keyRef": "finance-key-001"
    }
  },
  "cache": {
    "enabled": true,
    "ttlSeconds": 300,
    "semanticThreshold": 0.92,
    "isReadOnly": true,
    "flushOn": []
  }
}
```

### Schema Discovery at Startup

The middleware walks the `SCHEMAS_DIR` recursively at startup, loading all `.json` files:

```go
filepath.Walk(dir, func(path, info, err) {
    data := os.ReadFile(path)
    json.Unmarshal(data, &tool)
    s.RegisterTool(&tool)
})
```

### Adding New Tools (Development Workflow)

```bash
# 1. Register the raw ERP API
bridgectl api register \
  --name finance.create-invoice \
  --url http://localhost:8081/api/v1/finance/invoices \
  --method POST \
  --module finance \
  --description "Create a new vendor invoice" \
  --auth-type api-key \
  --auth-header X-API-Key \
  --auth-key finance-key-002

# 2. Generate schema (basic or from OpenAPI)
bridgectl tool generate --api finance.create-invoice
# OR from OpenAPI spec:
bridgectl tool generate --api finance --openapi http://localhost:8081/openapi.json

# 3. Verify the tool appears
bridgectl tool list

# 4. Test the tool
bridgectl tool invoke finance.create-invoice '{"number":"INV-2026-003","vendor_id":"v-001",...}'
```

---

## 10. Authentication Model

### Middleware → ERP Authentication

Each tool schema carries embedded auth credentials in the `endpoint.auth` field:

| Auth Type | Credential Flow |
|---|---|
| `api-key` | Sets `{header}: {keyRef}` on every outbound request |
| `basic` | `Authorization: Basic base64(username:key)` |
| `bearer` | `Authorization: Bearer {token}` |

Currently, the `keyRef` / `token` / `key` values are stored **inline in the JSON schema file**. In production, these should be resolved from a secrets manager (HashiCorp Vault, AWS Secrets Manager, etc.).

### Agent → Middleware Authentication

The current implementation does **not** enforce authentication on the MCP SSE endpoint or the management API. This is appropriate for local development but would require an API gateway or auth middleware layer in production.

### Mock ERP Role Isolation

The Mock ERP implements full role-based access:
- `GET` endpoints: accessible to any valid API key
- `POST /invoices`: requires `finance_editor` or `admin` role
- 401 Unauthorized: returned for missing/invalid credentials
- 403 Forbidden: returned for insufficient role

---

## 11. Configuration System

### Config File (`~/.bridgectl/config.yaml`)

```yaml
current-context: local
contexts:
  local:
    server: http://localhost:8080      # Management API (cache, logs, tool invoke)
    mcp-server: http://localhost:8080  # MCP SSE + tool invoke
    erp-base: http://localhost:8081    # Raw ERP URL
    auth:
      type: api-key
      header: X-API-Key
      key: ${BRIDGE_API_KEY}
  staging:
    server: https://middleware-staging.company.com
    mcp-server: https://middleware-staging.company.com
    erp-base: https://erp-staging.company.com
    auth:
      type: bearer
      token: ${STAGING_TOKEN}
```

### Context Switching

```bash
bridgectl context list          # View all contexts
bridgectl context set staging   # Switch and persist to config file
bridgectl --context prod tool list  # One-shot override without persisting
```

### Precedence (highest to lowest)

1. `--context` CLI flag (one-shot, not saved)
2. `BRIDGE_*` environment variables
3. `~/.bridgectl/config.yaml` file
4. Hardcoded defaults (`localhost` URLs, `api-key` auth type)

### `.env.example`

A reference `.env.example` file is provided at the repository root documenting all runtime environment variables for both the middleware and the CLI:

```
# Middleware
MCP_PORT=8080
BASE_URL=http://localhost:8080
SCHEMAS_DIR=./schemas
REDIS_URL=redis://localhost:6379
EMBEDDER_URL=http://localhost:8083
ERP_BASE_URL=http://localhost:8081
LOG_LEVEL=info
APP_ENV=development

# CLI (bridgectl)
BRIDGE_CONTEXT=local
BRIDGE_SERVER=http://localhost:8080
BRIDGE_MCP_SERVER=http://localhost:8080
BRIDGE_ERP_BASE=http://localhost:8081
BRIDGE_AUTH_TYPE=api-key
BRIDGE_API_KEY=your-api-key-here
BRIDGE_AUTH_HEADER=X-API-Key

# Mock ERP
MOCK_ERP_PORT=8081
MOCK_ERP_LOG_LEVEL=debug
```

---

## 12. API Endpoints Reference

### MCP Protocol Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/mcp/sse` | SSE transport for AI agents — initial handshake |
| `POST` | `/mcp/messages` | MCP message handler (used post-handshake) |
| `GET` | `/mcp/health` | Health check → `{"status":"ok"}` |

### Management API Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/tools/invoke` | Direct tool invocation (bypasses SSE) |
| `GET` | `/api/cache/stats` | Cache status summary |
| `GET` | `/api/cache/flush` | Flush cache by `?tool=`, `?module=`, or `?all=true` |
| `GET` | `/api/cache/list` | List cache entries *(not yet implemented)* |
| `GET` | `/api/cache/inspect` | Inspect a cache entry *(not yet implemented)* |
| `GET` | `/api/logs/stream` | SSE stream of live log events |
| `GET` | `/api/logs/recent` | JSON array of last 1000 log records |

### Mock ERP Endpoints

| Method | Path | Auth Required | Notes |
|---|---|---|---|
| `GET` | `/health` | None | Health check |
| `GET` | `/api/v1/finance/invoices` | Any valid key | Returns all invoices |
| `POST` | `/api/v1/finance/invoices` | `finance_editor` | Creates invoice |
| `GET` | `/api/v1/finance/invoices/{id}` | Any valid key | Get by ID |
| `GET` | `/api/v1/hr/employees` | Any valid key | Returns all employees |
| `GET` | `/api/v1/hr/employees/{id}` | Any valid key | Get by ID |
| `GET` | `/api/v1/hr/departments` | Any valid key | Returns all departments |
| `GET` | `/api/v1/inventory/items` | Any valid key | Returns all items |
| `GET` | `/api/v1/inventory/items/{id}` | Any valid key | Get by ID |

---

## 13. Docker & Deployment

### `docker-compose.yml` — Full Stack

```
Services:
  mock-erp   → builds from ./mock-erp/Dockerfile → :8081
  embedder   → ghcr.io/huggingface/text-embeddings-inference:cpu-1.5
               --model-id nomic-ai/nomic-embed-text-v1 --port 8083
  redis      → redis/redis-stack:7.2.0-v9 → :6379 + :8001 (RedisInsight)
  middleware → builds from ./Dockerfile.middleware → :8080
               depends_on: mock-erp, redis, embedder (all healthy)

Volumes:
  redis-data → persistent Redis data
  ./schemas  → mounted at /app/schemas (live schema reloads possible)
```

### Startup Dependencies

```
redis (healthy) ─┐
mock-erp (healthy)─┼──▶ middleware
embedder (healthy) ┘
```

### Dockerfiles

| File | Purpose | Base Image |
|---|---|---|
| `mock-erp/Dockerfile` | Python FastAPI mock ERP | `python:3.11-slim` |
| `Dockerfile.middleware` | Development middleware build | `golang:1.23-alpine` → `alpine:latest` |
| `Dockerfile.middleware.releaser` | Production middleware (GoReleaser) | Pre-built binary → `alpine:latest` |

> **Note:** `Dockerfile.middleware` uses `golang:1.23-alpine` as its builder base, while the module requires Go 1.26.2 (`go.mod`). For production builds, GoReleaser compiles the binary in CI with the correct Go version and uses `Dockerfile.middleware.releaser` (which only copies the pre-built binary into `alpine:latest`).

---

## 14. Release Pipeline (GoReleaser)

**File:** `.goreleaser.yaml`

Produces multi-platform, multi-architecture releases via GoReleaser.

### Build Matrix

| Binary | OS | Architecture |
|---|---|---|
| `middleware` | linux, windows, darwin | amd64, arm64 |
| `bridgectl` | linux, windows, darwin | amd64, arm64 |

All builds use `CGO_ENABLED=0` for fully static binaries with `-ldflags "-s -w -X main.version={{.Version}}"`.

### Release Artifacts

| Archive | Contents |
|---|---|
| `middleware_{OS}_{ARCH}.tar.gz` | `middleware` binary + `README.md` |
| `bridgectl_{OS}_{ARCH}.tar.gz` | `bridgectl` binary + `README.md` |
| `erpbridge-full_{OS}_{ARCH}.tar.gz` | Both binaries + `README.md` |
| `checksums.txt` | SHA256 checksums for all artifacts |

### Docker Images (via GoReleaser)

Published to **GitHub Container Registry** (`ghcr.io`):
- `ghcr.io/{owner}/middleware:{version}` — multi-arch manifest (amd64 + arm64)
- `ghcr.io/{owner}/middleware:latest` — multi-arch manifest

---

## 15. CI/CD Workflow (GitHub Actions)

**File:** `.github/workflows/release.yml`

The release pipeline is triggered on any tag push matching `v*` (e.g., `v0.1.0`).

### Permissions Required

| Permission | Reason |
|---|---|
| `contents: write` | Create GitHub Release with assets |
| `packages: write` | Push Docker images to `ghcr.io` |
| `issues: write` | GoReleaser release notes integration |
| `id-token: write` | OIDC token for provenance signing |

### Pipeline Stages

```mermaid
flowchart LR
    Push["git push tag v*"] --> Lint["golangci-lint\n(Go 1.26.2)"]
    Lint -->|pass| Release["GoReleaser release\n--clean"]
    Release --> Binaries["Binary Archives\nmiddleware + bridgectl\nlinux/windows/darwin\namd64 + arm64"]
    Release --> Docker["Docker Buildx\nghcr.io/{owner}/middleware\n:version + :latest\namd64 + arm64"]
    Release --> Checksums["checksums.txt\nSHA256 for all artifacts"]
    Release --> GHRelease["GitHub Release\nwith changelog"]
```

### Key Steps

1. **Lint** (`golangci` job): runs `golangci-lint-action@v6` with the latest linter version
2. **Setup** (`release` job): configures Docker Buildx + QEMU for multi-arch builds, logs in to `ghcr.io`
3. **GoReleaser**: runs with `GITHUB_TOKEN`, `REPO_OWNER`, creating all artifacts defined in `.goreleaser.yaml`

### Tags Released So Far

| Tag | Description |
|---|---|
| `v0.1.0-alpha.1` | First alpha release |
| `v0.1.0-alpha.2` | Second alpha release |

---

## 16. Testing

### Existing Test Files

| File | Coverage |
|---|---|
| `internal/cache/manager_test.go` | Cache manager unit tests |
| `internal/connector/client_test.go` | HTTP connector unit tests |
| `internal/config/config_test.go` | Config load/override unit tests |
| `internal/output/formatter_test.go` | Output formatter unit tests |

### Running Tests

```bash
go test ./...
```

### Test Coverage Gaps

- No integration tests for the full SSE tool call flow
- No tests for the semantic cache (requires Redis + embedder)
- No tests for the OpenAPI generator
- No tests for the CLI commands

---

## 17. Key Design Patterns

### 1. Interface-Driven Dependencies

The MCP server depends on `ERPConnector` (an interface), not the concrete `connector.Client`. This makes unit testing and future alternative connectors straightforward:

```go
type ERPConnector interface {
    Call(ctx context.Context, ep connector.EndpointConfig, queryParams url.Values, body io.Reader) (*http.Response, error)
}
```

Similarly, `cache.Embedder` is an interface, allowing the `HFEmbedder` to be swapped for any other embedding model.

### 2. Schema-Driven Tool Registration

Tools are defined as JSON files, not code. This means:
- Adding a new tool requires no Go code changes — just drop a `.json` file
- Tool definitions are version-controllable and shareable
- The schemas directory can be mounted as a Docker volume for live updates

### 3. Request-Scoped Structured Logging

Every tool invocation gets a unique `request_id` and attaches it to a child `slog.Logger` stored in `context.Context`. All downstream calls (connector, cache) pull this logger from context, ensuring all log lines from a single request share the same `request_id` for correlation.

### 4. Progressive Cache Fallback

```
Exact Match (O(1)) → Semantic Search (O(log n) with HNSW) → ERP Call
```

The most precise and cheapest lookup is tried first. The semantic layer only activates if both an embedder is configured AND the tool's `SemanticThreshold > 0`.

### 5. kubectl-Style Multi-Context Configuration

`bridgectl` follows the kubectl configuration pattern (contexts, current-context), making it intuitive for operators who manage multiple environments.

### 6. Dual-Path Tool Execution

The same tool execution logic runs via:
1. **MCP SSE path** — for AI agents using the standard protocol
2. **Direct HTTP path** (`/api/tools/invoke`) — for `bridgectl` and programmatic access without SSE

Both paths share identical caching, logging, and invalidation behaviour.

---

## 18. Known Limitations & Future Work

### Current Limitations

| Area | Limitation |
|---|---|
| **Auth** | API keys are stored in plaintext in JSON schema files |
| **Middleware Auth** | No authentication on `/api/*` management endpoints |
| **Tool Paths** | Hardcoded `localhost:8081` fallback for relative tool paths |
| **Role Extraction** | Role is never actually extracted from the MCP request context (always empty string) |
| **Cache List/Inspect** | `/api/cache/list` and `/api/cache/inspect` return 501 Not Implemented |
| **Semantic TTL** | Semantic cache entries do not expire (no TTL set on `HSET`) |
| **Connector Timeout** | Fixed 10-second timeout with no per-tool override |
| **Windows Paths** | `filepath.Walk` for schemas uses OS-native path separators |

### Suggested Improvements

1. **Secrets Management** — Resolve `keyRef` values from Vault/AWS Secrets Manager at runtime
2. **Middleware Authentication** — Add JWT/API key validation on management and invoke endpoints
3. **Dynamic Tool Reload** — Use filesystem watchers (e.g., `fsnotify`) to reload schemas without restart
4. **Semantic Cache TTL** — Honour the `TTLSeconds` config for `HSET` entries via `EXPIRE`
5. **Pagination** — Pass `page` parameter from tool input schema through to ERP calls
6. **gRPC/HTTP/2** — Add an alternative MCP transport for higher throughput
7. **Metrics** — Expose Prometheus metrics (cache hit rate, ERP latency, error rates)
8. **Schema Validation** — Validate tool input against `InputSchema` before calling the ERP

---

*Report generated from source analysis of the ERPBridge repository. Last updated 2026-05-06 to reflect `.env.example`, GitHub Actions CI/CD pipeline, Dockerfile notes, and Mermaid architecture diagrams.*
