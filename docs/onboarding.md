# ToolPlane Onboarding Guide

<a id="erpbridge-onboarding-guide"></a>

> Connect a new ERP system to ToolPlane in under 10 minutes with the `bridgectl` CLI.

---

## Windows CLI users

For an already configured remote deployment, follow [bridgectl on Windows](./windows.md)
instead of the local Make/Compose bootstrap below. Windows CLI use does not
require a local server, Docker, Go, or Make.

On Windows, configured context keys such as `prod` and `Prod` cannot coexist.
The CLI refuses case-only ambiguity before registry access or MCP child execution.
Registry paths also reject reserved device stems such as `CON` and `NUL.test`.
If the CLI refuses a name, stop the processes. Privately back up the config and
registries. Manually choose distinct safe names with matching registry filenames. Do not
merge or delete state. See the [recovery steps](./windows.md#windows-context-safety-and-recovery).
Linux context naming and registry behavior remain unchanged.

## Before You Start

Before you start, make sure that these tools are installed:

| Requirement | Purpose |
| --- | --- |
| Docker & Docker Compose | Runs the ToolPlane server and pinned MockERP image |
| `curl` | Checks service health after startup |
| Go 1.26.2+ | Needed to build `bridgectl` (if not pre-built) |

### Preflight before changes

If `bridgectl` is not already installed, build it before this read-only gate.
Choose the target context first. Use `--context <name>` on every CLI command
below. Before registration or apply, run these read-only checks:

```bash
CTX=local # replace local with the selected configured context
./bridgectl version
./bridgectl context list -o json
./bridgectl api list --context "$CTX" -o json
curl -fsS --max-time 5 http://localhost:8080/mcp/health >/dev/null
curl -fsS --max-time 5 http://localhost:8081/health >/dev/null
```

If an env file exists, make sure that its quoted Compose input is valid without
executing the file:

```bash
docker compose --env-file .env config --quiet
```

Never source `.env`. Make sure that a credential source is present without
printing its value.
The selected `mcp-server` must be a host root or end in only `/mcp` or `/mcp/`;
those exact suffixes are normalized for control-plane calls. `/mcp/` is the MCP
transport, not a REST path. A non-empty other path returns
`CONTROL_PLANE_URL_INVALID`. Make sure that `api test --help` contains `--local`.
The normal probe runs on the server and returns no body. A legacy registry, unhealthy
stack, invalid context, or ambiguous manifest location is a stop condition.

---

<a id="step-1--start-the-erpbridge-server"></a>

## Step 1 — Start the ToolPlane Server

Use the non-interactive bootstrap for local development:

```bash
make dev-up
```

If neither `MOCK_ERP_CREDENTIALS_JSON` nor `MOCK_ERP_CREDENTIALS_FILE` is set,
the bootstrap generates an ephemeral MockERP credential pair. It keeps those
values in memory. It runs `docker compose config --quiet` to make sure that the
configuration is valid. It force recreates the stack. It waits for MockERP and
ToolPlane health. It does not
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

Make sure that all services are running:

```bash
docker compose ps
```

The ToolPlane server is available at **`http://localhost:8080`**. The local
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

Tell ToolPlane how to connect to your ERP system. The credential flag names a
logical reference. It does not contain the credential value. By default, the
server resolves the reference from its environment:

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

Convert the pinned MockERP OpenAPI spec into MCP tool manifests. Keep reviewed
manifests under `manifests/<module>/`. Generated output is a temporary draft.
It must not become a second source of truth.

> **Warning:** The task below registers `erp` itself. If you completed Step 3
> or `erp` already exists, skip this task. Use the explicit CLI generation
> examples below instead. The task does not pass `--context "$CTX"`; it uses
> the CLI's configured selection. Before you run it, make sure that this
> selection is the intended context. Do not force replacement to bypass a
> duplicate-registration error.

```bash
make generate-tools
```

The target creates one bounded YAML draft stream. It applies that stream once.
On exit, it removes both temporary files. It does not create `schemas/`,
per-tool files, or another generated artifact. Set `MOCK_ERP_VERSION` and
`MOCK_ERP_OPENAPI_URL` together when upgrading the pinned MockERP contract.

To create one reviewed file per tool, use `--output-dir`. The CLI creates the
directory when needed. Each file uses the exact tool name:

```bash
./bridgectl tool generate \
  --context "$CTX" \
  --api erp \
  --openapi /path/to/openapi.yaml \
  --output-dir manifests/erp \
  -o yaml
# Creates manifests/erp/<tool-name>.yaml for each generated tool.
./bridgectl tool apply --context "$CTX" -f manifests/erp
```

For a review-only draft, write the CLI output to a temporary file. After review
or apply, remove that file:

```bash
manifest=$(mktemp "${TMPDIR:-/tmp}/erpbridge-draft.XXXXXX.yaml")
trap 'rm -f "$manifest"' EXIT
./bridgectl tool generate --context "$CTX" --api erp --openapi /path/to/openapi.yaml -o yaml > "$manifest"
# Review the draft, then validate it before the confirmed apply.
./bridgectl tool validate --context "$CTX" -f "$manifest"
./bridgectl tool apply --context "$CTX" -f "$manifest"
```

Generated output is a draft. Complete its intent metadata, schema, security,
and cache policy. Before you apply it, review those fields. OpenAPI request bodies preserve
nested objects, arrays, enums, defaults, and required fields. Generated
`execution.parameterLocations` routes path, query, header, and body arguments;
primitive or array request bodies use one complete `body` argument. Generated
path values are escaped, protected headers are rejected, and
`responsePath: data` is emitted only when the resolved response schema proves a
top-level `data` property. Successful `HEAD` and `204 No Content` responses return a nil
result without JSON decoding.

Generated manifests can include optional method-based MCP annotations. During
MCP `tools/list`, ToolPlane also exposes the reviewed intent guidance and
non-empty `allowedRoles` as namespaced `_meta` values:

- `io.erpbridge/whenToUse`
- `io.erpbridge/whenNotToUse`
- `io.erpbridge/examples`
- `io.erpbridge/allowedRoles`

These values help a host or model reference a tool. Some MCP clients might not
send custom `_meta` to the model. `allowedRoles` is only a discovery hint.
The authenticated identity, selected role, and server-side allow-list control
access. Do not use a role name as permission to call
a tool.

A `pii` or `restricted` tool
requires an existing non-identifying role slug in `allowedRoles`. Do not use a
person name, email, employee number, or ERP record in roles or examples. The
`system.*_test` tools are development-only and require
`MCP_ENABLE_TEST_TOOLS=true`. RedisInsight is for local inspection. It must
remain loopback-only unless you explicitly enable another interface.

---

## Step 5 — Apply Tools to the Registry

Apply your reviewed manifests to the ToolPlane server.

**Apply a reviewed manifest:**

Keep reviewed, applied manifests as the single source of truth under
`manifests/<module>/`. For example:

```bash
./bridgectl tool apply --context "$CTX" -f manifests/erp/tools.yaml
```

The command accepts one YAML sequence or multi-document YAML stream. It applies
each tool definition once. It also accepts one JSON/YAML tool or a directory
of reviewed manifest files. Do not retain generated per-tool JSON files or use
`schemas/` as a second artifact directory.

---

## Step 6 — Test the ERP API through the server

Run API probes through ToolPlane. The server resolves `credentialRef` from its
configured source. It returns only status, content type, latency, and success.
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
transport URL `http://localhost:8080/mcp/`. The CLI removes that exact suffix
for control-plane calls. Other non-empty paths are rejected. Keep `/mcp/` for
MCP clients. Do not use it as the REST API path.

### Optional mounted credential rotation

Keep environment variables for the default deployment. For a selected API or
plugin, register or manifest a `credentialSource: file` value and set
`ERPBRIDGE_CREDENTIALS_DIR` in the ToolPlane process. The process reads the
reference-named file immediately before each request. After provider-managed projected files refresh, it accepts them.
It does not watch or cache them. Missing, empty, invalid, control-character,
non-regular, or larger-than-64 KiB files cause the request to fail closed.

For a local writable mount, write a complete temporary file and atomically
rename it over the live file. For AWS EKS ASCP, AKS Key Vault CSI, or GKE Secret Manager CSI, use workload
identity and least privilege. Mount a directory, not `subPath`. Enable provider
rotation. Expect eventual refresh. Adding a
new cloud secret can require a provider mapping change and workload rollout
before its filename is mounted. `bridgectl api test --local` reads from the
CLI process, so it needs the same mount. The normal server-side probe reads
from ToolPlane. Environment-variable changes still require recreation.

## Step 7 — Verify Everything Is Working

Make sure that your tools are registered:

```bash
./bridgectl tool get --context "$CTX"
```

Your tools must have a `READY` status. If any show another status, see the Troubleshooting section below.

---

## Managing Tools

### Deleting a Tool

Remove a tool from the active registry when you no longer need it.

**Soft Delete (Default):**
A soft delete marks the tool as inactive. It hides the tool from MCP clients. The tool stays in the database for audit.

```bash
./bridgectl tool delete [tool_name] [version]

# Example:
./bridgectl tool delete list_items 1.0.0
```

**Hard Delete (Permanent):**
A hard delete permanently removes the tool from the SQLite database.

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

- `CONTEXT_NOT_FOUND`: list contexts. Rerun with a configured `--context`.
- `LEGACY_REGISTRY`: stop writes. Complete the confirmed scrub. Then migrate
  the cleaned global registry to the selected context.
- `REGISTRY_CONFLICT`: inspect the existing API. Use `--force` only for an
  intentional replacement.
- `CONTROL_PLANE_URL_INVALID`: use the host root or exact `/mcp` suffix only.
- `VALIDATION_FAILED`: repair the reviewed manifest. Make sure that it passes
  local validation.
- `AUTHENTICATION_FAILED` or `AUTHORIZATION_DENIED`: make sure that the caller
  has the intended bridge scope or tool role. Do not display credentials or
  elevate the caller.
- `UPSTREAM_UNREACHABLE`, `HEALTH_CHECK_FAILED`, or `API_PROBE_FAILED`: make sure
  that both health endpoints respond. Review bounded server-side probe evidence. A configured
  but unavailable Redis backend keeps its Redis label. Cache health returns
  `503 HEALTH_CHECK_FAILED` without memory substitution.
- `RATE_LIMITED`: retry the direct request after the `Retry-After` value. MCP
  tool calls keep HTTP `200` and expose `result.isError=true`.
- `INSECURE_TRANSPORT`: use HTTPS, or the exact documented local fixture
  exception only.
- `RECONCILIATION_FAILED` or `RESOURCE_NOT_FOUND`: before you retry, read back
  exact names and versions.

### Connection refused when running CLI commands

**Error:**

```text
apply failed: Get "http://localhost:8080/...": dial tcp 127.0.0.1:8080: connect: connection refused
```

**Cause:** The ToolPlane server is not running, or the CLI points to the wrong address.

**Fix:**

1. Make sure that the Docker services are running:

   ```bash
   docker compose ps
   ```

2. Make sure that the CLI context selects the intended deployment:

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

**Fix:** Make sure that the configured release exists. Fetch the contract explicitly:

```bash
MOCK_ERP_VERSION=0.2.1 make generate-tools
```

If you use a private fork, set `MOCK_ERP_OPENAPI_URL` to its versioned raw URL.

### Tools are applied but calls return `internal server error`

**Cause:** The ToolPlane server cannot reach the ERP service, or the tool cannot reach Redis.

**Fix:** Read the server logs. Make sure that they contain no errors:

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
