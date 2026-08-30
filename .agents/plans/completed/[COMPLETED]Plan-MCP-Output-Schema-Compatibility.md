# Plan: MCP Output-Schema Compatibility

## Goal

Stop ERPBridge from advertising malformed MCP `outputSchema` values, repair the
live `list_purchase_orders_api_resource_purchase_order_get` resource, and keep
OpenAPI generation from producing an empty output schema again.

## Current State

- The deployed raw `tools/list` payload includes
  `list_purchase_orders_api_resource_purchase_order_get` with `outputSchema: {}`.
  MCP Inspector rejects it because `outputSchema.type` is required, then drops
  the entry.
- The reviewed source has that same empty schema
  (`/home/nimendra/ERPBridge-Demo/manifests/erp/list_purchase_orders_api_resource_purchase_order_get.yaml:20`).
- `responseSchemaForOperation` returns every resolved OpenAPI response schema,
  including an untyped empty object (`internal/idp/generator.go:630-663`).
- `RegisterTool` blindly copies any persisted output schema to
  `RawOutputSchema` for MCP discovery (`internal/mcp/server.go:615-622`).
  Runtime result validation may still use an empty schema as an unconstrained
  JSON Schema (`internal/mcp/tool.go:567-604`).

## Decisions

1. Omit an output schema from MCP discovery unless it is an object with a
   non-empty top-level string `type`. This preserves runtime behavior and
   prevents one invalid persisted resource from poisoning strict client
   discovery.
2. OpenAPI generation treats a missing top-level response type as no
   discoverable output contract, so it emits no `spec.outputSchema`.
3. Remove the empty field from the reviewed Purchase Order manifest and reapply
   only that exact resource to `erpbridge-demo`. No credential, endpoint, or
   execution setting changes.

## Scope

In scope: generator, MCP wire projection, regression tests, the one reviewed
manifest, live resource reapply, documentation, and changelog.

Out of scope: changing tool invocation, arbitrary JSON Schema validation,
other tool manifests, protocol versions, or credential handling.

## Tasks

- [x] Add failing generator and MCP-registration tests for an untyped empty
  response schema; then omit it from generated and advertised output contracts.
  **Seam:** `responseSchemaForOperation` → `RegisterTool` MCP serialization.
  **Files:** `internal/idp/generator.go`, `internal/idp/generator_test.go`,
  `internal/mcp/server.go`, `internal/mcp/tool_test.go`.
  **Verify:** `go test ./internal/idp ./internal/mcp -run 'OutputSchema|OpenAPI'`.
- [x] Remove the empty `outputSchema` from the exact reviewed Purchase Order
  manifest, validate it, apply it to `erpbridge-demo`, and verify raw discovery
  and MCP Inspector no longer report a malformed entry. **Seam:** reviewed YAML
  → control-plane apply → `tools/list`. **Files:**
  `/home/nimendra/ERPBridge-Demo/manifests/erp/list_purchase_orders_api_resource_purchase_order_get.yaml`.
  **Verify:** `bridgectl tool validate -f <manifest> --context erpbridge-demo`,
  `bridgectl tool apply -f <manifest> --context erpbridge-demo`, and Inspector
  `tools/list` reports no dropped malformed entry.
- [x] Document that generated output schemas are advertised only when they have
  a concrete top-level type; add an Unreleased fix note and update the public
  documentation in the paired repository. **Files:** `docs/tool-schema.md`,
  `CHANGELOG.md`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/docs/erpbridge/tool-schema.mdx`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/CHANGELOG.md`.
  **Verify:** `npm --prefix /home/nimendra/Documents/Projects/erpbridge-docs run build`.

## Verification

- An OpenAPI `200` response schema of `{}` generates no output schema.
- A legacy persisted tool with `outputSchema: {}` remains executable but is
  advertised without `outputSchema`.
- The repaired live tool remains present in `tools/list`; Inspector reports no
  malformed entries.
- Focused Go tests, `make test`, `git diff --check`, and the public docs build
  pass.
