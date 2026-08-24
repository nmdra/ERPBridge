# Plan: Minimal Generic External Plugin System

## Goal

Add a backward-compatible, generic external-plugin control plane on a new
feature branch. A customer deploys and runs plugin code separately; ERPBridge
only stores `Plugin` and `PluginBinding` resources, invokes bound plugins for a
successful tool response, and returns the transformed response through the
existing MCP and direct-invoke paths.

The first implementation proves the generic contract with one deterministic
mock response plugin. It does **not** implement OCR, PII masking, binary image
handling, plugin deployment, or a plugin scheduler.

Plugin source code is stored in the separate `ERPBridge-Plugins` polyrepo
(`../ERPBridge-Plugins`), not in this repository. The polyrepo can contain
multiple independently buildable plugins under `plugins/<plugin-name>/`; this
plan's mock plugin is the first entry in that collection.

## Current State

- The repository is currently on `main`; the worktree already has unrelated
  untracked files. Implementation must begin on `feat/generic-external-plugins`
  and must never stage, delete, or modify those pre-existing files.
- `MCPTool` is the only persisted declarative runtime resource. The tool API
  validates, saves, lists, soft-deletes, and hard-deletes tools through
  `/apis/erpbridge.io/v1/tools` (`internal/mcp/server.go:649-784`). Tool data is
  stored as JSON in SQLite; `Store.init` contains the idempotent table setup and
  `Store` exposes save/list/get/delete methods (`internal/mcp/store.go:42-250`).
- The controller compares the current store hash to `lastDesiredHash` every ten
  seconds, registers active desired tools, and deregisters stale ones
  (`internal/mcp/server.go:339-418`). The same immediate-plus-background
  reconciliation model is the appropriate lifecycle for plugin resources.
- An MCP invocation reaches `Tool.Execute` through rate limit, logging,
  metrics, role authorization, and cache middleware
  (`internal/mcp/server.go:461-538`; `internal/mcp/middleware.go:168-222`).
  Direct `/api/tools/invoke` reconstructs an equivalent but currently
  duplicated execution adapter (`internal/mcp/server.go:813-899`).
- `Tool.Execute` is the normalized ERP response boundary: it decodes JSON,
  applies `responsePath`, and validates the successful result against
  `outputSchema` before returning `ToolResult` (`internal/mcp/tool.go:134-254`).
  Existing MCP output is JSON serialized into `TextContent`; direct invoke
  preserves the legacy `{ "result": ... }` compatibility envelope.
- The cache serializes the final `mcp.CallToolResult`, and cache keys are based
  on tool, authorized role scope, and canonical arguments
  (`internal/mcp/middleware.go:168-222`; `internal/cache/manager.go:60-147`).
  `cache.Manager.FlushTool` can invalidate every exact entry for a tool
  (`internal/cache/flush.go:13-35`).
- `bridgectl tool` already provides the desired user interaction pattern:
  JSON/YAML apply, get/list, validate, and soft/hard delete
  (`internal/cli/tool.go:28-150`, `:250-501`), with authenticated server
  requests via `doBridgeRequest` helpers.
- The local Compose stack contains ERPBridge, Mock ERP, and Redis only
  (`docker-compose.yml:1-58`). The shared deterministic Mock ERP fixture is
  specified separately in `../active/Plan-Mock-ERP.md`; this plan consumes it
  but does not own its implementation.
- The existing test style is Go TDD with `httptest`, `:memory:` SQLite, and
  connector mocks (`AGENTS.md:31-35`; `internal/mcp/api_test.go:17-291`).
  There is no existing Docker integration-test harness.
- Documentation and the public docs repository must be updated for every
  behavior change (`AGENTS.md:37-48`). The matching public pages currently
  live in `../erpbridge-docs/docs/erpbridge/architecture.mdx`, `api.mdx`, and
  `docker.mdx`.
- The separate `../ERPBridge-Plugins` repository is the source repository for
  plugin implementations. ERPBridge must consume a pinned plugin image or
  endpoint and must not store plugin source under this repository.

## Decisions

1. **Two resources only.** The v1 feature introduces `Plugin` and
   `PluginBinding`. There is no `PluginRuntime`, artifact registry, deployment
   object, or release-management object. A plugin process/container is owned
   and started by the customer or deployment operator, never by ERPBridge.

2. **One external protocol.** A `Plugin` identifies an existing `http` or
   `https` endpoint and a bounded timeout. ERPBridge POSTs JSON to
   `/v1/process` and expects JSON `{ "result": <JSON value> }`. The request
   carries only protocol version, generated invocation ID, exact tool name and
   version, the normalized successful tool result, and the binding's static
   JSON configuration. It deliberately omits ERP credentials, inbound headers,
   caller tokens, role identity, and original tool arguments.

3. **One execution phase in v1.** `PluginBinding.spec.phase` must be exactly
   `after_response`. It runs synchronously after a successful, normalized,
   already-output-schema-valid `ToolResult`, and before ERPBridge serializes the
   response for MCP or direct invoke. No request mutation, error-response
   processing, resource/prompt hooks, async events, retries, image artifacts,
   or binary/XML payloads are in scope.

4. **A binding is explicit and version-pinned.** A binding has a unique name
   and references exact active `Plugin{name, version}` and
   `MCPTool{name, version}` identities. It contains phase, ordered priority,
   `failurePolicy` (`continue` or `fail`), and JSON configuration. New tool or
   plugin versions never receive an old binding automatically.

5. **Failure behavior is explicit but safe by default.** `continue` is the
   default: timeout, non-2xx response, malformed JSON, oversized response, or
   invalid transformed output causes ERPBridge to log safe metadata and return
   the original validated ERP result. `fail` returns a generic tool failure
   without exposing the endpoint, response body, credentials, or plugin error
   payload. The plugin client never retries, preventing duplicate external side
   effects.

6. **The pipeline remains inside cache execution.** Refactor the duplicate
   MCP/direct base handler to one internal server invocation seam. The pipeline
   runs inside `CacheMiddleware` only on a cache miss; the cached value is the
   final transformed MCP result. A plugin/binding apply, update, soft delete,
   or hard delete flushes affected target-tool cache entries. This prevents
   stale enriched output while preserving all cache behavior for unbound tools.

7. **Revalidate transformed output.** Existing `Tool.Execute` output-schema
   validation remains unchanged. Extract a reusable tool-result validation
   helper and validate a plugin's transformed result again. This preserves the
   tool's declared response contract. A failed revalidation follows the
   binding's failure policy.

8. **Minimal control-plane security.** Plugin and binding routes use the same
   admin-only HTTP protection as the existing tool registry. Plugin endpoints
   must be absolute `http`/`https` URLs without userinfo, query, or fragment;
   inline credentials are rejected. The dedicated HTTP client disables
   redirects, uses the bound context plus the resource timeout, limits request
   and response JSON to 1 MiB, and logs only resource identity, tool identity,
   status, and duration.

9. **Simple test plugin, not a business feature.** The fixture is a separate
   deterministic HTTP process named `mock-plugin`; it adds a fixed
   `processedBy: "mock-plugin"` property to a test-only mock ERP response. It
   does not call an OCR, PII, or other external business service.

10. **Plugin implementations use a dedicated polyrepo.** Create or use the
    separate `ERPBridge-Plugins` repository as a polyrepo. Store each plugin in
    `plugins/<plugin-name>/` with its own source, tests, dependency manifest,
    and Dockerfile. The repository root may contain shared SDK code, CI, and
    release tooling, but plugins must remain independently buildable and
    releasable. Publish each plugin independently as
    `ghcr.io/nmdra/erpbridge-plugins/<plugin-name>:<version>`. ERPBridge must
    not vendor or build these plugin sources.

## Scope

### In scope

- A separate branch and TDD implementation of `Plugin` and `PluginBinding`
  resources, persistence, admin APIs, reconciliation, and bridgectl commands.
- A synchronous generic JSON post-response invocation contract.
- Shared MCP/direct invocation handling while preserving their existing wire
  envelopes.
- Final-result cache correctness, safe errors/logging, and focused metrics-free
  observability through structured logs.
- A separate `ERPBridge-Plugins` polyrepo with a proper multi-plugin layout,
  including the mock plugin that consumes the shared Mock ERP fixture contract,
  and an opt-in Docker Compose integration test using that plugin.
- In-repository and public-documentation updates, CLI reference generation,
  changelogs, focused lint, full tests, and atomic commits.

### Out of scope

- ERPBridge starting, stopping, downloading, signing, upgrading, or otherwise
  deploying plugin code.
- OCR, PII/DLP masking, image or attachment transport, non-JSON ERP responses,
  XML, WebAssembly, gRPC, webhooks, asynchronous delivery, queues, retries,
  secret brokering, plugin endpoint authentication, health discovery, request
  hooks, and plugin access to ERP credentials.
- Changes to existing MCP tool names, tool schemas, OpenAPI generation, MCP
  transport/session/auth behavior, direct-invoke envelope, role authorization,
  ERP retries, or behavior of any tool without an active binding.
- Storing plugin source code in ERPBridge; all plugin implementations belong in
  `../ERPBridge-Plugins/plugins/<plugin-name>/`.

## Tasks

- [ ] **Task 0: Create the isolated feature branch and preserve the starting worktree.** Before changing source, record `git status --short`, create and switch to `feat/generic-external-plugins` from the current `main` HEAD, and confirm the pre-existing untracked files remain unmodified and unstaged. Do not promote this upcoming plan to `active/` until the user explicitly approves execution. (**Seam:** Git worktree and project-required plan workflow; **Files:** no source files; **Verify:** `git branch --show-current` prints `feat/generic-external-plugins`, `git status --short` still lists only the original unrelated untracked files before implementation, and `git diff --cached --name-only` is empty; **Commit:** none.)

- [ ] **Task 1: Define the minimal external-plugin and binding contracts with a bounded HTTP JSON client.** Write failing unit tests first. Add `Plugin` (`apiVersion`, `kind`, `{name, version, isActive}`, `{endpoint, timeoutMilliseconds}`), `PluginBinding` (`apiVersion`, `kind`, `{name, isActive}`, exact plugin/tool refs, `after_response`, priority, failure policy, config), `PluginInvocation`, and `PluginResponse` types. Implement pure resource validation, endpoint validation, and an injected HTTP client that POSTs the generic request and decodes only `{result: ...}`. Enforce the 1 MiB request/response cap, no redirects, context deadline, no retries, and no raw payload/error logging. Tests must establish accepted/rejected resource shapes, required exact references, only allowed phase/policy values, no URL userinfo/query/fragment, successful round trip, timeout, redirect, non-2xx, malformed JSON, oversized payload, and that invocation payload omits arguments/headers/credentials. (**Seam:** new external process boundary called after a tool response; **Files:** `internal/mcp/plugin.go` (new), `internal/mcp/plugin_client.go` (new), `internal/mcp/plugin_test.go` (new), `internal/mcp/plugin_client_test.go` (new); **Verify:** first demonstrate targeted tests fail, then `go test ./internal/mcp -run 'Test(Plugin|PluginClient)'`; **Commit:** `feat: define generic external plugin contract`.)

- [ ] **Task 2: Persist plugin resources and bindings with safe lifecycle semantics.** Write failing store tests first. Extend schema initialization with idempotent `plugins` and `plugin_bindings` tables, JSON resource data, active-state columns, update timestamps, and indexes needed for exact plugin references and target tool lookups. Add a dedicated `plugin_store.go` rather than expanding tool CRUD indiscriminately. Implement save/list/get/soft-delete/hard-delete methods, exact binding lookup by tool name/version, and a combined desired-state hash that includes tools, plugins, and bindings. Require that a hard plugin delete fails with `409 Conflict` when an active binding references it; soft plugin deletion leaves the binding stored but inactive at runtime. Existing tool-only SQLite databases must initialize without schema or data changes. (**Seam:** `Store.init` and JSON-backed declarative-resource persistence; **Files:** `internal/mcp/store.go`, `internal/mcp/plugin_store.go` (new), `internal/mcp/plugin_store_test.go` (new), `internal/mcp/store_test.go`; **Verify:** first demonstrate new CRUD/migration/reference tests fail, then `go test ./internal/mcp -run 'Test(Store|PluginStore)'`; **Commit:** `feat: persist plugin resources and bindings`.)

- [ ] **Task 3: Add admin-only plugin control-plane APIs and reconcile an immutable active binding snapshot.** Write REST and reconciliation tests first. Extend `Server` with a lock-protected active binding map keyed by exact `toolName@toolVersion`; rebuild it during immediate and ten-second reconciliation from active persisted plugins/bindings. Add admin-only `POST`, `GET`, and `DELETE` routes for `/apis/erpbridge.io/v1/plugins` and `/apis/erpbridge.io/v1/pluginbindings`, matching existing tool resource status codes and list/filter semantics. Server admission must check binding referential integrity against active stored exact resources. Applying, changing, or deleting a binding/plugin must reconcile immediately and flush affected tool cache entries; it must not alter MCP `tools/list` or emit tool-list-changed notifications because no MCP tools are added or removed. Test malformed JSON, `422` admission failures, filters, soft/hard lifecycle, missing/inactive refs, hard-delete conflict, auth wrapping, store failure, controller refresh, and cache invalidation. (**Seam:** existing authenticated control-plane router and `Server.Reconcile`; **Files:** `internal/mcp/server.go`, `internal/mcp/plugin_registry.go` (new), `internal/mcp/plugin_api_test.go` (new), `internal/mcp/api_test.go`, `internal/mcp/server_test.go`, `internal/mcp/auth_test.go`; **Verify:** first demonstrate focused API/reconciliation tests fail, then `go test ./internal/mcp -run 'Test(Server_(Plugin|PluginBinding|Reconcile)|PluginAPI|AuthHandler)'`; **Commit:** `feat: add plugin control plane`.)

- [ ] **Task 4: Run active response bindings consistently in MCP and direct invocation paths.** Write failing behavior tests first. Extract the duplicated execution/serialization base in `handleMCPToolCall` and `handleDirectInvoke` into one internal server helper while leaving each public transport's result envelope unchanged. On a cache miss, call `Tool.Execute`; only when it returns a successful non-error result, resolve ordered active bindings for that exact tool version and synchronously invoke each plugin. Feed one plugin's transformed result into the next, then revalidate with the tool's existing output schema before serialization. Preserve the original result on `continue` failures; return a generic plugin failure on `fail`; never invoke bindings for ERP errors, response-path errors, base schema failures, denied calls, or cache hits. Cache the final transformed MCP result. Add tests for ordered transformations, no-binding byte-for-byte legacy result behavior, MCP/direct equivalence, failure policies without leaked data, post-transform schema validation, cache hit invokes plugin once, binding update flushes cache, role denial/no ERP error does not invoke the plugin, and existing `responsePath` tests remain green. (**Seam:** the base handler nested inside `CacheMiddleware`, shared by `tools/call` and `/api/tools/invoke`; **Files:** `internal/mcp/server.go`, `internal/mcp/tool.go`, `internal/mcp/tool_test.go`, `internal/mcp/server_plugin_test.go` (new), `internal/mcp/middleware_test.go`, `internal/mcp/authz_cache_test.go`; **Verify:** first demonstrate server-plugin tests fail, then `go test ./internal/mcp -run 'Test(ServerPlugin|Tool_Execute|CacheMiddleware|GuardedCache)'`; **Commit:** `feat: process bound external plugin responses`.)

- [ ] **Task 5: Add bridgectl support for both declarative resources.** Write CLI tests first. Add `bridgectl plugin apply|get|delete|validate` and `bridgectl plugin binding apply|get|delete|validate`. Plugin commands use exact `name@version`; binding commands use binding name. Match tool command behavior for JSON/YAML file input, YAML multi-document/sequence handling, recursive directory apply, output formats, authenticated management requests, server-error propagation, `--hard`, and `--yes`. Keep the existing tool decoder untouched; introduce typed plugin/binding decoders and reuse pure local shape validation while retaining server admission as authoritative. Test endpoint/method/query/body/auth header, document parsing, table/JSON/YAML output, required identity, invalid local definitions, hard-delete confirmation bypass, and server error behavior. (**Seam:** Cobra resource-command family and `doBridgeRequest` helpers; **Files:** `internal/cli/plugin.go` (new), `internal/cli/plugin_binding.go` (new), `internal/cli/plugin_test.go` (new), `internal/cli/plugin_binding_test.go` (new), `internal/cli/root.go`; **Verify:** first demonstrate CLI tests fail, then `go test ./internal/cli -run 'Test(Plugin|PluginBinding)'`; **Commit:** `feat: manage plugins with bridgectl`.)

- [ ] **Task 6: Add the polyrepo mock plugin and run repeatable black-box coverage.** Keep all tests red before implementation. Create or use the separate `../ERPBridge-Plugins` polyrepo and store the mock plugin under `plugins/mock-plugin/`; future plugins must use the same `plugins/<plugin-name>/` structure and remain independently buildable and releasable. Consume the shared `GET /api/resource/Plugin Fixture` contract from
  `../active/Plan-Mock-ERP.md`, which must be completed before this task. Add a
  simple standalone `mock-plugin` HTTP service with `/health` and `/v1/process`. The mock plugin must return the supplied result plus `processedBy: "mock-plugin"`; it must not implement OCR/PII behavior. Add a Compose test overlay, isolated project name/ports/volumes, a cleanup-trapped `scripts/test-plugin-integration.sh`, an opt-in Go black-box integration test, and `make test-plugin-integration`. The test starts the isolated stack, applies Plugin/Binding and two hand-authored test tools, then proves both MCP `initialize → tools/list → tools/call` and direct invoke return the same transformed fixture for the bound tool and the unchanged source fixture for the ordinary control tool. The script must tear down only the isolated project, including volumes, even after test failure. (**Seam:** `../ERPBridge-Plugins/plugins/mock-plugin/` image to the real Docker network from ERPBridge to separately running mock plugin and mock ERP; **Files:** `../ERPBridge-Plugins/plugins/mock-plugin/` (new), `docker-compose.plugin-test.yml` (new), `scripts/test-plugin-integration.sh` (new), `internal/integration/plugin_system_test.go` (new), `Makefile`; **Verify:** first demonstrate the opt-in integration test fails against the unimplemented fixture, test the plugin from its own directory with `go test ./...`, build and publish a pinned plugin image, then `make test-plugin-integration`, followed by `docker compose -p erpbridge-plugin-test -f docker-compose.yml -f docker-compose.plugin-test.yml ps` returning no running test project after cleanup; **Commit:** plugin source is committed in `ERPBridge-Plugins`, and ERPBridge integration changes use `test: add external plugin integration fixture`.)

- [ ] **Task 7: Document the control-plane contract and publish the matching public documentation.** In ERPBridge, document the two manifest schemas, plugin deployment boundary, HTTP request/response example, only-supported `after_response` phase, exact version binding, synchronous timeout/failure policy, cache behavior, admin-only APIs, CLI commands, and Docker test/deployment example. Generate the Cobra CLI Markdown reference after commands are stable; add an Unreleased changelog entry. Correct the architecture guide's stale statement that describes the state hash as `count-activeSum-max(updated_at)`: implementation uses a SHA-256 over ordered row identity, activity, and update timestamp (`internal/mcp/store.go:188-209`). In `../erpbridge-docs`, first create the repository-required plugin documentation plan, then add a `plugins.mdx` guide and navigation entry, update architecture/API/Docker pages, update that repository's changelog, and commit it separately. Do not include literal credentials in either repository. (**Seam:** in-repo docs as developer source of truth and Docusaurus site as user-facing mirror; **Files:** `docs/plugin-schema.md` (new), `docs/api.md`, `docs/architecture.md`, `docs/docker.md`, `docs/README.md`, `docs/cli/bridgectl_plugin.md` (generated), `docs/cli/bridgectl_plugin_binding.md` (generated), `CHANGELOG.md`; `../erpbridge-docs/.agents/plans/Plan-plugins.md` (new), `../erpbridge-docs/docs/erpbridge/plugins.mdx` (new), `../erpbridge-docs/docs/erpbridge/architecture.mdx`, `../erpbridge-docs/docs/erpbridge/api.mdx`, `../erpbridge-docs/docs/erpbridge/docker.mdx`, `../erpbridge-docs/sidebars.ts`, `../erpbridge-docs/CHANGELOG.md`; **Verify:** build `bridgectl`, run its document generator into a temporary directory and compare the two generated command files before replacing them, run `npm run build` in `../erpbridge-docs`, and confirm no raw credential appears with `rg -n -i '(api[_-]?key|token|password):\s*[^<$ {]' docs ../erpbridge-docs/docs`; **Commit:** `docs: document external plugin control plane` in ERPBridge and `docs: add ERPBridge plugin guide` in `erpbridge-docs`.)

- [ ] **Task 8: Run required quality gates and make atomic task commits.** For every preceding task, run its focused tests before its single Conventional Commit; include user-facing documentation/changelog changes with the behavior they describe and never commit generated binaries, generated schemas, Docker volumes, plugin source, or the pre-existing untracked worktree files in ERPBridge. After Task 7, run the full ERPBridge suite, targeted lint only across directories changed by this plan, the isolated plugin integration test, and both documentation checks. Review the MCP and direct responses for every legacy tool fixture to confirm existing result envelopes and unbound behavior did not change. (**Seam:** repository quality gates across ERPBridge and `../ERPBridge-Plugins`; **Files:** all files changed by Tasks 1-7 plus `../ERPBridge-Plugins/plugins/mock-plugin/`; **Verify:** `go test ./internal/mcp ./internal/cli`, plugin tests from `../ERPBridge-Plugins/plugins/mock-plugin/`, targeted lint in both repositories, `make test-plugin-integration`, `make test`, `git diff --check`, `git status --short`, and `npm run build` in `../erpbridge-docs`; **Commit:** no additional feature commit—resolve failures in the owning task commit before closing the plan.)

## Verification

1. The implementation runs only on `feat/generic-external-plugins`; no pre-existing untracked file is staged, deleted, or changed.
2. Existing tool resource routes, MCP `/mcp/` transport, tool discovery, role authorization, cache behavior, and direct `{ "result": ... }` compatibility response retain their current behavior for tools without a binding.
3. Only an administrator can apply/list/delete plugin resources or bindings when HTTP authentication is enabled.
4. A binding with an absent/inactive plugin or exact target tool is rejected. An active plugin with active bindings cannot be hard-deleted.
5. A successful bound tool calls its external HTTP plugin once on a cache miss; MCP and direct invoke return the same transformed JSON result.
6. A successful unbound tool does not make an external call and returns its prior normalized JSON unchanged.
7. Plugin timeout, redirect, malformed/non-2xx/oversized result, and invalid transformed schema follow the binding policy without exposing plugin URLs, payloads, credentials, or plugin response bodies.
8. Cache hits do not call plugins; any binding/plugin lifecycle change invalidates the affected tool cache before the next request.
9. Plugin source is stored in the separate `ERPBridge-Plugins` polyrepo under `plugins/<plugin-name>/`, with `plugins/mock-plugin/` as the first plugin and no plugin source duplicated in ERPBridge.
10. The isolated Docker integration stack demonstrates a separately running pinned mock plugin from `ERPBridge-Plugins` and the shared Mock ERP fixture; it leaves no running containers or volumes after cleanup.
11. Focused tests, targeted lint in both repositories, `make test-plugin-integration`, `make test`, `git diff --check`, and the public docs `npm run build` are green.

## Open Questions

None. The plan intentionally defers external plugin authentication, health discovery, image/OCR transport, PII/DLP policy semantics, and deployment management until the minimal generic contract is proven.
