# ERPBridge Onboarding Guide

> Connect a new ERP system to ERPBridge in under 10 minutes with the `bridgectl` CLI.

---

## Before You Start

Make sure that you have the following installed:

| Requirement | Purpose |
| --- | --- |
| Docker & Docker Compose | Runs the ERPBridge server and pinned MockERP image |
| `curl` | Checks service health after startup |
| Go 1.26.2+ | Needed to build `bridgectl` (if not pre-built) |

### Preflight before changes

If `bridgectl` is not already installed, build it before this read-only gate.
Choose the target context first and use `--context <name>` on every CLI
command below. Run these read-only checks before registration or apply:

```bash
CTX=local # replace local with the selected configured context
./bridgectl version
./bridgectl context list -o json
./bridgectl api list --context "$CTX" -o json
curl -fsS --max-time 5 http://localhost:8080/mcp/health >/dev/null
curl -fsS --max-time 5 http://localhost:8081/health >/dev/null
```

If an env file exists, validate its quoted Compose input without executing it:

```bash
docker compose --env-file .env config --quiet
```

Never source `.env`. Check credential-source presence without printing values.
The selected `mcp-server` must be a host root or end in only `/mcp` or `/mcp/`;
those exact suffixes are normalized for control-plane calls. `/mcp/` is the MCP
transport, not a REST path. A non-empty other path returns
`CONTROL_PLANE_URL_INVALID`. Confirm that `api test --help` contains `--local`;
the normal probe is server-side and body-free. A legacy registry, unhealthy
stack, invalid context, or ambiguous manifest location is a stop condition.

---

## Step 1 — Start the ERPBridge Server

Use the non-interactive bootstrap for local development:

```bash
make dev-up
```

The bootstrap generates an ephemeral MockERP credential pair only when neither
`MOCK_ERP_CREDENTIALS_JSON` nor `MOCK_ERP_CREDENTIALS_FILE` is set. It keeps
those values in memory, validates `docker compose config --quiet`, force
recreates the stack, and waits for MockERP and ERPBridge health. It does not
write credentials to `.env` or print them.

Compose also reads quoted values from an env file. Do not source `.env` in your
shell. If you provide credentials, use Compose's env-file handling or exported
variables, for example:

```bash
docker compose --env-file .env config --quiet
docker compose --env-file .env up --build --force-recreate -d
```

Direct Compose use requires one of the two MockERP credential sources. It does
not run the bootstrap preflight; without credentials, MockERP fails closed.

Confirm that everything is running:

```bash
docker compose ps
```

The ERPBridge server is available at **`http://localhost:8080`**. The local
health endpoints are `http://localhost:8081/health` and
`http://localhost:8080/mcp/health`.

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

Tell ERPBridge how to connect to your ERP system. The credential flag names a logical reference; it does not contain the
credential value. Environment resolution is the default:

```bash
./bridgectl api register \
  --context "$CTX" \
  --name erp \
  --url http://localhost:8081 \
  --module erp \
  --description "Internal Mock ERP for testing" \
  --credential-ref ERP_API_KEY
```

**What each flag does:**

| Flag | Description |
| --- | --- |
| `--name` | Unique identifier for this API |
| `--url` | Base URL of your ERP service |
| `--module` | Logical grouping (for example `finance`, `hr`, `erp`) |
| `--description` | Human-readable description. This flag is required. |
| `--credential-ref` | Logical credential reference. The server resolves it from the environment by default. |
| `--credential-source` | Optional `env` or `file`; use `file` with `ERPBRIDGE_CREDENTIALS_DIR` for a mounted credential. |

> **Tip:** The `--description` flag is mandatory. It helps the LLM layer understand the purpose of the API.

---

## Step 4 — Generate Tool Schemas

Convert the pinned MockERP OpenAPI spec into MCP tool schemas. Keep reviewed
manifests under `manifests/<module>/`; generated YAML is a temporary draft and
is not a second `schemas/` or per-tool JSON source of truth:

```bash
make generate-tools
```

The target creates one bounded YAML draft stream, applies that stream once, and
removes both temporary files when it exits. It does not create `schemas/`,
per-tool JSON files, or another generated artifact. Set `MOCK_ERP_VERSION` and
`MOCK_ERP_OPENAPI_URL` together when upgrading the pinned MockERP contract.

For a review-only draft, write the explicit CLI output seam to a temporary file
and remove it after review or apply:

```bash
manifest=$(mktemp "${TMPDIR:-/tmp}/erpbridge-draft.XXXXXX.yaml")
trap 'rm -f "$manifest"' EXIT
./bridgectl tool generate --context "$CTX" --api erp --openapi /path/to/openapi.yaml -o yaml > "$manifest"
# Review the draft, then validate it before the confirmed apply.
./bridgectl tool validate --context "$CTX" -f "$manifest"
./bridgectl tool apply --context "$CTX" -f "$manifest"
```

Generated output is a draft. Complete and review its intent metadata, schema,
security, and cache policy before applying it. A `pii` or `restricted` tool
requires an existing non-identifying role slug in `allowedRoles`; do not use a
person name, email, employee number, or ERP record in roles or examples. The
`system.*_test` tools are development-only and require
`MCP_ENABLE_TEST_TOOLS=true`; RedisInsight is for local inspection and must
remain loopback-only or opt-in.

---

## Step 5 — Apply Tools to the Registry

Upload your schemas to the ERPBridge server.

**Apply a reviewed manifest:**

Keep reviewed, applied manifests as the single source of truth under
`manifests/<module>/`. For example:

```bash
./bridgectl tool apply --context "$CTX" -f manifests/erp/tools.yaml
```

The command accepts one YAML sequence or multi-document YAML stream and applies
each tool definition once. It also accepts one JSON/YAML tool or a directory
of reviewed manifest files. Do not retain generated per-tool JSON files or use
`schemas/` as a second artifact directory.

---

## Step 6 — Test the ERP API through the server

Run API probes through ERPBridge. The server resolves `credentialRef` from its
configured source and returns only status, content type, latency, and success.
The ERP response body and headers never cross the probe boundary.

```bash
./bridgectl api test --context "$CTX" erp
```

Use `--local` only for an explicit offline or legacy host-side diagnostic. It
resolves the credential in the CLI process and is not the normal workflow:

```bash
./bridgectl api test --context "$CTX" erp --local
```

The `mcp-server` context value is used as a control-plane root by CLI commands.
It may be a host root such as `http://localhost:8080`, or the exact MCP
transport URL `http://localhost:8080/mcp/`; the CLI removes that exact suffix
for control-plane calls. Other non-empty paths are rejected. Keep `/mcp/` for
MCP clients; do not use it as the REST API path.

### Optional mounted credential rotation

Keep environment variables for the default deployment. For a selected API or
plugin, register or manifest a `credentialSource: file` value and set
`ERPBRIDGE_CREDENTIALS_DIR` in the ERPBridge process. The process reads the
reference-named file immediately before each request. It accepts provider-
managed projected files after they refresh, but does not watch or cache them;
missing, empty, invalid, control-character, non-regular, or larger-than-64 KiB
files fail closed.

For a local writable mount, write a complete temporary file and atomically
rename it over the live file. For AWS EKS ASCP, AKS Key Vault CSI, or GKE Secret
Manager CSI, use workload identity and least privilege, mount a directory and
not `subPath`, enable provider rotation, and expect eventual refresh. Adding a
new cloud secret can require a provider mapping change and workload rollout
before its filename is mounted. `bridgectl api test --local` reads from the
CLI process and therefore needs the same mount; the normal server-side probe
reads from ERPBridge. Environment-variable changes still require recreation.

## Step 7 — Verify Everything Is Working

Confirm that your tools are registered:

```bash
./bridgectl tool get --context "$CTX"
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
> ./bridgectl tool apply -f manifests/erp/tools.yaml
> ```

---

## Troubleshooting

### Stable error recovery

Control-plane errors return a stable code and a safe suggestion. Use the code
as the recovery key, not an HTML body or a stack trace:

- `CONTEXT_NOT_FOUND`: list contexts and rerun with a configured `--context`.
- `LEGACY_REGISTRY`: stop writes; complete the confirmed scrub, then migrate
  the cleaned global registry to the selected context.
- `REGISTRY_CONFLICT`: inspect the existing API; use `--force` only for an
  intentional replacement.
- `CONTROL_PLANE_URL_INVALID`: use the host root or exact `/mcp` suffix only.
- `VALIDATION_FAILED`: repair and locally validate the reviewed manifest.
- `AUTHENTICATION_FAILED` or `AUTHORIZATION_DENIED`: check the intended bridge
  scope or tool role without displaying credentials or elevating the caller.
- `UPSTREAM_UNREACHABLE`, `HEALTH_CHECK_FAILED`, or `API_PROBE_FAILED`: check
  both health endpoints and bounded server-side probe evidence.
- `INSECURE_TRANSPORT`: use HTTPS, or the exact documented local fixture
  exception only.
- `RECONCILIATION_FAILED` or `RESOURCE_NOT_FOUND`: read back exact names and
  versions before retrying.

### Connection refused when running CLI commands

**Error:**

```text
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

```text
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

```text
failed to load OpenAPI spec
```

**Cause:** The pinned MockERP OpenAPI URL is unavailable or the configured
release does not exist.

**Fix:** Check the configured release and fetch the contract explicitly:

```bash
MOCK_ERP_VERSION=0.2.1 make generate-tools
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
CTX=local # replace local with the selected configured context

# Start services with ephemeral local credentials
make dev-up

# Or use direct Compose with a credential source (never source .env)
docker compose --env-file .env up --build --force-recreate -d

# Build CLI
make build

# Register API (make generate-tools also performs this step)
./bridgectl api register --context "$CTX" --name erp --url http://localhost:8081 --module erp --description "..."

# Generate one temporary draft, apply it once, and clean it up
make generate-tools

# Apply the reviewed source manifest
./bridgectl tool apply --context "$CTX" -f manifests/erp/tools.yaml

# Verify tools are READY
./bridgectl tool get --context "$CTX"

# Delete a tool (Soft - sets to HIDDEN)
./bridgectl tool delete --context "$CTX" [tool_name] [version]

# Delete a tool (Hard - permanent removal)
./bridgectl tool delete --context "$CTX" [tool_name] [version] --hard
```
