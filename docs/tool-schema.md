# MCP Tool Schema Reference (V2)

ERPBridge uses a Kubernetes-style declarative resource format to define tools. This ensures tools are versioned, intent-based, and decoupled from the underlying ERP structure.

---

## 🏗 Resource Structure

A tool definition is composed of four main sections: `apiVersion/kind`, `metadata`, `spec`, and `status` (internal only).

```yaml
apiVersion: erpbridge.io/v1
kind: MCPTool
metadata:
  name: list_employees
  version: 1.2.0
  module: hr
spec:
  description: { ... }
  inputSchema: { ... }
  execution: { ... }
  security: { ... }
  cache: { ... }
  routing: { ... }
```

---

## 🔑 Field Definitions

### `metadata`

Identity and grouping information.

- **`name`**: (String) Unique identifier. Use **intent-based names** (e.g., `list_employees`) instead of technical ones (e.g., `get_resource_employee`).
- **`version`**: (String) SemVer version (e.g., `1.0.0`).
- **`module`**: (String) Logical grouping for access control and organization.

### `spec.description`

High-signal information to help the LLM select the correct tool.

- **`short`**: (String) Concise summary of what the tool does.
- **`whenToUse`**: (Array) List of scenarios where this tool is appropriate.
- **`whenNotToUse`**: (Array) List of similar scenarios where you must not use this tool.
- **`examples`**: (Array) Sample user queries that trigger this tool.

`bridgectl tool generate` derives `whenToUse` and `examples` from the
OpenAPI operation summary or description. Treat these values as draft evidence
and review them before applying the manifest.

### `spec.annotations`

Optional MCP behavioral hints. These values are additive and do not change the
input or output schema.

- **`title`**: (String, optional) Human-readable display title.
- **`readOnlyHint`**: (Boolean, optional) Whether the tool is expected not to modify its environment.
- **`destructiveHint`**: (Boolean, optional) Whether a modifying operation may be destructive.
- **`idempotentHint`**: (Boolean, optional) Whether repeated calls with the same arguments are expected to have no additional effect.
- **`openWorldHint`**: (Boolean, optional) Whether the tool interacts with external entities.

Generated annotations are method-based draft hints. Review them before
applying a generated manifest; they do not replace authorization or other
server-side controls.

### `spec.inputSchema`

Standard JSON Schema defining the arguments. **Strict typing is mandatory.**

- Use `properties` to define fields and `required` to enforce them.
- Nested objects use nested `properties` and `required`; arrays use `items`.
- **Avoid "filters" strings**: Break down complex query requirements into individual typed properties.

### `spec.execution`

Technical mapping to the ERP API.

- **`method`**: (String) `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS`, or `TRACE`.
- **`endpoint`**: (String) The ERP API URL path.
- **`mapping`**: (Map) Optional. Maps LLM arg names to ERP parameter names.
- **`parameterLocations`**: (Map) Generated metadata mapping each LLM argument to `path`, `query`, `header`, or `body`. If absent, GET arguments use the query and other methods use one JSON object body for compatibility.
- **`bodyArgument`**: (String) Generated complete-body argument for primitive or array JSON request bodies. Its value is serialized as the complete body instead of an object property.
- **`responsePath`**: (String) Optional JSON key to extract from the ERP response. OpenAPI generation emits `data` only when the resolved top-level response schema is an object with a top-level `data` property; the output schema then describes that unwrapped value.

Generated path values are URL-escaped. Generated headers are allowlisted and
cannot replace connector authentication or transport headers. `Authorization`,
`Proxy-Authorization`, `Cookie`, `Host`, connection, transfer, upgrade,
`Content-Length`, and `Content-Type` parameters are rejected during generation.
A successful `HEAD` or `204 No Content` response returns a successful nil
result without JSON decoding.

### `spec.security`

Authentication strategy.

- **`authType`**: `api-key`, `bearer`, or `basic`.
- **`credentialRef`**: A logical reference, normally the name of the environment variable containing the secret. **Never embed raw secrets here.**
- **`credentialSource`**: (Optional) `env` or `file`; omitted means `env`. `file` reads `<ERPBRIDGE_CREDENTIALS_DIR>/<credentialRef>` immediately before each request and never falls back to the environment.
- **`dataClass`**: (Optional string) Declared sensitivity: `public`, `internal`, `pii`, or `restricted`. The field is optional for compatibility with older schemas.
- **`allowedRoles`**: (Optional array) Roles that may call this tool. Each role must match `[a-z][a-z0-9_-]{0,63}`; the list must contain unique values and no more than 32 roles. `pii` and `restricted` tools must provide at least one role.

When `allowedRoles` is present, the tool is guarded. MCP clients select one of
the advertised roles with `arguments.role`. Direct API callers select a role
with `X-ERPBridge-Role`. The caller must have a verified identity and the
selected role must be present in both the identity and `allowedRoles`. The
server removes the MCP selector before executing the ERP request. A guarded
tool cannot define or require its own `role` argument.

When `allowedRoles` is absent, the tool is open and `role` remains an ordinary
business argument. In HTTP open-auth mode, a guarded tool still denies calls
because no caller identity is available. This also applies to guarded tools
over stdio.

---

## 📝 Annotated Example

```yaml
apiVersion: erpbridge.io/v1
kind: MCPTool
metadata:
  name: create_purchase_invoice
  version: 1.0.0
  module: finance
spec:
  description:
    short: "Create a new purchase invoice draft."
    whenToUse:
      - "User wants to record a new supplier bill"
      - "Adding an invoice to the finance module"
  inputSchema:
    type: object
    properties:
      supplier:
        type: string
        description: "The name of the vendor"
      amount:
        type: number
        description: "Total invoice amount"
    required: ["supplier", "amount"]
  execution:
    type: http
    method: POST
    endpoint: "/api/resource/Purchase Invoice"
    responsePath: "data" # ERP returns { "data": { ... } }, we only want the inner object
  security:
    authType: api-key
    credentialRef: ERP_FINANCE_KEY # Logical reference; resolves from the environment by default
    # credentialSource: file # Reads ERPBRIDGE_CREDENTIALS_DIR/ERP_FINANCE_KEY
    allowedRoles: [finance_reader, finance_writer]
  cache:
    enabled: false # Don't cache write operations
    flushOn: ["list_purchase_invoices"] # Flush list cache when a new one is created
```

Generated GET and HEAD tools default to a shared read-only cache with a
five-minute TTL. Generated write methods default to `enabled: false`; review
and change these values only when the operation is safe to cache. Generated
tools also include method-based annotation hints for all recognized HTTP
methods. Unknown methods retain only the HTTP-backed open-world hint.

### Generated drafts and reviewed manifests

Generation is pure: `bridgectl tool generate` writes the manifest only to its
selected output stream. The generator has an explicit `Save` method for callers
that intentionally persist a single JSON tool, but normal onboarding must not
use that seam. Keep reviewed manifests under `manifests/<module>/`; do not
create competing generated JSON files or use `schemas/` as a generated source.

---

## 🚀 Transitioning from V1

If you have old schemas, use `bridgectl tool generate` to convert them, or manually update the following:

1. Wrap the schema in `spec`.
2. Move `name`, `version`, `module` to `metadata`.
3. Separate `endpoint` into `execution` and `security`.
4. Ensure naming is lowercase and uses underscores.
