# ERPBridge Onboarding Guide

> Connect a new ERP system to ERPBridge in under 10 minutes with the `bridgectl` CLI.

---

## Before You Start

Make sure that you have the following installed:

| Requirement | Purpose |
| --- | --- |
| Docker & Docker Compose | Runs the ERPBridge server and pinned MockERP image |
| Go 1.26.2+ | Needed to build `bridgectl` (if not pre-built) |

---

## Step 1 — Start the ERPBridge Server

Start all required services with Docker Compose:

```bash
docker compose up --build -d
```

Confirm that everything is running:

```bash
docker compose ps
```

The ERPBridge server is available at **`http://localhost:8080`**.

---

## Step 2 — Build the CLI

If you do not have `bridgectl` in your project root, build it now:

```bash
make build
```

Or build it manually:

```bash
go build -o bridgectl ./tools/bridgectl/main.go
```

---

## Step 3 — Register Your ERP API

Tell ERPBridge how to connect to your ERP system:

```bash
./bridgectl api register \
  --name erp \
  --url http://localhost:8081 \
  --module erp \
  --description "Internal Mock ERP for testing"
```

**What each flag does:**

| Flag | Description |
| --- | --- |
| `--name` | Unique identifier for this API |
| `--url` | Base URL of your ERP service |
| `--module` | Logical grouping (for example `finance`, `hr`, `erp`) |
| `--description` | Human-readable description. This flag is required. |

> **Tip:** The `--description` flag is mandatory. It helps the LLM layer understand the purpose of the API.

---

## Step 4 — Generate Tool Schemas

Convert the pinned MockERP OpenAPI spec into MCP tool schemas:

```bash
make generate-tools
```

The command prints one YAML sequence containing all generated tools to stdout. `bridgectl tool apply` accepts that file directly, so you can keep it as one manifest file.

> **Note:** `make generate-tools` fetches the OpenAPI file from the pinned
> MockERP release. The `schemas/` directory is not tracked by git. Set
> `MOCK_ERP_VERSION` and `MOCK_ERP_OPENAPI_URL` together when upgrading.

---

## Step 5 — Apply Tools to the Registry

Upload your schemas to the ERPBridge server.

**Apply a generated manifest or a single tool:**

```bash
./bridgectl tool apply -f schemas/erp/generated.yaml
```

The command also accepts one JSON/YAML tool or a directory of tool files. It applies each YAML sequence item and each YAML document separately.

**Apply all tools in a directory (recursive):**

```bash
./bridgectl tool apply -f schemas/erp/
```

---

## Step 6 — Verify Everything Is Working

Confirm that your tools are registered:

```bash
./bridgectl tool get
```

You see your tools listed with a `READY` status. If any show another status, see the Troubleshooting section below.

---

## Managing Tools

### Deleting a Tool

Remove a tool from the active registry when you no longer need it.

**Soft Delete (Default):**
Marks the tool as inactive and hides it from MCP clients. The tool stays in the database for audit.

```bash
./bridgectl tool delete [tool_name] [version]

# Example:
./bridgectl tool delete list_items 1.0.0
```

**Hard Delete (Permanent):**
Completely removes the tool from the SQLite database.

```bash
./bridgectl tool delete [tool_name] [version] --hard
```

> CAUTION: `--hard` deletes the tool from the database. You cannot restore it.

> **Note:** To restore a soft-deleted (hidden) tool, apply its schema again:
>
> ```bash
> ./bridgectl tool apply -f schemas/erp/list_items.json
> ```

---

## Troubleshooting

### Connection refused when running CLI commands

**Error:**

```
apply failed: Get "http://localhost:8080/...": dial tcp 127.0.0.1:8080: connect: connection refused
```

**Cause:** The ERPBridge server is not running, or the CLI points to the wrong address.

**Fix:**

1. Check that the Docker services are up:

   ```bash
   docker compose ps
   ```

2. Check your CLI context:

   ```bash
   ./bridgectl context list
   ```

3. If the address is wrong, override it with environment variables:

   ```bash
   export BRIDGE_SERVER=http://localhost:8080
   export BRIDGE_MCP_SERVER=http://localhost:8080
   ```

   Or edit `~/.bridgectl/config.yaml` and change the `server` value of the active context.

### Registration fails with a missing flag error

**Error:**

```
required flag(s) "description" not set
```

**Cause:** The `--description` flag is required on `api register`.

**Fix:** Always include it:

```bash
./bridgectl api register \
  --name erp \
  --url http://localhost:8081 \
  --module erp \
  --description "Internal Mock ERP for testing"
```

### OpenAPI spec not found

**Error:**

```
failed to load OpenAPI spec
```

**Cause:** The pinned MockERP OpenAPI URL is unavailable or the configured
release does not exist.

**Fix:** Check the configured release and fetch the contract explicitly:

```bash
MOCK_ERP_VERSION=0.1.1 make generate-tools
```

If you use a private fork, set `MOCK_ERP_OPENAPI_URL` to its versioned raw URL.

### Tools are applied but calls return `internal server error`

**Cause:** The ERPBridge server cannot reach the ERP service, or the tool cannot reach Redis.

**Fix:** Check the server logs for errors:

```bash
docker compose logs erpbridge-server
```

Make sure that the `mock-erp` and `redis` containers are healthy:

```bash
docker compose ps
```

If a container is not healthy, restart it:

```bash
docker compose restart redis
```

---

## Quick Reference

```bash
# Start services
docker compose up --build -d

# Build CLI
make build

# Register API (make generate-tools also performs this step)
./bridgectl api register --name erp --url http://localhost:8081 --module erp --description "..."

# Fetch the pinned OpenAPI contract and generate schemas
make generate-tools

# Apply all tools
./bridgectl tool apply -f schemas/erp/

# Verify tools are READY
./bridgectl tool get

# Delete a tool (Soft - sets to HIDDEN)
./bridgectl tool delete [tool_name] [version]

# Delete a tool (Hard - permanent removal)
./bridgectl tool delete [tool_name] [version] --hard
```
