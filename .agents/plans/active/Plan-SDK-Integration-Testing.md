# Plan: Harden ERPBridge SDK Integration Verification

## Goal

Make the governed workflow-engine integration prove the real ERPBridge contract.
The integration must use authenticated HTTP, discover the reviewed business
catalog, exercise at least one guarded tool, and fail closed on catalog or
policy drift.

This plan fixes the integration environment and test contract. It does not
change ERPBridge's documented open development mode by default.

## Current State

- ERPBridge accepts open HTTP requests when `API_AUTH_TOKEN` is unset or empty.
  `internal/mcp/auth.go:23-90` implements this behavior. The default Compose
  service does not set the variable (`docker-compose.yml:33-46`).
- ERPBridge supports `spec.security.allowedRoles`, role-selector injection, and
  role middleware (`internal/mcp/server.go:772-810`, `internal/mcp/authz.go:45-125`).
- The OpenAPI generator does not populate `AllowedRoles`
  (`internal/idp/generator.go:45-71,191-219`). Current generated schemas contain
  no guarded tools.
- The governed workflow registry contains 17 reviewed tools with role policy.
  The running ERPBridge MCP surface contains 14 managed tools plus two built-ins.
- The governed adapter compares MCP names and input schemas, but not roles or
  other policy metadata (`../temp/low-code-workflow-engine/backend-ts/src/tools/erpbridge-registry-compatibility.ts:13-65`).
- The adapter and ERPBridge unit tests cover local role behavior. They do not
  prove that a guarded tool exists in the live registry.
- The SDK already supports Streamable HTTP bearer authentication and REST
  registry access (`../erpbridge-sdk/src/mcp.ts:37-92`, `src/registry.ts:39-99`).
- The active agent-integration plan has SDK verification work still pending.
  This plan supersedes that plan's live verification task for this scope. Do
  not execute both plans concurrently.

Evidence: `.agents/plans/upcoming/rca-sdk-integration-testing.md`.

## Decisions

1. **Keep development open mode.** Do not change the default `docker-compose.yml`
   or make `API_AUTH_TOKEN` mandatory for ordinary local development. Require a
   non-empty token in the dedicated integration deployment and in production
   deployment checks. This preserves the existing backward-compatible contract.
2. **Use the 17 governed tools as the integration source of truth.** Do not
   rename the reviewed tools to match the current 14-tool ERP catalog. Render a
   temporary ERPBridge manifest from the governed registry so names, input
   schemas, and role policy stay under one source.
3. **Use canonical remote roles.** Convert local labels such as `Platform Admin`
   and `Workflow Builder` to server-valid roles such as `platform_admin` and
   `workflow_builder`. Store only canonical roles in ERPBridge
   `allowedRoles`. Compare canonical values on both sides.
4. **Do not infer authorization from descriptions.** Add an explicit
   `x-erpbridge-allowed-roles` OpenAPI extension for generated tools. A sentence
   such as “requires finance_editor” is not authorization metadata.
5. **Use a safe, non-mutating integration endpoint.** Render the 17 tools to a
   mock-ERP echo endpoint. Use `demo.echo` for live role calls. Verify that the
   role selector is removed before the downstream request.
6. **Treat built-ins as a separate system surface.** The compatibility gate must
   allow exactly `system.progress_test` and `system.sensitive_log_test`, report
   them separately, and fail on any unexpected system tool.
7. **Compare both protocol planes.** Use the SDK MCP client for the client-visible
   tool surface. Use the SDK REST registry client with an admin credential for
   stored role and schema metadata. Do not give the workflow runtime an admin
   credential.
8. **Use a fresh integration database.** Do not prune or mutate a shared
   production registry as part of the integration run. Start with an isolated
   database, apply the exact rendered manifest, read it back, and tear it down.
9. **Keep SDK source changes out of scope initially.** Add or extend integration
   tests only. Change SDK source only if the live contract proves an SDK defect.

## Scope

### In scope

- A deterministic governed-registry-to-ERPBridge test manifest renderer.
- A safe mock-ERP echo endpoint for guarded live calls.
- Explicit OpenAPI role metadata support and focused generator tests.
- An auth-enabled, isolated integration deployment and preflight.
- Dual-plane registry compatibility checks with explicit built-in handling.
- Live authentication, role-allow, role-denial, and downstream-argument tests.
- Cross-repository test commands and synchronized documentation.

### Out of scope

- Changing the default open development mode.
- OAuth, dynamic client registration, or token refresh.
- A general production registry migration or automatic production pruning.
- Replacing the 17 governed tools with the current 14 generated ERP tools.
- Changes to the public SDK runtime API unless a live test proves a defect.
- Hard deletion of registry data during an integration run.

## Tasks

- [ ] **Task 1: Render the governed catalog into an isolated ERPBridge fixture.**
  Read `../temp/low-code-workflow-engine/backend-ts/configs/runtime/all_tools_master_registry.json`
  and render all 17 entries as `erpbridge.io/v1` `MCPTool` documents. Preserve
  exact names and versions. Copy `input_schema` without adding the authorization
  selector. Canonicalize every `allowed_roles` value. Reject invalid or
  unmappable roles before any HTTP request. Do not commit generated schema
  output.

  Consume the shared authenticated Mock ERP fixture owned by
  `../completed/[COMPLETED]Plan-Mock-ERP.md`: use `/api/integration/echo` for rendered tools
  and its readback endpoint for downstream assertions. Complete that fixture
  plan before applying this task's manifest.

  **Seam:** governed registry JSON → renderer output → `bridgectl tool apply`
  → ERPBridge SQLite store → MCP `tools/list`.

  **Files:**
  `../temp/low-code-workflow-engine/backend-ts/scripts/render-erpbridge-manifest.mjs` (new),
  `../temp/low-code-workflow-engine/backend-ts/tests/render-erpbridge-manifest.test.ts` (new).

  **Verify:**

  ```bash
  cd ../temp/low-code-workflow-engine/backend-ts
  npm test -- tests/render-erpbridge-manifest.test.ts
  node scripts/render-erpbridge-manifest.mjs --output /tmp/erpbridge-tools.json
  node -e 'const fs=require("fs"); const x=JSON.parse(fs.readFileSync("/tmp/erpbridge-tools.json")); if(x.length!==17) process.exit(1); console.log("17 tools")'
  ```

- [ ] **Task 2: Preserve explicit roles through OpenAPI generation.** Add support
  for `x-erpbridge-allowed-roles` on an OpenAPI operation. Copy the extension
  into `Tool.Spec.Security.AllowedRoles`. Leave the list empty when the
  extension is absent. Do not parse role names from summaries or descriptions.
  Validate canonical roles, duplicates, empty entries, and the 32-role limit
  through the existing admission rules.

  Add one guarded operation to the shared OpenAPI document owned by
  `../completed/[COMPLETED]Plan-Mock-ERP.md` so the generator path has a real
  example. Keep the hand-rendered 17-tool integration fixture as the source
  for the governed catalog.

  **Seam:** OpenAPI operation → `GenerateFromOpenAPI` → `MCPTool` JSON → server
  admission.

  **Files:**
  `internal/idp/generator.go`, `internal/idp/generator_test.go`,
  `internal/mcp/server_test.go`, `docs/tool-schema.md`.

  **Verify:**

  ```bash
  go test ./internal/idp ./internal/mcp -run 'Test(Generator|ValidateTool|SchemaForMCP)'
  ```

- [ ] **Task 3: Add an authenticated isolated integration deployment.** Add a
  Compose override that requires `ERPBRIDGE_TEST_ADMIN_TOKEN`, sets
  `API_AUTH_TOKEN`, uses a unique database volume, and points to the mock ERP.
  Keep the default Compose file unchanged. Add a preflight that checks health,
  anonymous protected MCP access, invalid-token access, wrong-scope access, and
  valid scoped-token access. The preflight must fail when authentication is
  disabled. It must never print token values.

  Provision admin, scoped MCP, and wrong-scope tokens outside source control.
  The workflow runtime uses only the scoped MCP token. Use the admin token only
  for manifest application and REST registry readback.

  **Seam:** actual `erpbridge-server` process and Streamable HTTP boundary.

  **Files:**
  `docker-compose.integration.yml` (new), `scripts/verify-sdk-integration.sh` (new),
  `Makefile`, `docs/docker.md`, `docs/environment-variables.md`,
  `docs/tokens.md`.

  **Verify:**

  ```bash
  : "${ERPBRIDGE_TEST_ADMIN_TOKEN:?Set this variable outside source control}"
  docker compose -p erpbridge-sdk-integration \
    -f docker-compose.yml -f docker-compose.integration.yml up -d --build
  ./scripts/verify-sdk-integration.sh
  docker compose -p erpbridge-sdk-integration \
    -f docker-compose.yml -f docker-compose.integration.yml down -v
  ```

  The script must assert these results: health `200`, anonymous MCP `401`,
  invalid bearer `401`, wrong-scope bearer `403`, and valid scoped MCP
  initialization `200` with `Mcp-Session-Id`.

- [ ] **Task 4: Make registry compatibility compare the complete contract.**
  Extend the governed verification path to fetch both surfaces through
  `@erpbridge/sdk`: `mcp.listTools()` with the scoped token and
  `registry.list()` with the admin token. Compare exact business names,
  canonical input schemas, `allowed_roles` against
  `spec.security.allowedRoles`, and the MCP-injected role enum. Strip the
  injected role property before comparing business input schema, then compare
  the role enum separately.

  Handle built-ins with an explicit allow-list. Return separate fields for
  missing tools, schema mismatches, role mismatches, unreviewed business tools,
  duplicate names, and unexpected system tools. Preserve fail-closed behavior.

  **Seam:** SDK MCP discovery plus SDK REST registry readback.

  **Files:**
  `../temp/low-code-workflow-engine/backend-ts/src/config/config.ts`,
  `../temp/low-code-workflow-engine/backend-ts/tests/config.test.ts`,
  `../temp/low-code-workflow-engine/backend-ts/src/tools/erpbridge-registry-compatibility.ts`,
  `../temp/low-code-workflow-engine/backend-ts/tests/erpbridge-registry-compatibility.test.ts`,
  `../temp/low-code-workflow-engine/backend-ts/scripts/verify-erpbridge-registry.mjs`.

  **Verify:**

  ```bash
  cd ../temp/low-code-workflow-engine/backend-ts
  npm test -- tests/erpbridge-registry-compatibility.test.ts tests/config.test.ts
  export ERPBRIDGE_BASE_URL="${ERPBRIDGE_BASE_URL:-http://localhost:8080}"
  : "${ERPBRIDGE_MCP_TOKEN:?Set this variable outside source control}"
  : "${ERPBRIDGE_ADMIN_TOKEN:?Set this variable outside source control}"
  export ERPBRIDGE_ROLE_MAP='{"Platform Admin":"platform_admin","Workflow Builder":"workflow_builder","Client":"client"}'
  MCP_TRANSPORT=erpbridge-mcp npm run verify:erpbridge-registry
  ```

  The compatible result must contain zero missing, schema, role, unreviewed,
  duplicate, and unexpected-system entries.

- [ ] **Task 5: Run the live authentication and role matrix through the SDK.**
  Add a live integration test in the governed adapter. Use `demo.echo` and a
  scoped token containing `workflow_builder`.

  Cover:

  1. Anonymous protected MCP initialization returns `401`.
  2. An invalid bearer returns `401`.
  3. A token without `mcp` scope returns `403`.
  4. A valid scoped token lists 17 governed tools and the two approved built-ins.
  5. A valid `workflow_builder` selector succeeds for `demo.echo`.
  6. A missing selector returns an MCP tool error.
  7. A role absent from the token returns an MCP tool error.
  8. A role absent from the tool allow-list returns an MCP tool error.
  9. The mock ERP echo payload contains no `role` field.
  10. The existing adapter unit test rejects a workflow-supplied role before
      the SDK call.

  Keep the SDK result-envelope assertions intact. A role denial from a guarded
  MCP tool is an MCP error result, not an HTTP authentication failure.

  **Seam:** real `@erpbridge/sdk` client and real ERPBridge MCP server, with the
  mock ERP echo endpoint as the downstream observation point.

  **Files:**
  `../temp/low-code-workflow-engine/backend-ts/tests/erpbridge-live.integration.test.ts` (new),
  `../temp/low-code-workflow-engine/backend-ts/package.json`,
  `../erpbridge-sdk/tests/integration/README.md`.

  **Verify:**

  ```bash
  cd ../temp/low-code-workflow-engine/backend-ts
  npm run test:erpbridge-live
  npm test -- tests/erpbridge-mcp-client.test.ts

  cd ../erpbridge-sdk
  ERPBridge_TEST_SERVER=http://localhost:8080 npm run test:mcp-compat
  ERPBridge_TEST_SERVER=http://localhost:8080 npm run test:integration
  npm test
  npm run build
  ```

- [ ] **Task 6: Synchronize documentation and release checks.** Document the
  authenticated integration profile, canonical role mapping, 17-tool source of
  truth, built-in handling, dual-plane compatibility check, safe fixture, and
  cleanup procedure. Keep the RCA as historical evidence and add a separate
  runbook for execution.

  Create the required matching documentation-repository plan before editing
  `erpbridge-docs`. Keep the server repository as the source of truth.

  **Files:**
  `docs/sdk-integration-testing.md` (new), `docs/connectivity.md`,
  `docs/agent-integrations.md`, `docs/tool-schema.md`,
  `docs/environment-variables.md`, `CHANGELOG.md`,
  `../temp/low-code-workflow-engine/backend-ts/docs/ERPBRIDGE_INTEGRATION.md`,
  `../erpbridge-docs/docs/erpbridge/tool-schema.mdx`,
  `../erpbridge-docs/docs/erpbridge/connectivity.mdx`,
  `../erpbridge-docs/docs/erpbridge/environment-variables.mdx`,
  `../erpbridge-docs/CHANGELOG.md`, and a plan file in
  `../erpbridge-docs/.agents/plans/`.

  **Verify:**

  ```bash
  go test ./...
  make test
  cd ../temp/low-code-workflow-engine/backend-ts && npm test && npm run build
  cd ../erpbridge-sdk && npm test && npm run build
  cd ../erpbridge-docs && npm run build
  ```

## Verification

The plan is complete only when all of these conditions hold:

- The default development deployment remains unchanged and documented as open.
- The integration deployment fails closed when `API_AUTH_TOKEN` is absent.
- Anonymous and invalid protected requests return `401`.
- A wrong-scope token returns `403`.
- A valid scoped token completes MCP initialization and discovery.
- The isolated ERPBridge registry contains exactly the 17 governed business
  tools. The MCP surface also contains exactly the two approved built-ins.
- The compatibility report has no missing, unreviewed, duplicate, schema, role,
  or unexpected-system entries.
- At least one guarded tool succeeds for an allowed verified role.
- Missing, wrong, and disallowed roles fail before downstream ERP execution.
- The downstream echo payload has no authorization selector.
- The workflow adapter rejects a workflow-supplied role before the SDK call.
- ERPBridge, the governed adapter, the SDK, and the documentation site pass
  their focused quality gates.
- No credential value appears in source control, test output, reports, or logs.

## Open Questions

None for the recommended integration-only scope. A change that makes
`API_AUTH_TOKEN` mandatory for every default deployment is a separate
backward-compatibility decision and requires a revised plan.
