# Plan: Expose MCP Tool Hints and `_meta`

## Goal

Expose useful tool guidance to MCP clients without changing the MCP protocol version, the existing `erpbridge.io/v1` manifest version, or the tool input/output schemas.

The plan adds an optional `spec.annotations` field to generated ERPBridge tool manifests, maps existing guidance fields to namespaced MCP `_meta`, and includes non-empty `security.allowedRoles` as informational metadata for host/model reference.

## Evidence and Current State

- `internal/mcp.Description` already stores `whenToUse`, `whenNotToUse`, and `examples`.
- OpenAPI generation has two paths: `Generator.Generate` and `Generator.GenerateFromOpenAPI` (`internal/idp/generator.go:42-375`). Both must be covered.
- `internal/mcp.Security` already stores `AllowedRoles`.
- `Server.RegisterTool` currently forwards only the tool name, short description, input schema, and output schema to the `mcp-go` tool (`internal/mcp/server.go:535-590`). It does not map guidance, roles, or annotations.
- Pinned `mcp-go v0.57.0` supports `Tool.Meta`, `Tool.Title`, `Tool.Annotations`, and pointer-based annotation hints (`mcp/tools.go:628-647,818-827`). Its custom marshaler serializes the `annotations` member unconditionally from its value field, so an empty wire `annotations: {}` is expected when no values are configured.
- The MCP specification defines annotations as optional behavioral hints. The MCP Tool Annotations article explains that they are untrusted and are not enforcement contracts: https://blog.modelcontextprotocol.io/posts/2026-03-16-tool-annotations/
- Current role authorization remains server-side. `allowedRoles` is also represented in the role-selection path, so `_meta` is a discovery duplicate, not a new authorization mechanism.

## Proposed Manifest Shape

Add this optional field under `spec`:

```yaml
spec:
  annotations:
    title: List orders
    readOnlyHint: true
    destructiveHint: false
    idempotentHint: true
    openWorldHint: true
```

Existing manifests can omit `spec.annotations`.

The existing fields remain the source for model guidance:

```yaml
spec:
  description:
    short: List customer orders
    whenToUse:
      - When the user asks to find or review orders
    whenNotToUse:
      - When the user wants to create or update an order
    examples:
      - Show my recent orders
  security:
    allowedRoles:
      - sales.read
      - sales.manager
```

No new arbitrary metadata map is added in this iteration. The existing description and security fields are projected into `_meta`.

## MCP Projection

The MCP `tools/list` tool definition will contain:

```json
{
  "name": "list-orders",
  "title": "List orders",
  "description": "List customer orders",
  "inputSchema": {},
  "annotations": {
    "title": "List orders",
    "readOnlyHint": true,
    "destructiveHint": false,
    "idempotentHint": true,
    "openWorldHint": true
  },
  "_meta": {
    "io.erpbridge/whenToUse": [
      "When the user asks to find or review orders"
    ],
    "io.erpbridge/whenNotToUse": [
      "When the user wants to create or update an order"
    ],
    "io.erpbridge/examples": [
      "Show my recent orders"
    ],
    "io.erpbridge/allowedRoles": [
      "sales.read",
      "sales.manager"
    ]
  }
}
```

Only non-empty metadata values are emitted. Endpoints, credential references, auth values, mappings, cache settings, and other internal configuration are never copied into `_meta`.

`_meta` is visible in the MCP `tools/list` response, but custom `_meta` keys are not guaranteed to be forwarded into the model context by every MCP client. If guaranteed model visibility becomes a requirement, the host-facing description projection needs a separate design; `_meta` alone must not be treated as a reliable model instruction channel.

## Decisions

- Add `ToolSpec.Annotations *ToolAnnotations ` + "`json:\"annotations,omitempty\"`" + ` to make the whole manifest object optional. Pointer booleans inside `ToolAnnotations` preserve explicit `false` values and distinguish them from omitted values.
- The annotation type mirrors standard MCP fields: `title`, `readOnlyHint`, `destructiveHint`, `idempotentHint`, and `openWorldHint`.
- Project `spec.annotations.title` to both the top-level MCP `Tool.Title` and `Tool.Annotations.Title`, because clients may prefer the top-level display title.
- Do not modify `InputSchema` or `OutputSchema`.
- Do not change MCP protocol negotiation or `apiVersion: erpbridge.io/v1`.
- Use one `annotationsForHTTPMethod` helper from both `Generator.Generate` and `Generator.GenerateFromOpenAPI`.
- Generate conservative, reviewable method defaults for recognized methods:
  - `GET`, `HEAD`, `OPTIONS`, and `TRACE`: `readOnlyHint=true`, `destructiveHint=false`, `idempotentHint=true`.
  - `PUT`: `readOnlyHint=false`, `destructiveHint=true`, `idempotentHint=true`.
  - `DELETE`: `readOnlyHint=false`, `destructiveHint=true`, `idempotentHint=true`.
  - `POST` and `PATCH`: `readOnlyHint=false`, `destructiveHint=true`, `idempotentHint=false`.
  - Unknown methods: leave method-specific hints nil; retain `openWorldHint=true` only when the tool is known to be HTTP-backed.
- Treat generated method defaults as hints requiring review. HTTP method conventions do not prove endpoint behavior; an ERP endpoint can violate them.
- Map `description.whenToUse`, `description.whenNotToUse`, and `description.examples` to:
  - `io.erpbridge/whenToUse`
  - `io.erpbridge/whenNotToUse`
  - `io.erpbridge/examples`
- Map non-empty `security.allowedRoles` to `io.erpbridge/allowedRoles` as a server-published discovery hint for a host or model. Authorization is determined only from the authenticated caller identity, selected role, and server-side manifest. A model or MCP client must never self-authorize from this value.
- Build `_meta` from an allowlist and assign nil when there are no values. Do not expose arbitrary fields.
- Map explicit annotations at registration. Generated manifests receive method-derived annotations; legacy manifests without the field do not receive inferred annotations unexpectedly.

## Backward Compatibility

- Existing manifests without `spec.annotations` remain valid and preserve current execution behavior.
- New manifests with `spec.annotations` are accepted by the current non-strict tool JSON/YAML decoding paths. An older ERPBridge server ignores the new field and still executes the tool, but cannot expose the new hints.
- Verify compatibility through CLI decode/apply, control-plane apply, SQLite save/load, reconciliation, and MCP `tools/list`; a standalone unmarshal test is insufficient.
- The pinned SDK emits `annotations: {}` even when the manifest has no annotation values. Acceptance tests must not require the wire member to be absent; they must verify that configured values and explicit false values survive.
- New servers expose standard annotations and namespaced `_meta` as optional tool members. Clients that tolerate unknown optional members can continue using the existing name, description, input schema, and output schema. Custom `_meta` visibility in the model context is client-dependent.
- `allowedRoles` is additive informational output only. Existing authorization checks, role selection, and filtering remain unchanged.

## Scope

In scope:

- Add the optional `spec.annotations` manifest field.
- Emit method-derived annotations in both API and OpenAPI tool-generation paths.
- Map explicit annotations, existing description guidance, and allowed roles at MCP registration.
- Test the MCP `tools/list` boundary, Streamable HTTP/stdio filtering seams, persistence, reconciliation, and old-manifest compatibility.
- Document client visibility, model-context limitations, namespaced keys, generated defaults, and trust/enforcement limitations.
- Update `CHANGELOG.md` under Unreleased and the corresponding public documentation repository as required by `AGENTS.md`.

Out of scope:

- MCP version upgrades.
- Changes to input/output JSON schemas or manifest versioning.
- Arbitrary user-defined metadata fields or CLI flags.
- Server-side authorization, rate limiting, sandboxing, or other enforcement based on annotations or `_meta`.
- Automatic annotation inference for legacy manifests without `spec.annotations`.

## Atomic Tasks and Commits

Each implementation task follows Red → Green → Refactor and is one atomic Conventional Commit. Behavior-changing tasks include their required documentation and public-documentation sync in the same commit.

- [ ] Task 1 — Manifest field and generation: Add `ToolAnnotations` and optional `ToolSpec.Annotations`, preserving pointer booleans, explicit false values, and whole-object omission. Add one method-inference helper and call it from both `Generator.Generate` and `Generator.GenerateFromOpenAPI`. Cover all recognized methods (`GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS`, `TRACE`) and unknown methods. Update the canonical schema docs to document the recognized inference methods and unknown-method behavior, update relevant generator docs, and update `CHANGELOG.md` in the same commit. (**Files:** `internal/mcp/tool.go`, `internal/idp/generator.go`, their tests, `docs/tool-schema.md`, relevant CLI docs, `CHANGELOG.md`; **Paired public-doc deliverable:** update `~/Documents/Projects/erpbridge-docs/docs/erpbridge/tool-schema.mdx` and `~/Documents/Projects/erpbridge-docs/docs/bridgectl/bridgectl_tool_generate.md`, then create and record a corresponding Conventional Commit there; **Verify:** targeted `internal/mcp` and `internal/idp` tests plus both repository commit checks.)
- [ ] Task 2 — MCP projection: Map explicit annotations to `mcp-go` `ToolAnnotation`, project annotation title to top-level `Tool.Title`, and map allowlisted description guidance plus non-empty allowed roles to namespaced `mcp-go` `Meta`. Add the MCP-facing documentation, security/authorization caveat, and model-visibility caveat in the same commit, including the public SDK/client reference. (**Files:** `internal/mcp/server.go`, `internal/mcp/server_test.go`, `docs/onboarding.md`, `docs/tokens.md`, relevant CLI docs, `CHANGELOG.md`; **Paired public-doc deliverable:** update `~/Documents/Projects/erpbridge-docs/docs/sdk/mcp-tools.mdx` and `~/Documents/Projects/erpbridge-docs/docs/erpbridge/api.mdx`, then create and record a corresponding Conventional Commit there; **Verify:** MCP registration tests, documentation review, and explicit commit checks in both repositories.)
- [ ] Task 3 — End-to-end compatibility: Persist two tools in a file-backed SQLite flow: (a) an explicitly annotated tool with `destructiveHint: false`, guidance, and allowed roles, and (b) a legacy tool with `Annotations:nil`. Load both into a fresh server, reconcile, and assert title, annotations, allowlisted `_meta`, explicit false, and the expected empty-wire-annotations behavior separately through both Streamable HTTP and real stdio `tools/list` paths. Also verify old manifests without `spec.annotations` through CLI decode/apply and control-plane apply. Keep role filtering and authorization assertions unchanged. (**Files:** `internal/mcp/auth_test.go`, `internal/mcp/filter_writer` tests as appropriate, `internal/mcp/store_test.go`, `internal/mcp/api_test.go`, `internal/mcp/server_test.go`, `internal/cli/tool_test.go`; **Verify:** targeted end-to-end tests.)
- [ ] Task 4 — Full verification: Run repository verification and resolve findings before closing the plan. (**Verify:** `make test`, `golangci-lint run ./internal/mcp ./internal/idp`, and `lens_diagnostics mode=full`.)

## Acceptance Criteria

- Generated manifests may contain optional `spec.annotations`; old manifests remain valid.
- The whole `spec.annotations` object is omitted when nil; explicit false values are preserved.
- An MCP `tools/list` response contains configured standard annotations and namespaced `_meta` values.
- `_meta` includes `whenToUse`, `whenNotToUse`, `examples`, and non-empty `allowedRoles`; empty values are omitted.
- The SDK’s expected empty `annotations: {}` behavior is covered without being treated as a regression.
- The allowlist prevents endpoint, credential reference, auth value, mapping, cache policy, or secret disclosure.
- `allowedRoles` is documented and tested as a discovery hint only; server authorization remains authoritative.
- A configured tool survives save/load, fresh-server reconciliation, and both Streamable HTTP and stdio `tools/list` serialization paths.
- Old manifests continue through decode, apply, persistence, reconciliation, and registration.
- The implementation does not change MCP protocol negotiation, `erpbridge.io/v1`, input schemas, or output schemas.
- Documentation does not promise that arbitrary `_meta` is visible to the model in every MCP client.
- Task 3 uses separate annotated and legacy fixtures, proving both configured metadata and the SDK’s empty `annotations: {}` behavior after persistence, reconciliation, and transport serialization.
- Documentation-changing behavior Tasks 1 and 2 each record the paired ERPBridge and `erpbridge-docs` commits; Task 3 is test-only and has no documentation deliverable.
