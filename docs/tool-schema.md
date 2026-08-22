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

### `spec.inputSchema`
Standard JSON Schema defining the arguments. **Strict typing is mandatory.**
- Use `properties` to define fields and `required` to enforce them.
- **Avoid "filters" strings**: Break down complex query requirements into individual typed properties.

### `spec.execution`
Technical mapping to the ERP API.
- **`method`**: (String) `GET`, `POST`, `PUT`, `DELETE`.
- **`endpoint`**: (String) The ERP API URL path.
- **`mapping`**: (Map) Optional. Maps LLM arg names to ERP parameter names.
- **`responsePath`**: (String) JSON key to extract from the ERP response (e.g., `"data"` or `"message"`).

### `spec.security`
Authentication strategy.
- **`authType`**: `api-key`, `bearer`, or `basic`.
- **`credentialRef`**: The name of the environment variable containing the secret. **Never embed raw secrets here.**
- **`allowedRoles`**: (Optional array) Roles that may call this tool. Each role must match `[a-z][a-z0-9_-]{0,63}`; the list must contain unique values and no more than 32 roles.

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
    credentialRef: ERP_FINANCE_KEY # Resolves to ENV["ERP_FINANCE_KEY"]
    allowedRoles: [finance_reader, finance_writer]
  cache:
    enabled: false # Don't cache write operations
    flushOn: ["list_purchase_invoices"] # Flush list cache when a new one is created
```

---

## 🚀 Transitioning from V1
If you have old schemas, use `bridgectl tool generate` to convert them, or manually update the following:
1. Wrap the schema in `spec`.
2. Move `name`, `version`, `module` to `metadata`.
3. Separate `endpoint` into `execution` and `security`.
4. Ensure naming is lowercase and uses underscores.
