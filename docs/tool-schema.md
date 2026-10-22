# MCP Tool Schema Reference (V2)

ToolPlane defines tools with a Kubernetes-style declarative resource format.
The format supports tool versions and intent-based descriptions. It separates
the tool contract from the underlying ERP structure.

---

## 🏗 Resource Structure

A tool definition has four main sections: `apiVersion/kind`, `metadata`, `spec`, and `status` (internal only).

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

Identity and grouping information. On first apply, ToolPlane canonicalizes the executable resource and records `resourceDigest` plus server-authored `admission` bookkeeping. The SHA-256 digest includes the API version, kind, identity, descriptive contract, schemas, policy, and execution mapping. It excludes runtime `status`, `isActive`, `isServing`, the digest itself, and review bookkeeping. The first active version becomes serving. Applying an admitted version with `metadata.isServing: true` moves the unqualified-name pointer without changing either revision's executable digest. Reapplying identical content is idempotent. Applying different executable content with the same name and version returns `REGISTRY_CONFLICT`; use a new version and review it before applying.

- **`name`**: (String) Unique identifier. Use **intent-based names** (e.g., `list_employees`) instead of technical ones (e.g., `get_resource_employee`).
- **`version`**: (String) SemVer version (e.g., `1.0.0`).
- **`module`**: (String) Logical grouping for access control and organization.

### `spec.description`

Information that helps the LLM select the correct tool.

- **`short`**: (String) Concise summary of what the tool does.
- **`whenToUse`**: (Array) List of scenarios where this tool is appropriate.
- **`whenNotToUse`**: (Array) List of similar scenarios where you must not use this tool.
- **`examples`**: (Array) Sample user queries that trigger this tool.

`bridgectl tool generate` derives `whenToUse` and `examples` from the
OpenAPI operation summary or description. Treat these values as draft evidence.
Review them before applying the manifest.

### `spec.annotations`

Optional MCP behavioral hints. These values are additive and do not change the
input or output schema.

- **`title`**: (String, optional) Human-readable display title.
- **`readOnlyHint`**: (Boolean, optional) Whether the tool is expected not to modify its environment.
- **`destructiveHint`**: (Boolean, optional) Whether a modifying operation may be destructive.
- **`idempotentHint`**: (Boolean, optional) Whether repeated calls with the same arguments are expected to have no additional effect.
- **`openWorldHint`**: (Boolean, optional) Whether the tool interacts with external entities.

Generated annotations are method-based draft hints. Review them before
applying a generated manifest. They do not replace authorization or other
server-side controls.

### `spec.rateLimit` and `spec.concurrency`

Optional execution protection for high-volume or expensive tools.

- **`rateLimit.requestsPerSecond`**: Positive finite per-tool token-bucket rate.
- **`rateLimit.burst`**: Positive per-tool token-bucket burst size.
- **`concurrency.limit`**: Positive maximum number of active calls.
- **`concurrency.perPrincipal`**: When `true`, apply the active-call limit per
  authenticated principal or MCP session; when `false`, share it across the
  tool. The global principal/session limiter remains in effect for every tool.

Unconfigured tools retain the server defaults. Side-effecting ERP methods are
not automatically retried.

### MCP discovery metadata

During MCP `tools/list`, ToolPlane projects existing guidance into namespaced
`_meta` keys:

- `io.erpbridge/whenToUse`
- `io.erpbridge/whenNotToUse`
- `io.erpbridge/examples`
- `io.erpbridge/allowedRoles`
- `toolplane.resourceDigest`
- `toolplane.resourceVersion`
- `toolplane.serving`

Every active revision has a protocol-safe exact name ending in
`.rev_<encoded-version>`. The `.rev_` marker is reserved. Declarative tool names
cannot contain it. By default, `tools/list` does **not** list exact names, so a
client sees one entry per tool. To list exact names, set
`MCP_ADVERTISE_REVISION_ALIASES=true`.

Discovery visibility does not change execution. An exact-name call or a call
bound with `params._meta.toolplane.resourceDigest` works with either setting.
These calls validate and execute the selected active revision. If that revision
is unavailable, they return a rediscovery-required error. They never select a
later revision. The runtime clones the resource before schema validation,
middleware, and execution. A serving-pointer change cannot change an in-flight
call. Cache entries use the resource digest as their revision scope.
Programmatic tools without a digest use the version.

The values are informational. `allowedRoles` describes the server's tool
allow-list; it does not grant access or replace server-side authorization. MCP
clients may show or ignore custom `_meta`, and they may not forward it to the
model context.

### `spec.inputSchema`

Standard JSON Schema defining the arguments. **Strict typing is mandatory.**

- Use `properties` to define fields and `required` to enforce them.
- Nested objects use nested `properties` and `required`; arrays use `items`.
- **Avoid "filters" strings**: Break down complex query requirements into individual typed properties.

### Enums and object boundaries

Enum values retain their JSON types. Use `enum: [0]` for an integer draft
status and `enum: [true]` for a boolean. Quoted values such as `"0"` and
`"true"` are strings, not substitutes for numbers or booleans. Existing
string enums keep their meaning.

Set `additionalProperties: false` on every object that must reject undeclared
fields, including nested objects and objects inside array `items`. If omitted, the schema
retains permissive legacy behavior. Explicit `true` permits extra fields.
A schema-valued `additionalProperties` validates each extra value against
that schema.

```yaml
inputSchema:
  type: object
  additionalProperties: false
  properties:
    docstatus:
      type: integer
      enum: [0]
    flags:
      type: object
      additionalProperties: false
      properties:
        approved:
          type: boolean
          enum: [true]
      required: [approved]
  required: [docstatus, flags]
```

OpenAPI generation preserves these constraints. When an object request body
is flattened into tool arguments, its extra-property policy applies to the
generated root object; declared path and query parameters remain valid.
Primitive and array bodies retain their constraints on the generated body
argument. Guarded MCP tools still advertise and validate the server-managed
`role` selector with a closed root object, then remove it before ERP execution.

Input validation uses the selected tool revision before middleware or outbound
ERP requests. Descriptions and `readOnlyHint` are guidance, not enforcement.
For example, a closed root rejects an undeclared legacy dispatcher such as
Frappe's `cmd` before the ERP can interpret it.

These constraints do not automatically close legacy manifests. They do not
repair enums that an older generator already stored as strings. Regenerate or edit the reviewed schemas. Declare their object boundaries.
Before changing the serving revision, admit a new version. Existing deployments need a separately reviewed
upgrade; installing source changes alone does not harden their stored tools.

### `spec.outputSchema`

Optional JSON Schema for a successful ERP response. When set, it must have a
non-empty top-level string `type`, such as `object` or `array`.

`bridgectl tool validate` rejects an untyped output schema. ToolPlane omits an
untyped legacy schema from MCP discovery. It still accepts any JSON result at
runtime. OpenAPI generation omits an output schema when the response has no
top-level type.

Output validation rejects responses that do not match the declared schema.
It does not remove extra fields to create a safe projection. If the response can contain private data, use a backend endpoint with fixed
approved fields or an explicit trusted projection.

### `spec.execution`

Technical mapping to the ERP API.

- **`method`**: (String) `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS`, or `TRACE`.
- **`endpoint`**: (String) The ERP API URL path.
- **`approvedOrigins`**: (Array) Exact normalized `scheme://host[:port]` values allowed after runtime endpoint rewriting. When omitted, apply binds the effective origin derived from `endpoint` and the current `ERP_BASE_URL`. A later rewrite to another origin fails before connector entry.
- **`mapping`**: (Map) Optional. Maps LLM argument names to ERP parameter names.
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
- **`authHeader`**: (Optional) The outbound HTTP header that carries the resolved credential, such as `X-API-Key`. When omitted, the connector uses `Authorization` for backward compatibility. This is a header name only; never put a credential value here.
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
    authHeader: X-API-Key # Optional; defaults to Authorization
    credentialRef: ERP_FINANCE_KEY # Logical reference; resolves from the environment by default
    # credentialSource: file # Reads ERPBRIDGE_CREDENTIALS_DIR/ERP_FINANCE_KEY
    allowedRoles: [finance_reader, finance_writer]
  cache:
    enabled: false # Don't cache write operations
    flushOn: ["list_purchase_invoices"] # Flush list cache when a new one is created
```

Generated GET and HEAD tools default to a shared read-only cache with a
one-minute TTL. Generated write methods default to `enabled: false`. Review these values.
Change them only when the operation is safe to cache. Generated
tools also include method-based annotation hints for all recognized HTTP
methods. Unknown methods retain only the HTTP-backed open-world hint.

### Generated drafts and reviewed manifests

`bridgectl tool generate` writes the manifest only to its selected output
stream. It does not save another copy. The generator has an explicit `Save` method for callers
that intentionally persist a single JSON tool, but normal onboarding must not
use that method. Keep reviewed manifests under `manifests/<module>/`. Do not create competing
generated JSON files. Do not use `schemas/` as a generated source.

---

## 🚀 Transitioning from V1

If you have old schemas, use `bridgectl tool generate` to convert them.
Alternatively, update them manually:

1. Wrap the schema in `spec`.
2. Move `name`, `version`, `module` to `metadata`.
3. Separate `endpoint` into `execution` and `security`.
4. Ensure naming is lowercase and uses underscores.
