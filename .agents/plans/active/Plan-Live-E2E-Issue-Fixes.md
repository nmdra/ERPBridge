# Plan: Live E2E regression remediation

## Goal

Fix the confirmed ERPBridge server contract gaps found by the fresh black-box
run, correct the E2E harness findings that were caused by test configuration,
and leave fixture-only limitations explicit. The target is a green focused
regression suite and a repeatable live run with no credential, ERP-body, plugin-
payload, or raw-log leakage.

## Current State

The live run covered 35 capabilities: 27 passed, 6 failed, and 2 were marked
`BLOCKED_FIXTURE` (`../Erpbridge-demo/reports/run-20260827.md:3-9` and
`../Erpbridge-demo/reports/capability-matrix.md:6-45`). The retained sanitized
evidence is under `../Erpbridge-demo/evidence/`; runtime credentials, databases,
and logs were deleted. The protected
`docs/cli/bridgectl_api_scrub-credentials.md` file is already an unstaged
pre-plan change; this plan must preserve that baseline and must not stage or
rewrite it.

Confirmed implementation seams:

- Rate limiting calls `Limiter.Allow()` and returns an MCP `isError` result from
  `RateLimitMiddleware.Handle` (`internal/mcp/middleware.go:85-104`). Direct
  invocation maps every `mcpResult.IsError` to HTTP `502` instead of preserving
  a rate-limit outcome (`internal/mcp/server.go:952-1032`). The live rate phase
  therefore observed no `429` or `Retry-After` (`../Erpbridge-demo/reports/issues/RES-001-rate-limit-not-observed-on-cache-route.md`).
- `/api/logs/stream` sets SSE headers but does not flush until the first event;
  an idle client can time out before receiving headers
  (`internal/mcp/server.go:1116-1138`). The current test uses a
  `ResponseRecorder` and does not exercise a real client connection
  (`internal/mcp/server_test.go:363-386`).
- OpenAPI generation hard-codes `responsePath: data` and maps request bodies
  only from direct `schema.Properties` (`internal/idp/generator.go:234-331`).
  The runtime model is flat (`internal/mcp/tool.go:69-91`), while execution
  puts only GET arguments in the query and all other arguments in a JSON body
  (`internal/mcp/tool.go:150-221`). The live generated POST sweep exposed
  empty body schemas and `502` calls, while a reviewed relative POST probe
  succeeded (`../Erpbridge-demo/reports/issues/GEN-001-openapi-generated-post-and-parameterized-calls.md`).
- A configured Redis manager remains selected, but cache-stat errors are mapped
  to a generic HTTP `500` (`internal/mcp/server.go:1095-1114`). Cache reads
  intentionally suppress backend errors as misses and cache writes log errors
  without failing the tool (`internal/cache/manager.go:59-107`,
  `internal/mcp/middleware.go:168-233`). The root documentation explicitly
  says configured Redis must not silently fall back to memory
  (`docs/api.md:245-255`); the live report incorrectly treated this as a
  fallback failure (`../Erpbridge-demo/reports/issues/RES-002-redis-unavailable-cache-stats.md`).
- Plugin admission requires an authenticated admin and an exact endpoint
  allowlist (`internal/mcp/plugin_api.go:75-121,242-261`), and the client
  resolves `PLUGIN_*` references and sends one bounded authenticated request
  (`internal/mcp/plugin_client.go:53-158`). The sample plugin expects
  `MOCK_PLUGIN_API_KEY` (`../ERPBridge-Plugins/plugins/mock-plugin/main.go:14-16,49-55`),
  while the disposable demo Compose file passed only the server-side
  reference name. The root integration script already exports both names
  (`scripts/test-plugin-integration.sh:20-27`), so this is a harness mapping
  defect, not evidence of an ERPBridge authentication defect.
- The plugin pipeline already implements phase-local ordering and explicit
  `continue`/`fail` behavior (`internal/mcp/plugin_pipeline.go:63-273`), with
  focused tests for both policies (`internal/mcp/server_plugin_test.go:283-321`
  and `:434-469`). The live outage assertion used the `continue` binding while
  expecting `fail` (`../Erpbridge-demo/manifests/bindings/after-response.yaml`
  and `../Erpbridge-demo/reports/issues/PLUGIN-002-plugin-outage-assertion-failed.md`).
  The raw probe also used a response path that removed the sample plugin's
  marker; it did not prove that raw processing was absent
  (`../Erpbridge-demo/reports/issues/PLUGIN-003-raw-response-fixture-not-observable.md`).
- The public sample plugin does not expose deterministic binary, malformed,
  oversize, redirect, non-2xx, or timeout modes. These remain fixture
  limitations, not production behavior to simulate by weakening transport
  safety (`../Erpbridge-demo/reports/issues/PLUGIN-004-sample-plugin-fault-modes-blocked.md`;
  `../Erpbridge-demo/reports/issues/RES-003-upstream-fault-fixtures-blocked.md`).

External contract references used for the design:

- MCP Streamable HTTP: <https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/transports/>
- Go token-bucket semantics: <https://pkg.go.dev/golang.org/x/time/rate>
- go-redis error handling and graceful degradation: <https://redis.io/docs/latest/develop/clients/go/error-handling/>

## Decisions

- Direct `/api/tools/invoke` rate-limit rejections will use HTTP `429`, a
  stable `RATE_LIMITED` error identifier, and a `Retry-After` value computed
  from the limiter's reservation delay in whole seconds (minimum `1`).
  Streamable HTTP MCP tool calls will remain HTTP-successful protocol
  responses with `result.isError=true`, because tool errors belong inside MCP
  results. The direct path will carry a typed internal outcome rather than
  matching error text; the outcome will never be serialized into plugin or ERP
  payloads.
- Authenticated rate limits will remain keyed by token principal, while an MCP
  session remains the fallback identity for unauthenticated/stateful calls.
  Documentation and tests will state this distinction instead of calling all
  limits “per-session.”
- The REST SSE endpoint will flush headers immediately, check write failures,
  and retain its authenticated structured-log contract. The Console BFF will
  remain the projection/redaction boundary; it will not be replaced by raw
  REST log output.
- Generated tools will preserve OpenAPI request-body structure and parameter
  locations. Existing hand-authored manifests remain compatible when the new
  mapping metadata is absent. Response unwrapping will be emitted only when
  the response schema proves the wrapper, and the post-path output schema will
  match the unwrapped value.
- An explicitly configured but unavailable Redis backend will never become a
  memory backend. Tool execution remains best-effort without cache data, while
  cache health returns a bounded HTTP `503` with the existing
  `HEALTH_CHECK_FAILED` identifier. Invalid cache configuration will not be
  silently converted into a nil cache manager.
- The sample plugin's server pipeline contract will not be changed to work
  around a bad fixture assertion. The demo harness will pass the plugin's
  expected environment variable, use a `fail` binding for fail-policy tests,
  and make raw processing observable with a safe object-shaped probe.
- Unsupported fault modes remain `BLOCKED_FIXTURE` unless a disposable,
  documented fixture is added. No redirects, retries, broad allowlists, or
  real endpoints will be introduced to force a live pass.
- Every behavior change gets adjacent TDD coverage, an atomic Conventional
  Commit, root documentation plus the matching public-docs update, and no
  changes to the protected `docs/cli/bridgectl_api_scrub-credentials.md` file.

## Scope

### In scope

- ERPBridge rate-limit transport semantics, SSE connection establishment,
  OpenAPI schema/mapping generation, and Redis health/error signaling.
- Focused unit/integration tests at the existing MCP, connector, cache, logger,
  and generator seams.
- Corrective changes to the disposable `Erpbridge-demo` runner, Compose
  environment mapping, reviewed probes, and sanitized expected outcomes.
- Synchronization of `docs/`, `CHANGELOG.md`, and the corresponding
  `../erpbridge-docs` guides.
- A fresh post-fix black-box rerun and sanitized evidence/cleanup verification.

### Intentionally out of scope

- Changing the MCP protocol version or replacing `mcp-go`.
- Adding production plugin installation, scheduling, retries, or redirects.
- Changing MockERP's published image/contract or using `Bridgectl-Demo`.
- Treating absent sample-plugin fault switches as product failures.
- UI redesign, SDK integration work, or unrelated pre-existing lint findings.
- Any credential value, ERP record, plugin payload, response body, token, hash,
  caller identity, or raw service log in reports or committed artifacts.

## Tasks

- [x] **Task 1: Correct the regression harness and classify observed outcomes.**
  Update the live test workspace so it tests the real contracts before changing
  server code: send rate-limit bursts through healthy `tools/call` and direct
  probes, map `PLUGIN_MOCK_API_KEY` into the sample plugin service while using
  the server-side `PLUGIN_*` reference, create separate `continue` and `fail`
  binding manifests, and use a raw probe with no wrapper-stripping
  `responsePath`. Prepare the final RES-002 classification for Task 7 after
  the cache contract is fixed; retain RES-003 and PLUGIN-004 as explicit
  fixture blocks. **Seam:** fresh Compose phase, direct HTTP, Streamable HTTP,
  plugin `/v1/process`, and sanitized matrix evidence. **Files:**
  `../Erpbridge-demo/compose/mock-plugin.compose.yml`,
  `../Erpbridge-demo/scripts/run-phase.sh`,
  `../Erpbridge-demo/scripts/test-rate-limit.py`,
  `../Erpbridge-demo/scripts/test-plugins.py`,
  `../Erpbridge-demo/scripts/test-plugin-failure.py`,
  `../Erpbridge-demo/manifests/bindings/`,
  `../Erpbridge-demo/manifests/live-e2e/custom/`,
  `../Erpbridge-demo/reports/capability-matrix.md`, and the affected issue
  reports. **Verify:** `bash -n ../Erpbridge-demo/scripts/*.sh`,
  `python3 -m py_compile ../Erpbridge-demo/scripts/*.py`, and a fresh harness
  dry run records no raw payloads or credential markers.

- [x] **Task 2: Add a transport-aware rate-limit contract.** Start with failing
  tests for two direct calls, two MCP sessions, shared authenticated principals,
  separate principals, `Retry-After`, and denied calls not reaching the
  connector/cache. Add a stable `ErrorRateLimited` identifier and an internal
  typed rate-limit outcome; preserve MCP `isError=true` while mapping only the
  direct REST outcome to `429` with `Retry-After`. Obtain the delay from a
  reservation that is cancelled when the request is rejected; use `1` second
  only when the reservation cannot provide a delay. Validate positive,
  finite RPS and burst values during server configuration, including rejection
  of trailing-invalid numeric input before server construction. Keep
  `/tools/list` outside tool-execution throttling and test at least one
  sub-second and one low-RPS configuration. **Seam:**
  `RateLimitMiddleware.Handle` through `handleDirectInvoke` and the Streamable
  HTTP handler. **Files:** `internal/mcp/middleware.go`,
  `internal/mcp/server.go`, `internal/mcp/errors.go`,
  `internal/mcp/middleware_test.go`, `internal/mcp/server_test.go`,
  `internal/mcp/errors_test.go`, `services/erpbridge-server/main.go`,
  `services/erpbridge-server/main_test.go`, `docs/connectivity.md`, `docs/api.md`,
  `../erpbridge-docs/docs/erpbridge/connectivity.mdx`,
  `../erpbridge-docs/docs/erpbridge/api.mdx`, and `CHANGELOG.md`. **Verify:**
  `go test ./internal/mcp -run 'RateLimit|DirectInvoke' -count=1`,
  `go test ./services/erpbridge-server -run 'Rate|Config' -count=1`, plus a
  fresh live phase where direct bursts contain `429` and a reservation-derived
  `Retry-After`, while MCP tool calls remain protocol `200` with `isError=true`.

- [x] **Task 3: Make REST SSE connections establish and terminate cleanly.**
  Write failing real-client tests that receive response headers before any log
  event, receive one correctly framed event, cancel the request, and detect a
  client write failure. Flush after setting headers, stop on `fmt.Fprintf`
  errors, and preserve unsubscribe-on-exit. Use a controlled failing
  `ResponseWriter` test for the write-error branch; use a real client for
  header flush, event framing, and cancellation. Document and test the
  existing bounded 100-event subscriber channel's drop-on-full policy rather
  than leaving slow-subscriber behavior implicit. Keep recent-log responses
  and the Console BFF's projection/redaction behavior separate. **Seam:** `handleLogStream` over an `httptest.Server` and
  the logger subscription channel. **Files:** `internal/mcp/server.go`,
  `internal/mcp/server_test.go`, `internal/logger/logger.go`,
  `internal/logger/logger_test.go`, `internal/cli/log.go`,
  `internal/cli/log_test.go`, `docs/api.md`, `docs/connectivity.md`,
  `docs/docker.md`, `../erpbridge-docs/docs/erpbridge/api.mdx`,
  `../erpbridge-docs/docs/erpbridge/connectivity.mdx`, and
  `CHANGELOG.md`. **Verify:**
  `go test ./internal/mcp ./internal/logger ./internal/cli -run 'Log|SSE|Stream' -count=1`
  and a real HTTP client receives headers within one second on an idle stream.

- [x] **Task 4: Preserve OpenAPI request and response schemas.** Add failing
  generator/tool tests for `$ref` request bodies, nested objects, arrays,
  enums, defaults, required fields, path/query/header parameters, HEAD and
  non-HEAD `204 No Content` behavior, top-level non-`data` responses, and the
  existing wrapped MockERP
  response. Define a backward-compatible execution contract before implementation:
  preserve `Execution.Mapping` as the legacy LLM-name-to-ERP-name map and add
  generated `ParameterLocations` keyed by the LLM argument name, with values
  `path`, `query`, `header`, or `body`. The MCP input remains a flat object for
  object request bodies; body fields are serialized as one JSON object, while
  primitive/array request bodies use one generated `body` argument serialized
  as the complete JSON body. Arguments with the same mapped ERP name are
  allowed across different locations but are rejected during generation when
  they collide within one location. If two locations share a source name,
  retain the unsuffixed name only when there is no collision; otherwise use
  deterministic `${name}__${location}` names for every colliding location,
  with a stable numeric suffix if a source name already occupies that name.
  Merge path-item and operation parameters using OpenAPI override rules, then
  apply that collision rule. Old manifests with no location metadata retain
  the current GET-query/non-GET-JSON-body fallback. URL path values are
  escaped. Add allowlisted generated headers to `EndpointConfig`; reject
  `Authorization`, `Proxy-Authorization`, `Cookie`, `Host`, connection,
  transfer, upgrade, content-length, and content-type parameters during
  generation, and never let generated values override connector auth or
  transport headers. Dereference request bodies before recursive conversion,
  including nested objects, arrays, enums, defaults, and required fields, and
  assert the serialized schema sent by `tools/list`, not only generator
  structs. Infer response unwrapping only when the resolved top-level response
  schema is an object with an exact `data` property; otherwise omit
  `responsePath`. Nested `data` properties and unrelated sibling metadata do
  not trigger unwrapping. Emit the unwrapped `data` schema as the output
  schema. For any successful HEAD or non-HEAD `204 No Content` response, skip
  JSON decoding and return a successful nil result; malformed non-empty JSON
  remains an error. Generation-only mapping metadata is not exposed through
  Console projections, so no web DTO change is required. **Seam:**
  `Generator.GenerateFromOpenAPI` → generated `MCPTool` → `Tool.prepareERPCall`
  and `Tool.Execute` against an `httptest.Server`. **Files:**
  `internal/idp/generator.go`, `internal/idp/generator_test.go`,
  `internal/mcp/tool.go`, `internal/mcp/tool_test.go`,
  `internal/mcp/server.go`, `internal/mcp/server_test.go`,
  `internal/connector/client.go`, `internal/connector/client_test.go`,
  `docs/tool-schema.md`, `docs/api.md`,
  `../erpbridge-docs/docs/erpbridge/tool-schema.mdx`,
  `../erpbridge-docs/docs/erpbridge/api.mdx`, and `CHANGELOG.md`.
  **Verify:** `go test ./internal/idp ./internal/mcp -run 'Generator|PrepareERPCall|ResponsePath|OpenAPI' -count=1`,
  followed by a fresh MockERP generation/apply/call where the reviewed POST
  probe succeeds and generated body properties are non-empty.

- [x] **Task 5: Make Redis health failure explicit without silent fallback.**
  Add failing `miniredis` tests for closed-backend stats, cache GET/SET errors,
  cache-miss continuation, no cache population after backend failure, and
  memory/Redis backend labels. Return HTTP `503` plus safe
  `HEALTH_CHECK_FAILED` from cache stats when the selected Redis backend is
  unavailable; preserve best-effort tool execution without a false cache hit;
  ensure configured Redis is never replaced by memory; and make malformed
  `REDIS_URL` startup failure explicit instead of leaving a nil manager. Extract
  cache initialization from `main` so a subprocess test can assert that an
  invalid configured URL returns before a listener or server is started.
  **Seam:** `cache.Manager`, `CacheMiddleware`, server cache routes, and
  service startup. **Files:** `internal/cache/manager.go`,
  `internal/cache/manager_test.go`, `internal/cache/redis_backend.go`,
  `internal/cache/flush.go`, `internal/cache/flush_test.go`,
  `internal/mcp/middleware.go`, `internal/mcp/middleware_test.go`,
  `internal/mcp/server.go`, `internal/mcp/server_test.go`,
  `services/erpbridge-server/main.go`, `docs/api.md`,
  `docs/environment-variables.md`, `../erpbridge-docs/docs/erpbridge/caching.mdx`,
  `../erpbridge-docs/docs/erpbridge/api.mdx`, and `CHANGELOG.md`.
  **Verify:** `go test ./internal/cache ./internal/mcp -run 'Cache|Redis' -count=1`,
  `go test ./services/erpbridge-server -count=1`, and a fresh Redis-unavailable
  phase showing health `200`, cache stats `503`, tool execution bounded, and no
  memory backend substitution.

- [x] **Task 6: Close plugin authentication and phase-coverage gaps.** Add a
  black-box regression to the existing plugin integration test that asserts
  missing/wrong/correct `X-API-Key` statuses without printing the key. Correct
  the disposable demo Compose mapping to `MOCK_PLUGIN_API_KEY`, use a
  `failurePolicy: fail` binding when stopping the plugin and assert direct
  HTTP `500` plus MCP `isError=true`, then use `continue` to assert safe
  original-result fallback. Make the raw probe's final schema retain the
  sample plugin's `processedBy` marker and assert raw-before-normalization via
  both direct and MCP calls. Preserve the existing server unit tests; only
  change `internal/mcp/plugin_pipeline.go` if a corrected fail-policy live test
  contradicts its current contract. **Seam:**
  `PluginClient.Process`, `executeRawTool`,
  `processAfterResponseBindings`, `handleDirectInvoke`, and the real sample
  plugin `/v1/process`. **Files:** `internal/mcp/plugin_client_test.go`,
  `internal/mcp/server_plugin_test.go`, `internal/integration/plugin_system_test.go`,
  `scripts/test-plugin-integration.sh`,
  `../ERPBridge-Plugins/plugins/mock-plugin/main.go`,
  `../ERPBridge-Plugins/plugins/mock-plugin/main_test.go`,
  `../Erpbridge-demo/compose/mock-plugin.compose.yml`,
  `../Erpbridge-demo/manifests/plugins/`,
  `../Erpbridge-demo/manifests/bindings/`,
  `../Erpbridge-demo/scripts/test-plugins.py`, and the affected sanitized
  reports. **Verify:** `go test -tags pluginintegration ./internal/integration -run TestPluginSystemBlackBox -count=1`,
  `make test-plugin-integration`, and a fresh demo phase with `401/401/200`
  plugin auth results, after-response markers, raw-probe markers, and no
  secret markers.

- [x] **Task 7: Synchronize contracts, fixture limitations, and release notes.**
  Update the root API/connectivity/plugin/onboarding/environment guides with
  the final rate-limit status split, initial SSE flush, generator mapping and
  response-path rules, Redis-unavailable behavior, plugin environment names,
  and the exact distinction between raw REST logs and projected Console logs.
  Mark timeout/circuit and unsupported sample-plugin fault modes as
  `BLOCKED_FIXTURE`, correct the RES-002/PLUGIN-002/PLUGIN-003 harness
  classifications, and update `CHANGELOG.md` Unreleased. Apply matching edits
  in the public `erpbridge-docs` repository; do not touch the protected scrub
  document. Record its pre-existing unstaged diff as a baseline and verify
  that no task commit includes or changes that file. **Seam:** documentation examples, issue links, and generated
  public-doc builds. **Files:** `docs/api.md`, `docs/connectivity.md`,
  `docs/docker.md`, `docs/tool-schema.md`, `docs/plugin-schema.md`,
  `docs/onboarding.md`, `CHANGELOG.md`,
  `../erpbridge-docs/docs/erpbridge/api.mdx`,
  `../erpbridge-docs/docs/erpbridge/connectivity.mdx`,
  `../erpbridge-docs/docs/erpbridge/caching.mdx`,
  `../erpbridge-docs/docs/erpbridge/plugins.mdx`,
  `../erpbridge-docs/docs/erpbridge/onboarding.mdx`,
  `../erpbridge-docs/docs/erpbridge/environment-variables.mdx`,
  `../Erpbridge-demo/reports/`, and
  `../Erpbridge-demo/scripts/finalize-report.sh`. **Verify:**
  `npm run build --prefix ../erpbridge-docs`, the demo finalizer's
  case-insensitive marker scan, and
  `bash -euo pipefail -c 'for path in ../Erpbridge-demo/reports
  ../Erpbridge-demo/evidence; do gitleaks dir --redact --no-banner "$path";
  done'` return clean without printing matched content; any scanner error or
  match fails the task, and all linked reports exist.

- [ ] **Task 8: Re-run the complete live matrix and close the remediation plan.**
  Build from the fixed ERPBridge revision, use a new Erpbridge-demo workspace
  and unique Compose projects, execute onboarding, REST, HTTP MCP, stdio,
  authz, cache, resilience, CORS, and plugin checks, and retain only aggregate
  sanitized evidence. Require all confirmed product rows to PASS; retain only
  explicitly linked fixture limitations. Run focused lint on changed Go
  packages, the full Go suite, the plugin integration test, and public docs
  builds. Remove only uniquely labeled containers, networks, volumes, runtime
  credentials, drafts, databases, and raw logs; verify no task changed or
  staged the protected file beyond its recorded baseline. **Seam:** final matrix, evidence, cleanup, and release-readiness
  report. **Files:** `../Erpbridge-demo/scripts/`,
  `../Erpbridge-demo/evidence/`, `../Erpbridge-demo/reports/`,
  `../Erpbridge-demo/reports/run-20260827-fixed.md`, and the completed plan filename. **Verify:**
  `make test`, `go test ./...`, focused `golangci-lint run` on changed
  directories, `make test-plugin-integration`,
  `npm run build --prefix ../erpbridge-docs`, the fresh black-box runner, and
  zero remaining resources for every unique test project. Confirm the
  protected scrub document still matches its pre-plan baseline and was not
  staged by any task.

## Verification

1. Before Task 1, capture a non-committed protected-file baseline outside
   reports/evidence, for example with
   `sha256sum -- docs/cli/bridgectl_api_scrub-credentials.md | cut -d' ' -f1 > .git/info/erpbridge-protected-scrub.baseline`.
   Do not print or publish the digest. This plan is promoted to
   `.agents/plans/active/`; execute tasks in order. One plan task produces one
   atomic Conventional Commit; documentation changes are included in the same
   task and mirrored in the public docs repository.
2. Focused tests must pass before each task is marked complete. The final gates
   include both `make test` and `go test ./...`; lint is limited to changed Go
   directories, per `AGENTS.md`.
3. The post-fix live matrix must contain no unexplained `FAIL` rows. Fixture
   limitations may remain only when linked to a report that states the missing
   public fixture and stores no raw payload.
4. Rate-limit acceptance: direct REST `429` plus `Retry-After`; MCP tool calls
   remain JSON-RPC success with `isError=true`; `/tools/list` is not throttled.
5. SSE acceptance: authenticated clients receive headers without waiting for
   an event, receive valid framing, and disconnect without leaked subscribers.
6. Generator acceptance: fresh generated POST request schemas contain their
   required body fields; path/query/header/body mappings are preserved; wrapped
   and top-level responses validate against the correct post-path schema; and
   both HEAD and non-HEAD `204 No Content` responses succeed without JSON
   decoding.
7. Cache acceptance: Redis hit/miss/TTL/flush behavior remains correct;
   unavailable Redis is never replaced with memory; cache health is bounded and
   stable; tool results do not become false cache hits.
8. Plugin acceptance: the real fixture returns `401/401/200` for missing,
   wrong, and correct keys; after-response and raw probes are observable and
   schema-valid; `continue` and `fail` semantics are tested through MCP and
   direct invocation; no plugin payload or secret is retained.
9. Final security checks fail closed on any scanner error or
   credential/token/header/body marker in reports or evidence, Docker label
   queries find zero resources for all unique live-test projects, and the
   protected scrub document's pre-existing digest matches the private baseline
   and remains unstaged.

## Open Questions

None. The direct REST `429` contract, Redis `503` health response, principal-
keyed authenticated limiter, and fixture-only classification are selected
above and should not be reopened during implementation without a new plan.
