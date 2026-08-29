# Plan: Preserve API-Key Headers in MCP Tool Execution

## Goal

Make ERPBridge MCP tools send credentialed requests with the registered
upstream authentication header. In particular, data-transformation and
policy-gate tools must send `X-API-Key` instead of falling back to
`Authorization`, without storing or exposing API-key values. Preserve the
existing `Authorization` default for older tools and keep the API probe,
tool generation, reviewed manifests, documentation, and deployment behavior
consistent.

## Current State

- API registrations retain an `AuthHeader`, and the selected remote registry
  reports `X-API-Key` for both affected API registrations. The tool manifests
  retain only `authType`, `credentialRef`, and `credentialSource`
  (`/home/nimendra/ERPBridge-Demo/manifests/erp-data-transformation/getrecord.yaml:47-50`,
  `/home/nimendra/ERPBridge-Demo/manifests/policy-gate/policy-gate-evaluate.yaml:120-123`).
- The tool `Security` model has no auth-header field, and
  `prepareERPCall` populates only the authentication type and resolved key
  (`internal/mcp/tool.go:139-146`, `internal/mcp/tool.go:198-314`).
- The connector defaults an empty authentication header to `Authorization`
  (`internal/connector/client.go:245-266`). The existing connector seam already
  supports an explicit header and tests `X-API-Key`
  (`internal/connector/client_test.go:179-215`).
- The API probe accepts `AuthHeader` and assigns it after preparing the request,
  so API probes can reach an upstream endpoint with the correct header while
  normal tool execution cannot (`internal/mcp/api.go:29-43`,
  `internal/mcp/api.go:49-116`).
- Both simple and OpenAPI tool generation copy the API authentication type and
  credential reference but omit `api.AuthHeader`
  (`internal/idp/generator.go:41-93`, `internal/idp/generator.go:95-377`).
- Credential resolution is server-side and supports environment or explicitly
  file-backed references without falling back between sources
  (`internal/credentials/credentials.go:66-137`). No credential value belongs
  in a manifest, test, log, or diagnostic report.
- The affected manifests are locally valid, and unauthenticated direct probes
  to the data-transformation and policy endpoints return `401`; the remote
  server-side API probes return upstream `404`/`422` statuses rather than a
  credential-preparation failure. This separates the header propagation defect
  from missing environment variables. The initial policy `-32001` timeout is a
  separate latency/deadline concern; connector calls use a 30-second deadline
  (`internal/connector/client.go:35`, `internal/connector/client.go:298-301`),
  and the MCP transport also maps deadline expiry to its request-interrupted
  error (`github.com/mark3labs/mcp-go@v0.57.0/client/transport/streamable_http.go:1060-1090`).

## Decisions

1. **Store the upstream header name in each tool's security contract.** Add an
   optional `authHeader` field to `spec.security`, propagate it from API
   registrations during both generation paths, and use it when constructing
   `connector.AuthConfig`. Deriving the header from a later registry lookup is
   rejected because tool execution currently owns an independent persisted
   tool definition and the endpoint may be a full URL rather than an exact API
   registration identity.
2. **Preserve backward compatibility.** An omitted `authHeader` continues to
   use the connector's existing `Authorization` default. Existing tools and
   manifests therefore keep their current behavior until an explicit header is
   reviewed and applied.
3. **Keep authentication server-controlled.** `authHeader` is manifest
   metadata, not an input argument; the credential remains a resolved
   environment or mounted-file value. Generated caller headers must not be
   allowed to replace authentication or transport headers.
4. **Make API probes use the same security path.** Populate the tool security
   field before `prepareERPCall` and retain the existing body-free probe
   contract, so probe and real tool execution cannot diverge again.
5. **Update all tools sharing the affected registrations.** Add `X-API-Key` to
   the three data-transformation manifests and the three policy-gate manifests,
   not only the four reported failures. This prevents `policy-gate-actions` and
   `policy-gate-assist` from retaining the same latent defect.

## Scope

### In scope

- Server-side tool security schema and outbound authentication-header
  propagation.
- Simple and OpenAPI tool generation.
- Regression tests at the tool-preparation, generator, connector, and API-probe
  seams.
- Root documentation and Unreleased changelog updates.
- The six reviewed demo manifests under
  `/home/nimendra/ERPBridge-Demo/manifests/`.
- Public documentation synchronization in the separate
  `erpbridge-docs` checkout.
- Redeployment, manifest reapplication, and safe MCP verification.

### Out of scope

- Rotating, printing, or embedding any API key.
- Changing the downstream data-transformation or policy-gate services.
- Automatically retrying policy `POST` requests.
- Increasing the 30-second timeout unless a separate post-fix latency
  investigation proves it is required.
- Changing MCP negotiation, authorization roles, cache policy, or response
  schemas.

## Tasks

- [x] **Task 1: Add header propagation and regression coverage.** Extend
  `mcp.Security` with optional `AuthHeader`, pass it through
  `prepareERPCall`, copy it in both `Generator.Generate` and
  `Generator.GenerateFromOpenAPI`, and make `handleAPIProbe` populate the same
  field before request preparation. Add tests for explicit `X-API-Key`
  propagation, generated-tool propagation, API-probe propagation, and the
  unchanged `Authorization` fallback. Follow red-green-refactor: add the
  failing assertions first, then implement the smallest compatible change.
  **Files:** `internal/mcp/tool.go`, `internal/mcp/api.go`,
  `internal/idp/generator.go`, `internal/mcp/tool_test.go`,
  `internal/mcp/api_test.go`, `internal/idp/generator_test.go`,
  `internal/connector/client_test.go` (only if the existing connector seam
  needs an additional assertion). **Seam:** `Tool.prepareERPCall` →
  `connector.EndpointConfig.Auth.Header`, with the existing `httptest` client
  seam confirming the actual outbound header. **Verify:**
  `go test ./internal/mcp ./internal/idp ./internal/connector -count=1` and
  `go test ./...`.

- [x] **Task 2: Document and review the persisted contract.** Add the optional
  `spec.security.authHeader` field to the root tool-schema and API guidance,
  state that omitted values preserve `Authorization`, and state that the field
  contains only a header name while credentials remain references. Add the
  Unreleased changelog entry. Update all six demo manifests with
  `authHeader: X-API-Key`, validate each file, and preserve the existing
  versions, endpoints, mappings, schemas, cache settings, and credential
  references. **Files:** `docs/tool-schema.md`, `docs/api.md`, `CHANGELOG.md`,
  `/home/nimendra/ERPBridge-Demo/manifests/erp-data-transformation/getrecord.yaml`,
  `/home/nimendra/ERPBridge-Demo/manifests/erp-data-transformation/getrepresentation.yaml`,
  `/home/nimendra/ERPBridge-Demo/manifests/erp-data-transformation/search.yaml`,
  `/home/nimendra/ERPBridge-Demo/manifests/policy-gate/policy-gate-actions.yaml`,
  `/home/nimendra/ERPBridge-Demo/manifests/policy-gate/policy-gate-assist.yaml`,
  `/home/nimendra/ERPBridge-Demo/manifests/policy-gate/policy-gate-evaluate.yaml`.
  **Seam:** reviewed YAML → local validator → persisted tool security
  contract. **Verify:** run `bridgectl tool validate -f` for each manifest with
  the explicit `erpbridge-demo` context, assert each contains the expected
  header name and no credential value, then run `git diff --check`.

- [x] **Task 3: Synchronize public documentation.** Mirror the root schema and
  API guidance in the public documentation checkout, preserving unrelated
  pending edits. Include the explicit-header example and the backward-
  compatible default, without adding a secret or changing the public API
  contract beyond the new optional field. **Files:**
  `/home/nimendra/Documents/Projects/erpbridge-docs/docs/erpbridge/tool-schema.mdx`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/docs/erpbridge/api.mdx`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/CHANGELOG.md`.
  **Seam:** root developer schema → public user-facing schema. **Verify:**
  compare the relevant wording and run `npm run build` from
  `/home/nimendra/Documents/Projects/erpbridge-docs`.

- [ ] **Task 4: Apply, redeploy, and verify the live behavior.** Build and
  deploy the server revision containing the source change, then apply the six
  reviewed manifests only after the exact target and affected tools are
  confirmed. Read back the six tool definitions and confirm `authHeader` is
  `X-API-Key`. Run server-side API probes, discover the tools through MCP, and
  make safe read-only calls using synthetic/non-sensitive inputs. Confirm the
  three data-transformation calls and all three policy calls no longer return
  unauthorized header errors; record the policy latency and determine whether
  the timeout persists after authentication is fixed. Inspect bounded logs and
  metrics for request status and timing only, with no credentials or upstream
  bodies. **Files:** deployment configuration only if the existing deployment
  requires a source revision update; no new credential files. **Seam:**
  deployed MCP `tools/call` → outbound upstream request. **Verify:**
  `make build`, `make test`, focused lint, authenticated `bridgectl tool get`
  readback, MCP discovery/calls, and bounded log/metric inspection. If the
  policy call still exceeds 30 seconds with a successful `X-API-Key` header,
  open a separate timeout plan rather than changing retry or timeout behavior
  in this fix.

## Verification

- A tool generated from an API with `authHeader: X-API-Key` constructs an
  outbound request with `X-API-Key` and does not add an unintended
  `Authorization` header.
- A tool with no `authHeader` retains the existing `Authorization` behavior.
- API probes and normal tool execution use the same header propagation path.
- All six reviewed manifests validate and contain only credential references,
  never credential values.
- `getrecord`, `getrepresentation`, `search`, `policy-gate-evaluate`,
  `policy-gate-actions`, and `policy-gate-assist` are active after apply and
  no longer fail solely because the upstream API-key header is missing.
- Existing auth, authorization, caching, MCP envelopes, retry safety, and
  response schemas remain unchanged.
- Root tests, build, focused lint, public documentation build, and
  `git diff --check` pass. Remote evidence contains only status, timing,
  resource names, and redacted diagnostics.

## Open Questions

None. The header must be an explicit optional tool-security field with a
backward-compatible `Authorization` default; changing downstream services or
adding a timeout change would create unrelated scope.
