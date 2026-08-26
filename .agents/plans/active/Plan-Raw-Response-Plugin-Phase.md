# Plan: Raw-Response Plugin Phase and Media Adaptation

## Goal

Add a `raw_response` plugin phase that can adapt an ERP HTTP response before
ERPBridge applies `responsePath` normalization and the tool's final output
schema. The phase must support media responses, including an image-to-text
plugin that exposes only text to MCP clients, while preserving the existing
`after_response` contract for tools without raw bindings.

A developer owns the final MCP tool contract: the plugin does not dynamically
change the schema advertised by MCP. When a response transformation changes the
meaning or type of a tool result, the developer publishes a new exact tool
version with the post-plugin output schema and binds the raw plugin to that
version.

## Current State

- `PluginBinding` accepts only `after_response`; its validation rejects every
  other phase (`internal/mcp/plugin.go:270-315`). The runtime stores all active
  bindings in one priority-sorted slice per exact tool (`internal/mcp/plugin_registry.go:94-139`).
- `PluginInvocation` requires a normalized JSON `result` and deliberately omits
  headers, credentials, caller identity, and original arguments
  (`internal/mcp/plugin.go:131-159`). `PluginClient.Process` JSON-encodes the
  invocation, caps each request and response at 1 MiB, disables redirects, and
  performs no retries (`internal/mcp/plugin_client.go:53-157`).
- `Tool.Execute` currently calls the ERP connector, JSON-decodes the response,
  applies `responsePath` only for 2xx responses, validates only successful
  results, and sets `IsError` for statuses at or above 400
  (`internal/mcp/tool.go:133-254`). A binary image fails at the JSON decoder
  before any plugin can run (`internal/mcp/tool.go:227-230`).
- The connector retries `429` and `5xx`, closes those response bodies, and
  returns an error after the final attempt (`internal/connector/client.go:168-196`).
  Therefore a raw phase cannot currently inspect all non-2xx responses merely
  by extracting code from `Tool.Execute`.
- The response pipeline exits before binding lookup when `ToolResult.IsError`
  is true (`internal/mcp/plugin_pipeline.go:31-49`). It otherwise chains
  normalized results, then revalidates the final transformed value
  (`internal/mcp/plugin_pipeline.go:53-100`).
- MCP serialization currently marshals only `result.Result` into `TextContent`
  and drops the internal `IsError` value (`internal/mcp/plugin_pipeline.go:22-28`).
  The direct endpoint also relies on the MCP result and maps an MCP error to a
  generic HTTP 500 (`internal/mcp/server.go:902-976`).
- Cache writes are gated by MCP-level `CallToolResult.IsError`, not the internal
  `ToolResult.IsError` (`internal/mcp/middleware.go:200-230`). Raw error handling
  must not create successful cache entries.
- `RegisterTool` creates an MCP tool with an input schema but explicitly clears
  structured output schema fields (`internal/mcp/server.go:515-534`). The
  internal `ToolSpec.OutputSchema` is therefore a validation contract, not
  currently an advertised MCP output contract.
- Raw plugin admission must account for the existing security boundary:
  `PLUGIN_ENDPOINT_ALLOWLIST` is required only when a plugin has configured
  authentication (`internal/mcp/plugin_api.go:193-214`). A raw plugin would
  otherwise receive unfiltered ERP data at an unauthenticated endpoint.
- MCP officially defines `outputSchema` as the expected structured tool output
  and requires tool execution errors to be represented with `isError: true`:
  <https://modelcontextprotocol.io/specification/2025-11-25/server/tools>.

## Decisions

1. **Call the phase `raw_response`, but define it as media-aware pre-normalization.**
   It runs after the ERP request completes and before `responsePath`. For JSON
   responses the plugin receives the decoded JSON value. For non-JSON responses
   it receives bounded bytes encoded as base64. The contract includes a
   normalized `contentType` but no ERP headers, URL, credentials, or response
   metadata beyond status and media type.

2. **Use a tagged raw-body contract.** A raw invocation contains:
   `status`, normalized `contentType`, and `body` with `encoding` plus `value`.
   `encoding` is `json` for a decoded JSON value or `base64` for binary bytes.
   JSON classification requires one complete JSON document; empty, malformed,
   and non-JSON bodies use base64 so a plugin can handle them explicitly. An
   absent or unparsable content type becomes `application/octet-stream`.
   The HTTP status is immutable; plugin responses can replace only the body.
   A plugin response continues to use `{"result": ...}`. When another raw
   plugin follows, that result becomes a JSON body with `application/json`
   content type while the original status remains unchanged.

3. **Keep protocol version `v1` and preserve existing after-response payloads.**
   `rawResponse` is optional. Normal invocations continue to contain the same
   `result` payload, including an explicit `result: null` when the legacy
   result is nil, and no `rawResponse`. Raw invocations omit `result` entirely.
   Use a custom invocation marshaler or equivalent presence-aware wire type so
   omission of `result` in the raw variant does not change the legacy null
   payload. Raw validation rejects a non-nil result alongside `rawResponse`;
   the legacy variant remains valid when its result is nil.

4. **Capture response bytes before decoding.** Add a bounded ERP response
   capture seam returning status, media type, and body bytes. Keep the existing
   `ERPConnector.Call` interface unchanged for `Resource.Execute` and legacy
   test connectors; add an additive `ERPResponseConnector` capability used only
   by the raw path. `Tool.Execute` continues to use a compatibility
   normalization path when no raw binding is active. The raw path opts into
   terminal HTTP response access. If a configured connector lacks the additive
   capability, the raw path fails closed rather than silently skipping the raw
   phase. Existing ERP retry and circuit-breaker behavior remains intact; a
   terminal `429` or `5xx` can be inspected without being mistaken for a
   successful connector call.

5. **Run phases in fixed order, with priority independent inside each phase.**
   The raw chain runs first in ascending priority and binding-name order. For a
   successful response, the resulting body then goes through `responsePath`,
   the tool output-schema gate, and the existing `after_response` chain. The
   after-response chain receives only the normalized `result` field exactly as
   before. Raw bindings never run after `after_response` bindings.

6. **Preserve success/error semantics explicitly.** Raw processing may inspect
   terminal non-2xx responses, but only successful 2xx responses go through
   `responsePath`, output-schema validation, and `after_response`. For raw-bound
   calls, every status outside 200–299, including 3xx, sets the error state;
   the plugin cannot clear it. A successful raw error chain may return a
   bounded JSON error value as MCP error content, but a failed raw chain never
   falls back to an unfiltered binary or ERP error body: `continue` returns a
   generic upstream error and `fail` returns the generic plugin error. Calls
   without raw bindings retain the current `status >= 400` behavior to avoid
   an unrelated compatibility change.

7. **Fallback must preserve the declared final schema.** On a raw plugin,
   normalization, or transformed-output validation failure, `continue` retries
   the default path from the original captured response. If that fallback
   cannot satisfy the tool's final output schema—as is expected when image
   bytes must become text—it returns a safe execution error rather than
   emitting incompatible binary data. `fail` always returns the existing
   generic plugin-processing error. Media-conversion bindings should use
   `failurePolicy: fail` unless their fallback is known to satisfy the final
   schema.

8. **The developer declares the final MCP output contract.** A raw-bound HTTP
   tool must have an explicit object-shaped `outputSchema`, because the current
   MCP specification defines `structuredContent` as a JSON object. For
   image-to-text, the tool's versioned schema is
   `{type: object, properties: {text: {type: string}}, required: [text]}`.
   The MCP result's text content contains the extracted text, while its
   structured content is the conforming `{text: ...}` object. The plugin does
   not supply or mutate the MCP schema at invocation time.
   ERPBridge advertises the declared schema through MCP and returns conforming
   structured content plus equivalent text content containing only the text.

9. **Strengthen raw-data admission.** A raw binding may be applied only when
   `API_AUTH_TOKEN` is configured and the request is authenticated as an admin.
   It also requires an active HTTP-backed tool with an explicit object-shaped
   output schema and a plugin endpoint present in `PLUGIN_ENDPOINT_ALLOWLIST`,
   regardless of whether plugin authentication is configured. Reconciliation
   must not activate a raw binding when the raw-data admission prerequisites
   are absent. Existing credentialed-plugin checks remain in force. Raw bodies
   are never logged or included in error messages.

10. **Bound media and keep the existing plugin safety controls.** The ERP body
    is limited to `MaxPluginJSONBytes` before decoding or base64 encoding. The
    base64 expansion and complete invocation envelope count toward the existing
    1 MiB plugin request limit; an oversized request is rejected before the
    plugin outbound call and follows the binding policy. Plugin timeout,
    no-retry, and no-redirect behavior remain unchanged. A raw-bound empty 2xx
    body can be adapted by a plugin; without a valid adapted result it produces
    a safe error. A raw-bound malformed JSON body is presented as base64. No
    arbitrary ERP headers are forwarded.

11. **Do not change schemas dynamically during a call.** The exact tool version
    is a control-plane identity; the current MCP registration exposes only
    `metadata.name` and resolves the latest active version
    (`internal/mcp/server.go:527-566`, `internal/mcp/registry.go:40-127`). For
    an incompatible output contract, the preferred migration creates a new
    MCP-visible tool name `read-invoice-text` with exact tool version `1.0.0`,
    so the original tool remains stable. Replacing the schema behind an
    existing MCP name is an explicit breaking migration that requires tool-list
    notification, client migration, and separate approval. Binding changes do
    not dynamically mutate a tool schema.

12. **Synchronize behavior documentation with each owning commit.** Tasks that
    change a user-visible contract include the relevant in-repository guide and
    Unreleased changelog entry in the same root commit. The public
    `../erpbridge-docs` mirror, its required plan, navigation, and changelog
    are updated in a separate documentation commit after the local contract is
    stable.

## Scope

### In scope

- `raw_response` binding validation, phase-aware runtime lookup, and exact
  mixed-phase ordering.
- Bounded JSON and binary ERP response capture with status and media type.
- Image/document-to-text plugin input and JSON result output.
- Terminal non-2xx response access while retaining ERP retry and
  circuit-breaker semantics.
- Final output-schema validation, MCP output-schema advertisement,
  structured-content generation, error propagation, and cache correctness.
- Safe raw-data admission, failure fallback, redaction, and observability.
- Unit, wire-contract, connector, MCP/direct, cache, and opt-in black-box tests.
- In-repository and public documentation, skill guidance, changelogs, and
  separate public-docs synchronization.

### Out of scope

- ERPBridge hosting, deploying, or managing plugin processes or LLM providers.
- Streaming media, multipart responses, audio/video conversion, or arbitrary
  binary plugin outputs. Raw input supports bounded binary-to-JSON adaptation;
  plugin responses remain JSON `result` values.
- Dynamic per-request MCP tool schemas or plugin-controlled tool registration.
- Forwarding ERP headers, URLs, cookies, credentials, or caller identity.
- Plugin retries, asynchronous processing, queues, or status mutation.
- Changes to tools without raw bindings except the deliberate MCP output-schema
  and error-propagation behavior required by the finalized contract.

## Tasks

- [x] **Task 1: Define the raw media wire contract and binding validation.** Write
  failing tests first. Add `PluginPhaseRawResponse`, a tagged raw-body type,
  `PluginRawResponse`, the optional `PluginInvocation.RawResponse` field, and
  mutually exclusive invocation validation while preserving existing
  after-response JSON. Accept both phases in `PluginBinding.Validate`. Require
  valid status/media encoding values and document that `PluginResponse.Result`
  replaces only the body. (**Seam:** `PluginInvocation` JSON boundary and pure
  `PluginBinding.Validate`; **Files:** `internal/mcp/plugin.go`,
  `internal/mcp/plugin_test.go`, `internal/mcp/plugin_client.go`,
  `internal/mcp/plugin_client_test.go`, `docs/plugin-schema.md`,
  `CHANGELOG.md`; **Verify:** first demonstrate focused contract tests fail,
  then `go test ./internal/mcp -run 'Test(Plugin|PluginClient)' -count=1`,
  update the local contract guide and Unreleased entry in this same commit;
  **Commit:** `feat(plugin): define raw response invocation contract`.)

- [x] **Task 2: Separate bounded ERP response capture from normalization.** Write
  failing connector and tool tests first. Introduce an internal ERP response
  capture value containing status, normalized content type, and bounded body
  bytes. Extract request construction and response capture from `Tool.Execute`,
  exposing the capture seam as `Tool.CallERP` and keeping JSON normalization in
  a separate helper used by `Execute`. Preserve `Execute`'s no-raw behavior
  while allowing raw execution to request terminal HTTP error responses.
  Extend the connector with an
  explicit response-preserving mode that retains final `429`/`5xx` bodies while
  preserving retries, circuit-breaker accounting, auth, endpoint safety, and
  metrics. Keep JSON `responsePath` normalization in a separate helper.
  (**Seam:** additive `ERPResponseConnector.CallWithOptions`, the concrete
  connector retry closure, and `Tool.Execute`; **Files:**
  `internal/connector/client.go`,
  `internal/connector/client_test.go`, `internal/connector/resilience_test.go`,
  `internal/mcp/tool.go`, `internal/mcp/tool_test.go`,
  `internal/mcp/mock_test.go`; **Verify:** first demonstrate response-capture,
  2xx, 3xx, 4xx, 429, 5xx, retry, empty-body, malformed-body, and size-boundary
  tests fail, then run `go test ./internal/connector ./internal/mcp -run 'Test(Client|Tool|ERPResponse)' -count=1`;
  **Commit:** `refactor(mcp): separate ERP response capture from normalization`.)

- [x] **Task 3: Add phase-aware lookup and secure raw-binding admission.** Write
  failing registry and API tests first. Add
  `RuntimeBindingsForToolPhase(name, version, phase)`, preserve immutable
  priority/name ordering inside each phase, and keep existing all-phase
  snapshot and cache invalidation behavior. During binding admission, require
  `API_AUTH_TOKEN` and an authenticated admin context, an active HTTP tool with
  an explicit object-shaped output schema for `raw_response`, and a plugin
  endpoint matching `PLUGIN_ENDPOINT_ALLOWLIST` even when plugin auth is absent.
  Reconciliation must keep raw bindings inactive when these prerequisites are
  absent. Add defensive runtime checks for stale snapshots and native tools.
  (**Seam:** `buildPluginBindingSnapshot`, runtime lookup, and
  `validatePluginBindingReferences`; **Files:** `internal/mcp/plugin_registry.go`,
  `internal/mcp/plugin_registry_test.go`, `internal/mcp/plugin_api.go`,
  `internal/mcp/plugin_api_test.go`, `internal/mcp/plugin.go`,
  `docs/plugin-schema.md`, `CHANGELOG.md`;
  **Verify:** first demonstrate phase-filter, ordering, native-tool rejection,
  missing-schema rejection, and unauthenticated-allowlist tests fail, then
  `go test ./internal/mcp -run 'Test(PluginRegistry|PluginBindingAPI|PluginAPI)' -count=1`;
  **Commit:** `feat(plugin): add phase-aware binding admission`.)

- [x] **Task 4: Implement raw processing and mixed-phase execution.** Write
  failing pipeline tests first. Add a raw response adapter that converts JSON
  bodies to decoded values and binary bodies to bounded base64 values, invokes
  all raw bindings in priority order with immutable status, and wraps each
  `PluginResponse.Result` as the next JSON body. For successful responses run
  raw → `responsePath` → schema validation → existing `after_response` chain →
  final validation. For raw-bound terminal errors, statuses outside 200–299
  retain error state and do not apply success-only normalization or
  after-response processing. For a raw-chain failure on a terminal error,
  `continue` returns a generic upstream error without the original body. For a
  successful 2xx chain, apply the declared `continue` fallback from the
  original captured response and return a generic safe error when that
  fallback cannot satisfy the final schema.
  Keep no-raw and after-response-only invocations byte-compatible. (**Seam:**
  `Server.executeTool`, `Tool.CallERP`, normalization helper, and
  `PluginProcessor.Process`; **Files:** `internal/mcp/plugin_pipeline.go`,
  `internal/mcp/tool.go`, `internal/mcp/server_plugin_test.go`,
  `internal/mcp/tool_test.go`, `docs/plugin-schema.md`,
  `skills/bridgectl-ops/references/plugins.md`,
  `skills/bridgectl-ops/assets/plugin-binding.yaml`, `CHANGELOG.md`; **Verify:**
  first demonstrate raw JSON, image,
  4xx, mixed-phase, ordering, status preservation, failure-policy, fallback,
  `responsePath`, and legacy after-response tests fail, then
  `go test ./internal/mcp -run 'Test(ServerPlugin|RawResponse|Tool_Execute)' -count=1`;
  **Commit:** `feat(plugin): process raw ERP responses`.)

- [ ] **Task 5: Advertise final MCP schemas and preserve transport/cache semantics.**
  Write failing MCP, direct, and cache tests first. Map a declared
  object-shaped `ToolSpec.OutputSchema` to MCP `RawOutputSchema`, return
  structured content conforming to that schema alongside equivalent text
  content, and use `{text: string}` for the image-to-text fixture rather than
  a scalar structured result. Propagate tool-execution `isError` from upstream
  status, use generic error content when raw processing fails, and ensure
  direct invocation retains its safe compatibility envelope. Ensure raw error
  results and schema
  failures cannot populate the success cache, while cache hits still bypass
  ERP and plugin calls. Verify that binding changes do not mutate tool schemas
  or require dynamic plugin-defined schemas; an incompatible output contract
  uses a new MCP-visible tool name and exact version rather than silently
  replacing an existing name. (**Seam:** `RegisterTool`,
  `executeToolCall`, direct invocation adapter, and `CacheMiddleware`; **Files:**
  `internal/mcp/server.go`, `internal/mcp/plugin_pipeline.go`,
  `internal/mcp/middleware.go`, `internal/mcp/server_test.go`,
  `internal/mcp/server_plugin_test.go`, `internal/mcp/middleware_test.go`,
  `docs/api.md`, `docs/architecture.md`, `CHANGELOG.md`;
  **Verify:** first demonstrate tools/list output-schema, structured-content,
  MCP/direct error, cache, and no-dynamic-schema tests fail, then
  `go test ./internal/mcp -run 'Test(RegisterTool|MCP|Direct|Cache|ServerPlugin)' -count=1`;
  **Commit:** `feat(mcp): expose stable transformed output contracts`.)

- [ ] **Task 6: Add focused media and black-box integration coverage.** Write
  failing tests first. Add an in-process `httptest` plugin test that asserts the
  raw payload contains status, content type, base64 image bytes, no ERP headers,
  no URL, and no credentials; it returns `{text: ...}` and proves the final
  object schema. Add JSON and image fixtures, body-size boundaries, empty/malformed
  bodies, 2xx/3xx/4xx/429/5xx status cases, raw failure policies, raw-to-after
  ordering, direct/MCP parity, cache behavior, and unchanged after-response
  payloads. Extend the Docker-backed `pluginintegration` test only through a
  reachable plugin service; do not use a host-only `httptest` endpoint from
  inside the container network. (**Seam:** `PluginClient.Process`, the server's
  shared execution seam, and the existing `internal/integration` harness;
  **Files:** `internal/mcp/plugin_client_test.go`,
  `internal/mcp/server_plugin_test.go`, `internal/mcp/integration_test.go`,
  `internal/integration/plugin_system_test.go`,
  `../ERPBridge-Plugins/plugins/mock-plugin/main.go`,
  `../ERPBridge-Plugins/plugins/mock-plugin/main_test.go`; **Verify:**
  `go test ./internal/mcp -run 'Test(Raw|PluginClient|ServerPlugin)' -count=1`,
  `go test -tags pluginintegration ./internal/integration -run
  TestPluginSystemBlackBox -count=1` when the plugin stack is available,
  `go test ./...` from `../ERPBridge-Plugins/plugins/mock-plugin`, and the
  in-process image-to-text acceptance test returns only the declared text
  result. The Docker black-box extension uses the reachable mock plugin for
  raw JSON; the image case stays in-process because the current Docker MockERP
  fixture does not provide a binary image endpoint. **Commit:** root
  `test(plugin): cover raw media adaptation` and
  plugin repository `test: extend mock plugin for raw responses`.)

- [ ] **Task 7: Publish the finalized contract and synchronize public guidance.**
  Reconcile cross-links after the local schema, API, architecture, plugin
  operations, and Unreleased documentation fragments were updated in their
  owning behavior commits. In `../erpbridge-docs`, create or update the
  required documentation plan, mirror the tagged raw-body wire format, media
  limits, status handling, fixed phase order, output-schema ownership,
  tool-version workflow, raw-data trust gate, cache behavior, failure fallback,
  and image-to-text example; update navigation and changelog, and commit the
  public docs separately. (**Seam:** in-repo developer documentation as source
  of truth and the published MCP/plugin contract; **Files:**
  `../erpbridge-docs/.agents/plans/Plan-plugins-raw-response.md`,
  `../erpbridge-docs/docs/erpbridge/plugins.mdx`,
  `../erpbridge-docs/docs/erpbridge/api.mdx`,
  `../erpbridge-docs/docs/erpbridge/architecture.mdx`,
  `../erpbridge-docs/CHANGELOG.md`, `../erpbridge-docs/sidebars.ts`;
  **Verify:** local link checks,
  `npx --yes skills-ref@0.1.5 validate skills/bridgectl-ops`,
  `npm run build` in `../erpbridge-docs`, and a credential/raw-data audit;
  **Commit:** public docs `docs: document raw media plugin adaptation`.)

- [ ] **Task 8: Run quality gates and close the plan atomically.** Run each
  focused test before its owning commit. After Task 7, run the complete ERPBridge
  suite, targeted lint only for changed Go packages, public documentation build,
  skill validation, integration coverage when the stack is available, and
  working-tree audits. Confirm that no credentials, ERP URLs, headers, raw
  bodies, generated binaries, or evaluation artifacts are committed. (**Seam:**
  repository quality gates; **Files:** all files changed by Tasks 1–7;
  **Verify:** `go test ./...`, `make test`,
  `golangci-lint run ./internal/connector ./internal/mcp`,
  `go build ./tools/bridgectl`,
  `npx --yes skills-ref@0.1.5 validate skills/bridgectl-ops`,
  `go test -tags pluginintegration ./internal/integration -run
  TestPluginSystemBlackBox -count=1` when the isolated stack is available,
  `npm run build` in `../erpbridge-docs`, `git diff --check`, and clean
  staged-file/security audits; **Commit:** no additional feature commit—fix
  failures in the owning task commit, then prefix this plan with
  `[COMPLETED]` and move it to `.agents/plans/completed/`.)

## Verification

1. A raw-bound HTTP tool can receive a bounded `image/*` ERP response and send
   only a bounded, tagged base64 representation plus status and media type to
   the allowlisted plugin.
2. The plugin can return a `{text: ...}` result, and ERPBridge validates and
   exposes it using the selected tool name and version's declared object output
   schema.
3. The raw plugin cannot change the HTTP status, access ERP headers or URL, or
   cause credentials or raw bodies to appear in logs and errors.
4. JSON raw responses support decoded-body transformation, while binary raw
   responses support base64 input; malformed and oversized bodies fail safely.
5. Raw, normalization, and after-response phases execute in the documented
   order. Existing after-response invocations remain unchanged when no raw
   binding is active.
6. Terminal `429` and `5xx` responses are available to raw processing after the
   connector's normal retries without disabling retry or circuit-breaker
   accounting.
7. Successful 2xx outputs pass the final schema gate; terminal non-2xx outputs
   retain upstream error state and do not run success-only normalization.
8. `continue` never emits a result that violates the final tool schema. An
   image-to-text conversion with no valid fallback returns a safe error; `fail`
   returns the generic plugin-processing error.
9. MCP `tools/list` advertises the final output schema for the selected
   MCP-visible tool name, successful calls return conforming structured and
   text content, and execution errors use `isError: true`. Direct invocation
   and MCP remain equivalent for successful results.
10. Cache hits bypass ERP and plugins, cache writes exclude error results, and
    binding/plugin lifecycle changes invalidate affected tool entries.
11. Existing unbound tools and after-response-only tools retain their prior
    normalized results and invocation payloads.
12. Local and public documentation explain that developers must define the
    final schema and publish a new MCP-visible tool name plus exact version when
    a plugin changes output type. All quality gates and documentation builds
    pass.

## Open Questions

None. The finalized design uses a tagged JSON/base64 raw-body contract, allows
terminal non-2xx inspection, keeps success-only normalization and
`after_response`, requires allowlisted raw plugins, and treats the tool version
as the owner of the final MCP output schema.
