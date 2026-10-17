# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add `bridgectl mcp --stdio`, a local management MCP server for AI coding
  agents. It serves existing `bridgectl` management operations over stdio with
  read-only defaults and operator-approved write groups, pins each session to
  its launch-time context, endpoints, and credential, keeps stdout strictly for
  protocol traffic, and reports an interrupted write as `outcome_unknown`
  instead of claiming rollback.
- Add the read-only management catalogue to `bridgectl mcp --stdio`:
  `bridgectl_context_list`, `bridgectl_api_list`, `bridgectl_tool_get`,
  `bridgectl_tool_generate`, `bridgectl_tool_validate`, `bridgectl_plugin_get`,
  `bridgectl_plugin_validate`, `bridgectl_binding_get`,
  `bridgectl_binding_validate`, `bridgectl_token_list`,
  `bridgectl_cache_stats`, and `bridgectl_log_stats`. Results are an explicit
  allow-list: credential values, literal header values, schema defaults and
  examples, raw endpoint URLs, plugin configuration values, token values, and
  raw log messages are never returned. Manifests are one inline object, and an
  embedded credential value is rejected before it reaches disk.
- Add a structured output branch to `bridgectl log stats`, so `-o json` and
  `-o yaml` return `{"levelCounts":{...},"toolCounts":{...}}` counts without
  log messages. Table output is unchanged.
- Add the approved write and probe catalogue to `bridgectl mcp --stdio`, gated by
  the `registry`, `resources`, `destructive`, `probe`, and `context` startup
  groups: `bridgectl_api_register`, `bridgectl_api_set_credential_ref`,
  `bridgectl_api_test`, `bridgectl_tool_apply`, `bridgectl_tool_delete`,
  `bridgectl_plugin_apply`, `bridgectl_plugin_delete`,
  `bridgectl_binding_apply`, `bridgectl_binding_delete`,
  `bridgectl_token_revoke`, `bridgectl_cache_flush`, and
  `bridgectl_context_set`. Every write requires an exact context and target
  confirmation, and forced registration also requires the destructive group.

### Changed

- Report a launched write or probe as `outcome_unknown` when it fails, times
  out, is cancelled, exceeds its output limit, or returns unusable output, so a
  caller reads the target back instead of assuming the change did not happen.
  Only an argument vector the child rejects before running, or the session
  `context_changed` precondition, reports a specific failure.
- Accept an explicit empty `authType` in `bridgectl_api_register`, which
  registers an API that carries no authentication.
- Keep `bridgectl context set` from rewriting the whole configuration. It now
  edits the raw file, so environment placeholders, stored values, and unknown
  fields survive, the replacement is written with mode 0600, and a concurrent
  external change is refused instead of overwritten.
- Document the management MCP server end to end: the tool catalogue, result
  redaction, startup write groups, canonical confirmation targets, the unknown
  outcome contract, Codex CLI and Claude Code registration examples, session
  pinning, and restart-based recovery. The runtime `erpbridge-server --stdio`
  server and the management `bridgectl mcp --stdio` server are documented as
  separate registrations.

### Fixed

- Check the control-plane status before decoding a typed body in `bridgectl tool
  get`, `tool describe`, and `log stats`. A rejected or missing credential
  previously surfaced as
  `json: cannot unmarshal object into Go value of type []*mcp.Tool` and exited
  `1`, hiding the server's own `AUTHENTICATION_FAILED` envelope and its
  suggestion. These commands now report the server's message and exit `4`.
- Report a tool that is not in the selected deployment from `tool describe` with
  the recognized `RESOURCE_NOT_FOUND` code, matching `tool get`.
- Report the server's `AUTHENTICATION_FAILED` message and suggestion instead of a
  decoding failure on every read path, including `log stats`. A rejected
  credential exits `4`.
- Reword the control-plane `401` suggestion to
  `set a valid token in the Authorization header`. The previous wording,
  `set a valid Authorization bearer token`, was mangled by the client's own
  bearer redaction into `set a valid Authorization Bearer [REDACTED]`, so the
  operator could not tell whether the CLI had hidden a value. The redaction stays
  strict: a credential is redacted whatever it is spelled, and `tool get`
  completion reports a server error as a directive rather than decoding it.
- Serialize an empty collection as a list rather than `null` in `tool get`,
  `plugin get`, `plugin binding get`, `token list`, and `context list`. A nil
  slice produced `{"tools": null}`, which a consumer that reads the output as
  data cannot treat as a list.
- Render the named `tool get` payload through the command's output writer, so
  both branches of the command reach the same destination.
- Stage a management manifest under a unique private name and only after policy
  checks pass, so a concurrent call cannot overwrite or remove the input of an
  active one.
- Reject a submitted manifest whose endpoint carries credentials in its userinfo
  or query, and a concrete default or example for a credential-like schema
  property, before anything reaches disk. A schema declaration such as a
  property named `password` stays legal.
- Report a launched write as `outcome_unknown` whenever the deployment answered,
  because a remote rejection arrives with the same exit code as a local argument
  rejection, and only an envelope-free rejection proves that nothing ran.
- Report a management child that can no longer resolve its pinned context as
  `context_changed`, for reads and writes alike, instead of a generic
  context-not-found failure.
- Reject an inventory, statistics, probe, or flush payload that omits its typed
  fields, instead of reporting an empty or zeroed success.
- Write the replacement `bridgectl` configuration with mode 0600 in every case,
  instead of preserving a wider existing mode.
- Make `bridgectl tool get`, `plugin get`, and `plugin binding get` report a
  missing named resource with the CLI's recognized not-found error code instead
  of a generic failure, so an agent can classify it without reading prose.
- Make `bridgectl tool get` honour its own `--output` flag when listing tools.
  The command defines its own flag, which shadows the global one, so the list
  path previously rendered a table even when JSON or YAML was requested. The
  named-resource path and the `plugin get` and `plugin binding get` commands are
  unchanged.

## [v0.5.0-alpha.6] - 2026-10-15

### Changed

- Default the generated read cache TTL to 60 seconds instead of 300 seconds. Generated GET and HEAD tools keep the shared read-only cache; the shorter TTL reduces stale ERP reads. Generated write methods still default to `enabled: false`.

## [v0.5.0-alpha.5] - 2026-10-16

### Fixed

- Keep exact-revision names callable while they stay out of MCP `tools/list`. Hiding them through the SDK tool filter also removed them from `tools/call` dispatch, so a pinned name such as `get_item.rev_MS4wLjA` returned `tool not found` in `v0.5.0-alpha.4`. The listing is filtered by an `OnAfterListTools` hook instead, so registration and dispatch are unchanged.

## [v0.5.0-alpha.4] - 2026-10-16

### Changed

- Keep exact-revision names out of MCP `tools/list` by default, so clients show one entry per tool as they did before `v0.5.0-alpha.3`. Exact-revision calls, `name@version` on the direct endpoint, and `_meta.toolplane.resourceDigest` binding all still work. Set `MCP_ADVERTISE_REVISION_ALIASES=true` to list the exact names again.

## [v0.5.0-alpha.3] - 2026-10-16

### Changed

- Scope MCP validation and cache entries to the resolved immutable tool revision, refresh serving metadata for every exact descriptor, and reserve the `.rev_` generated-name namespace.
- Make serving transitions transactional, migrate legacy revisions to canonical admission records with semantic-version-compatible serving selection, and preserve intentional no-fallback withdrawals across restarts.
- Force redirect suppression at the ERP authority boundary, propagate authority failures through raw-response plugins, and invoke authority observers outside the commitment lock.
- Treat tool-registration failures and persisted serving-pointer differences as reconciliation failures until the runtime actually converges.
- Bind each admitted tool name and version to a canonical executable-content
  digest and server-authored review record. Identical apply requests remain
  idempotent; changed same-version content now returns `REGISTRY_CONFLICT`.
- Persist an operator-selected serving revision, expose active revisions through
  protocol-safe exact MCP names and digest metadata, and carry one cloned tool
  snapshot through middleware and execution without revision fall-forward.
- Serialize single-process withdrawal with final connector commitment, expose
  acknowledged sanitized authority events for tests, and reject effective ERP
  origins that are not bound to the admitted tool revision.
- Expose safe process-local reconciliation attempt, success, error, desired-hash,
  observed-generation, and convergence status; retain periodic recovery after a
  complete failed interval without requiring a restart.
- Add deterministic cache/request microbenchmarks and a bounded concurrent
  in-memory cache-hit stress benchmark with documented local baselines.
- Add a Tool Details action that opens the selected tool's focused integration topology path.
- Require a concrete top-level `type` for declared output schemas. The local
  validator rejects untyped schemas, and MCP discovery omits legacy untyped
  schemas.

### Fixed

- Preserve JSON enum types and explicit object extra-property policies from
  OpenAPI generation through manifest storage, MCP discovery, and argument
  validation, including nested objects and flattened request bodies
- Preserve the connector request context until the returned response body reaches
  EOF, fails to read, or is closed. Delayed success and preserved error bodies can
  now be consumed after headers arrive while existing deadlines, cancellation,
  authentication, and unsafe-method no-retry behavior remain enforced.

## [v0.5.0-alpha.2] - 2026-08-30

### Changed

- Refactor the read-only topology console into a progressive component explorer
with ELK layout, expandable high-cardinality groups, custom React Flow node
families, semantic edge styling, and an accessible relationship fallback.
- Add explicit ERP endpoint nodes and API-to-endpoint edges so focused views show
MCP tool → ERP API component → exact method/path endpoint relationships.
- Move the component navigator and inspector below the full-width topology canvas.
- Narrow a selected MCP tool to its direct transport, ERP API, exact endpoint,
  and plugin path instead of rendering unrelated component members dimmed.
- Collapse topology filter controls until the user selects **Show filters**.
- Increase the default tool-execution rate limit to 10 requests per second with
  a burst of 20, while preserving environment overrides.
- Align MCP tool errors with 2025-11-25 handling: protocol failures use
  JSON-RPC errors, execution failures use sanitized `isError` results, and
  bounded retry/concurrency metadata is available through namespaced `_meta`.
- Preserve registered upstream authentication headers in generated and
  declarative MCP tools, including `X-API-Key` for API-key services.
- Refresh the bundled `bridgectl-ops` skill for file-backed credential
  rotation, split tool generation, MCP annotation and `_meta` guidance, and
  safe embedded-skill installation.
- Add optional project-local `bridgectl-ops` operational memory guidance with
  bounded retrieval, redacted execution evidence, and gated skill proposals.
- Add a `bridgectl-ops` major-task completion checkpoint that attempts one
  redacted append-only execution record after verification, supports terminal
  blocked outcomes, and reports absent-memory or append failures without
  inventing evidence or claiming a runtime hook.

## [v0.5.0-alpha.1] - 2026-08-29

### Added

- Add environment-default credential resolution with explicit per-resource
  mounted-file sources, bounded fail-closed reads, hot rotation without a
  server restart, safe outcome metrics, and AWS/Azure/GCP CSI deployment
  guidance
- Add a disposable `scripts/test-onboarding.sh` Compose check covering
  server-side probing, draft generation, validation, apply, MCP discovery/call,
  duplicate protection, and control-plane URL recovery without printing
  credentials or ERP bodies
- Expand `bridgectl-ops` onboarding and diagnosis guidance with deterministic
  preflight, stable-code recovery branches, safe manifest ownership, and
  credential-free positive and near-miss evaluations
- Gate development-only `system.*_test` tools, validate tool data classes, and
  require roles for `pii` and `restricted` tools
- Reload Console context and inventory snapshots with browser-session context
  persistence, stale-data retention, and retry/refresh controls
- Return bounded structured control-plane errors with stable codes and safe
  remediation messages across the server and CLI
- Generate one reviewable YAML tool manifest with deterministic intent metadata,
  method-aware cache defaults, and sanitized-name collision detection
- Add optional `bridgectl tool generate --output-dir` output with one JSON or YAML
  file per tool, using the tool name as the filename
- Add optional method-based MCP tool annotation hints to generated manifests
- Expose tool guidance and allowed-role discovery metadata through MCP
- Run `bridgectl api test` through an authenticated server-side probe by
  default, with bounded status summaries and an explicit `--local` legacy mode
- Normalize MCP transport suffixes for unambiguous control-plane CLI calls and
  reject other configured paths with actionable errors
- Add a non-interactive `make dev-up` Compose bootstrap with ephemeral local
  MockERP credentials, safe health polling, and loopback-only RedisInsight
  exposure by default
- Gate development-only MCP demo tools behind `MCP_ENABLE_TEST_TOOLS`, classify
  tool data with optional `security.dataClass`, and require roles for PII and
  restricted tools
- Advertise developer-owned tool output schemas through MCP, return structured
  content with equivalent text, propagate tool execution errors with `isError`,
  and prevent error results from populating the cache
- Process bounded ERP JSON and binary responses through ordered
  `raw_response` plugins before normalization, response-path selection, schema
  validation, and legacy `after_response` processing
- Require secure admission for `raw_response` bindings, including authenticated
  admin control-plane access, allowlisted plugin endpoints, HTTP-backed tools,
  and explicit object-shaped output schemas
- Add the versioned `raw_response` plugin invocation contract with bounded,
  tagged JSON/base64 ERP response bodies while preserving legacy
  `after_response` payloads
- Add `bridgectl skill install` with embedded `bridgectl-ops` Agent Skill files,
  global, project-scoped, and explicit-directory destinations, and safe forced
  replacement.
- Add a green-accent operational console workspace with grouped navigation,
  responsive mobile navigation, context-aware overview dashboard, session-local
  metric trends, label-safe metric tables, and consistent stale/empty states
- Add compact connected topology overviews whenever endpoint components exist,
  with selected-context registry resolution, component-menu drill-down,
  diagnostic match states, raw relationship preservation, and closed safety-cap
  edges without exposing full URLs, credentials, or raw configuration
- Add topology search and facet filters, shared node/edge selection,
  relationship highlighting, safe edge inspection, and incomplete-graph
  warnings without exposing endpoints, credentials, or raw configuration
- Add client-side pagination with page ranges for tools, plugins, bindings,
  topology relationships, logs, and metrics after filtering or sorting
- Add read-only plugin and binding metadata views, clickable exact-version
  plugin detail pages, and plugin-aware topology nodes without exposing
  endpoints, credentials, or raw configuration

### Changed

- Keep configured Redis selected when unavailable, degrade cache reads/writes
  without false hits, return safe `503 HEALTH_CHECK_FAILED` cache health, and
  reject malformed Redis URLs before server startup.
- Preserve OpenAPI request schemas and parameter locations in generated tools,
  protect connector-managed headers, infer `data` unwrapping from resolved
  schemas, and accept successful HEAD/204 responses without JSON bodies.
- Flush REST log-stream headers immediately, stop on client write failures, and
  document bounded subscriber drop behavior separately from Console projection.
- Return direct tool rate-limit rejections as HTTP `429` with stable
  `RATE_LIMITED` errors and `Retry-After`, while MCP `tools/call` keeps its
  HTTP-successful `isError` result contract; validate rate settings strictly.
- Scope local API registries to the selected context, require explicit
  credential-safe migration for legacy registries, and require `--force` for
  intentional duplicate replacement
- Keep generated tool manifests pure until explicitly written or applied; the
  `generate-tools` workflow now applies one temporary YAML stream exactly once
  and cleans up temporary artifacts

## [v0.4.0-alpha.1] - 2026-08-26

### Added

- Add the generic external-plugin contract with exact-version bindings and
  persisted plugin and binding resources
- Add admin plugin and binding control-plane APIs with reconciliation, cache
  invalidation, and typed `bridgectl plugin` management commands
- Process successful tool responses through ordered external plugin bindings
  while preserving MCP and direct-invoke envelopes
- Add an isolated external-plugin integration fixture with authenticated
  control-plane and runtime checks

### Fixed

- Create the `.bridgectl` configuration directory before saving a context

### Security

- Require secure transport for credentialed ERP and plugin calls except on
  configured exact development hosts
- Remove plaintext ERP registry credentials and redact ERP and plugin request
  data from logs and process arguments

## [v0.3.0-alpha.4] - 2026-08-26

### Added

- Add the loopback-only read-only `bridgectl web` console command, including
  per-launch capability protection, browser security headers, and graceful
  local lifecycle
- Add the pinned React, TypeScript, Vite, Tailwind CSS, and testing toolchain
  for the embedded console frontend
- Add semantic light, dark, and system themes with accessible status primitives
  for the console
- Embed hashed frontend assets in `bridgectl` builds and verify release-like
  binaries against the fallback asset and compressed-size budget
- Add explicit `bridgectl web --dev` proxying for local Vite development
- Add capability-protected context and deployment projections without upstream
  credentials or arbitrary proxying
- Add safe health, MCP tool inventory, and cache statistics projections with
  fixed upstream route mapping
- Add bounded, redacted recent-log and SSE projections with stream limits
- Add typed live Prometheus snapshots with session-local rates and average
  latency from histogram sum and count
- Add the console application shell, routes, deployment selector, and safe
  context switching without changing persistent CLI state
- Add bounded live log filters and current-session metric tables with accessible
  text alternatives
- Add bounded API-to-MCP topology aggregation with exact, base-prefix,
  ambiguous, and unresolved match states
- Add a lazy-loaded topology canvas with keyboard-accessible relationship and
  path tables
- Add authenticated safe server build and runtime metadata with older-server
  fallback handling
- Add clickable Tools inventory entries and read-only, user-friendly manifest
  detail pages with safe input, execution, security, routing, and lifecycle
  projections
- Default the console to light mode, add a collapsible sidebar, and brand the
  interface as ERPBridge Console
- Add a homepage notice that directs monitoring to the console and configuration
  changes to `bridgectl`
- Sort Logs page events with the most recent valid timestamps first

### Changed

- Keep the Tailwind 3 toolchain paired with `tailwind-merge` 2.6, align Node
  types with the Node 22 runtime, and pin the compatible React Flow release so
  production asset builds remain deterministic.

### Fixed

- Keep authenticated upstream response bodies readable until each console
  projection or stream finishes consuming them.
- Render the safe MCP tool inventory on the Tools route instead of showing a
  placeholder page.
- Treat root API registrations as non-authoritative topology base prefixes
  across generated tool HTTP methods.

## [v0.3.0-alpha.3] - 2026-08-25

### Added

- Add environment-backed ERP API credential references, `api set-credential-ref`, and confirmed legacy credential scrubbing without plaintext backups.
- Add typed plugin metadata and protected bearer or API-key credential references, with bounded authenticated requests, for external plugin endpoints.
- Add the initial generic external-plugin and exact-version binding contract with bounded HTTP JSON requests.
- Persist versioned plugin and binding resources with soft-delete and active-reference protection.
- Add admin-only plugin and binding APIs with exact-reference admission, reconciliation, and cache invalidation.
- Process successful tool responses through ordered external bindings while preserving MCP and direct-invoke envelopes.
- Add `bridgectl plugin` and `bridgectl plugin binding` resource-management commands with typed JSON/YAML handling.
- Add an isolated black-box integration fixture with the external `mock-plugin` polyrepo service, MockERP, and repeatable Compose cleanup.
- Protect the real external-plugin integration fixture with generated API keys, authenticated control-plane requests, and missing/wrong/correct-key checks.
- Document the generic external-plugin control plane, HTTP protocol, deployment boundary, and Docker integration workflow.
- Add agent-integration guidance for Codex CLI, OpenCode, OpenClaw, and Hermes
  Agent, including scoped HTTP bearer authentication and stdio credential
  boundaries

### Changed

- Pin the standalone MockERP image and versioned OpenAPI contract to `0.2.1`,
  which includes the SQLite-backed SCP scenario and integration fixtures
- Document the MockERP integration contract, credentials boundary, reset flow,
  and supported fixture groups
- Pass MockERP credential-source settings through Compose and persist its named
  data volume configuration

### Fixed

- Require HTTPS for credentialed ERP and plugin calls, except configured exact development hosts.
- Redact sensitive attributes in root log sinks and stop logging ERP request and response bodies.
- Keep ERPBridge stdio stdout reserved for MCP JSON-RPC by routing the startup
  banner to stderr

## [v0.3.0-alpha.2] - 2026-08-23

### Fixed

- Initialize MCP tool invocation and duration metric series at registration so
  cold-start `/metrics` scrapes expose zero-valued samples.

## [v0.3.0-alpha.1] - 2026-08-22

### Added

- Add bounded in-memory caching when Redis is not configured, with full-hash keys, cache timestamps, and backend-aware flushing
- Apply generated YAML sequences and multi-document YAML streams, with exact server-side tool filtering
- Add hashed, scoped API tokens with expiry, revocation, admin roles, and one-time token disclosure
- Add bearer-token precedence and `bridgectl token create`, `list`, and `revoke` commands
- Add optional per-tool role allow-lists with MCP discovery and direct-invoke selectors
- Add the `bridgectl-ops` operations skill for onboarding, maintenance, diagnosis, and sanitized bug reporting

### Changed

- Document MCP result envelopes, registry-only REST invocation, memory-cache behavior, and the current tool schema template
- Document authenticated route policy, role selectors, CORS, token operations, and CLI configuration

### Fixed

- Fail closed for unresolved credential references and redact tool arguments across log handlers
- Hide inactive tools on Stdio, harden registry state hashing and migrations, evict idle rate-limit identities, and validate nested response paths
- Dereference generated output schemas and report unresolved OpenAPI references with operation context
- Gracefully shut down the HTTP listener on SIGTERM and SIGINT

### Security

- Redact authorization headers and prevent denied guarded calls from reaching cache or downstream ERP execution

## [v0.2.0-alpha.5] - 2026-05-10

### Added

- feat(db): add `HardDelete` support to SQLite store for permanent tool removal
- feat(server): support `hard=true` parameter in tool delete API
- feat(cli): enhance `bridgectl tool delete` with `--hard` flag and interactive confirmation
- feat(cli): add `-y, --yes` flag to bypass hard-delete confirmation
- test: add comprehensive tests for hard-delete in database, server, and CLI layers

### Changed

- docs: update `tool delete` documentation and onboarding guide with hard-delete instructions
- style(cli): improve `tool delete` error messaging for missing arguments
- style(cli): improve `tool delete` success output with descriptive states

## [v0.2.0-alpha.4] - 2026-05-10

### Added

- docs: add comprehensive "Onboarding New APIs" guide with troubleshooting and quick reference
- feat(cli): add recursive directory support for `bridgectl tool apply -f`

### Changed

- docs: update README.md index to include onboarding guide
- docs: polish and expand onboarding guide for better developer experience

### Fixed

- fix(cli): resolve issue where `bridgectl tool apply` failed on directory paths

## [v0.2.0-alpha.3] - 2026-05-10

### Changed

- ci: clean up GoReleaser indentation
- docs: add comprehensive GitHub badges to README

## [v0.2.0-alpha.2] (Observability & Hardening) - 2026-05-10

### Added

- feat(mcp): implement Server Hooks for lifecycle telemetry and business logic
- feat(mcp): implement deletion reconciliation (automatic tool deregistration)
- feat(mcp): add tool icons and output schema validation
- feat(metrics): add server lifecycle and session Prometheus metrics
- feat: add ASCII startup banners for server and CLI
- feat: add Makefile for automated setup, build, test, and tool generation
- test: achieve high coverage (>70%) across core packages (MCP, IDP, Cache, CLI, Output)
- ci: modernize GoReleaser config with multi-platform builds, SBOM (Syft), and Signing (Cosign)

### Changed

- refactor(mcp): optimize reconciliation loop with state-hash short-circuiting
- refactor(mcp): migrate built-in system tools to type-safe structured handlers
- chore(ci): harden production Dockerfile with non-root user and pinned Alpine 3.22.4
- chore: ignore schemas directory in git (transition to DB-driven registry)
- ci: update GitHub Actions to latest major versions (checkout@v6, setup-go@v6, goreleaser@v7)
- ci: limit GoReleaser targets to linux/amd64 and standardize configuration quotes

### Fixed

- fix(mcp): ensure graceful shutdown of reconciliation controller via signal-aware context
- fix(logger): safely handle nil context in FromContext to prevent panics
- fix(ci): resolve Cosign version resolution and bundle signing issues

## [v0.2.0-alpha.1] (Declarative V2) - 2026-05-09

### Added

- feat(mcp): implement Declarative Control Plane with SQLite registry
- feat(mcp): add SemVer-based tool versioning and resolution
- feat(mcp): implement background reconciliation controller
- feat(cli): add `bridgectl tool apply`, `get`, `delete`, and `describe`
- feat(cli): add dynamic shell auto-completion for tool resources
- feat(idp): enhance OpenAPI generator with semantic naming and pluralization
- docs: add V2 architecture overview and schema reference guides

### Changed

- refactor(mcp): migrate from file-system hot-reloading to API-driven registry
- refactor(security): decouple secrets from schemas using `credentialRef`
- chore(config): migrate all technical ERP schemas to semantic intent-based V2 format

### Fixed

- fix(cli): resolve lint issues and missing error checks for HTTP body close
- fix(test): update all test suites to align with V2 architecture

## [v0.1.0-alpha.4] (Kotiya) - 2026-05-08

### Added

- feat(mcp): implement custom notification system
- feat(core): add middleware infrastructure and native tool support
- docs: implement wiki-style documentation and docker guide

### Fixed

- fix(mcp): support recursive directory watching for schema hot reloading

### Changed

- refactor(mcp): migrate server logic to middleware-based architecture
- feat(mcp): remove deprecated SSE support in favor of Stdio and Streamable HTTP
- docs: clarify that schema hot reloading supports nested directories

### Improved

- test(mcp): improve test coverage for notifications and endpoints

## [v0.1.0-alpha.3] - 2026-05-08

### Added

- feat(mock-erp): enhance OpenAPI spec for better MCP tool integration
- feat(server): support STDIO transport via --stdio flag and MCP_TRANSPORT env var
- feat(mcp): add client logging, tool notifications, and completion providers
- feat(logger): support redirecting logs to stderr via LOG_TO_STDERR env var
- feat(mcp): implement native streamable http transport support
- docs: add connectivity and transport guide

### Fixed

- fix(docker): resolve build issues and dynamic URL routing
- fix(mcp): resolve tool marshaling conflict by clearing structured schema fields
- fix(logger): use t.Setenv and remove unused os import in tests

### Changed

- refactor(mcp): remove unused resource and prompt completion handlers
- style(cli): standardize error messages and improve context usage
- style(idp): update GenerateFromOpenAPI to accept context
- style(logger): unexport sensitiveKeys to improve encapsulation

### Improved

- docs(cli/mcp/connector): add documentation comments and enable linting
- test: add unit tests for logger, metrics, and cli errors

## [v0.1.0-alpha.2] - 2026-05-07

### Fixed

- fix: include erpbridge-server source and fix gitignore patterns
- fix(goreleaser): update repo owner to nmdra

### Changed

- chore: rename middleware to erpbridge-server
- docs: update README with package details and rename middleware to server

## [v0.1.0-alpha.1] - 2026-05-07

### Added

- feat: add lefthook configuration for automated linting, formatting, and testing
- feat(cli): implement agent-friendly improvements with structured errors and isolated JSON output
- feat: implement schema hot-reloading and bridgectl tool validate
- feat: instrument MCP server with metrics and implement stats endpoint
- feat: implement Prometheus metrics and cache statistics
- feat: add support for MCP Resources and Prompts
- feat: implement circuit breaking and intelligent retries in ERPConnector
- feat: switch embedder to nomic-ai/nomic-embed-text-v1
- feat: implement structured logging with slog, request tracing, and CLI log tailing
- feat: configure cache for finance tools
- feat: add bridgectl cache commands and output formatting
- feat: integrate semantic cache with MCP server and tool execution (internal)
- feat: implement role-aware semantic cache manager
- feat: implement openapi generation and response validation
- feat(cli): implement bridgectl CLI with API and tool management
- feat(middleware): implement MCP bridge middleware and ERP connector
- feat(mock-erp): implement mock ERP server with finance, HR, and inventory modules
- docs: enhance CLI self-documentation and add bridgectl doc generator
- docs: update README with resilience, observability, and DX features
- docs: add .env.example with customizable variables
- docs: add comprehensive guide to README.md
- ci: add GoReleaser release pipeline for middleware and bridgectl

### Fixed

- fix(workflow): downgrade go to 1.24 for golangci-lint compatibility
- fix(cli): resolve numerous lint issues (unchecked errors, body close, etc.)
- fix(idp/connector/config/logger): resolve various lint and stability issues

### Changed

- refactor(monorepo): restructure Go code into services/tools
- chore: prepare for versioning and release
- chore: rename middleware to erpbridge-server

[Unreleased]: https://github.com/nmdra/ERPBridge/compare/v0.5.0-alpha.6...HEAD
[v0.5.0-alpha.6]: https://github.com/nmdra/ERPBridge/compare/v0.5.0-alpha.5...v0.5.0-alpha.6
[v0.5.0-alpha.5]: https://github.com/nmdra/ERPBridge/compare/v0.5.0-alpha.4...v0.5.0-alpha.5
[v0.5.0-alpha.4]: https://github.com/nmdra/ERPBridge/compare/v0.5.0-alpha.3...v0.5.0-alpha.4
[v0.5.0-alpha.3]: https://github.com/nmdra/ERPBridge/compare/v0.5.0-alpha.2...v0.5.0-alpha.3
[v0.5.0-alpha.2]: https://github.com/nmdra/ERPBridge/compare/v0.5.0-alpha.1...v0.5.0-alpha.2
[v0.5.0-alpha.1]: https://github.com/nmdra/ERPBridge/compare/v0.4.0-alpha.3...v0.5.0-alpha.1
[v0.4.0-alpha.1]: https://github.com/nmdra/ERPBridge/compare/v0.3.0-alpha.4...v0.4.0-alpha.1
[v0.3.0-alpha.4]: https://github.com/nmdra/ERPBridge/compare/v0.3.0-alpha.3...v0.3.0-alpha.4
[v0.3.0-alpha.3]: https://github.com/nmdra/ERPBridge/compare/v0.3.0-alpha.2...v0.3.0-alpha.3
[v0.3.0-alpha.2]: https://github.com/nmdra/ERPBridge/compare/v0.3.0-alpha.1...v0.3.0-alpha.2
[v0.3.0-alpha.1]: https://github.com/nmdra/ERPBridge/compare/v0.2.0-alpha.5...v0.3.0-alpha.1
