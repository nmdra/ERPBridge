# Plan: Fix Onboarding Reliability, CLI/Server Errors, and Agent Guidance

## Goal

Make the first-run ERPBridge workflow reliable for a human or AI agent:
start a safe local stack, select the intended context, register and test an
ERP API through the server, generate one reviewable manifest, apply it to the
correct control-plane URL, and receive a safe, actionable error when a
precondition fails. Keep credentials and ERP data out of logs, CLI output,
error envelopes, manifests, and console projections.

This plan extends the completed raw-response and plugin work. It includes the
remaining `bridgectl-ops` improvements needed for this onboarding workflow and
must not be executed in parallel with the still-active plugin-only plan
`.agents/plans/active/Plan-Bridgectl-Ops-Plugin-Details.md`; that plan should be
archived as completed before this plan is promoted.

## Current State

### Local stack and environment

- The base Compose file passes empty defaults for both MockERP credential
  variables (`docker-compose.yml:2-22`). The pinned MockERP image fails closed
  without one of those sources, so the documented `docker compose up` path can
  leave the upstream container exited instead of failing before startup.
- The stack uses Docker-only `ERP_BASE_URL=http://mock-erp:8081` but publishes
  MockERP on a host port and passes `ERP_PRIMARY_KEY` through Compose
  (`docker-compose.yml:40-50`). The integration script already demonstrates a
  safe generated credential pair and isolated ports
  (`scripts/test-plugin-integration.sh:8-31`).
- The onboarding guide currently tells users to run `docker compose up --build
  -d` and does not provide a credential bootstrap or recreate step
  (`docs/onboarding.md:14-24`). Compose supports quoted `.env` values and an
  explicit `--env-file`; `docker compose up --force-recreate` is the documented
  way to recreate containers regardless of detected configuration changes:
  [Compose interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/)
  and [`compose up`](https://docs.docker.com/reference/cli/docker/compose/up/).

### Context and local API registry

- `config.Load` replaces every load error with a default config, including
  parse errors, and `ActiveContext` silently returns a default context for a
  missing name (`internal/config/config.go:40-78`). This can hide malformed
  YAML and make the CLI and Console disagree about a dead current context.
- The CLI loads configuration once in `PersistentPreRunE`
  (`internal/cli/root.go:58-81`). The local API registry defaults to one global
  `~/.bridgectl/registry.json` file (`internal/idp/registry.go:42-78`), and
  `Register` writes `r.APIs[api.Name]` without a duplicate check
  (`internal/idp/registry.go:190-218`). Legacy `authKey`/`authToken` fields
  intentionally block writes until scrubbed (`internal/idp/registry.go:34-35,
  153-185`).
- The API list is returned from a map without stable ordering
  (`internal/idp/registry.go:220-228`). The current registry behavior therefore
  combines cross-context collisions, silent overwrites, nondeterministic output,
  and a security migration that is difficult for an agent to recover from.

### API testing and control-plane routing

- `bridgectl api test` resolves the ERP credential in the CLI process and calls
  the registered URL directly (`internal/cli/api.go:174-232`). Its response
  model includes decoded upstream data (`internal/cli/api.go:235-258`), even
  though table output does not display it.
- Tool apply constructs `ctx.MCPServer + "/apis/..."`
  (`internal/cli/tool.go:77-98`). `MCPServer` is also the base for the
  streamable `/mcp/` transport (`internal/mcp/server.go:606-710`), so configuring
  it with `/mcp/` can route a control-plane request into the MCP session
  handler and produce the misleading `Invalid session ID` response.
- The server's control-plane handlers mostly use plain `http.Error`, often
  concatenating internal error strings (`internal/mcp/server.go:713-803` and
  `internal/mcp/plugin_api.go:30-86`). CLI commands then print raw response
  text instead of preserving a machine-readable error class
  (`internal/cli/tool.go:122-142`). MCP tool execution already uses
  `isError: true` for execution failures (`internal/mcp/server.go:554-572`), so
  the new HTTP error contract must not change the MCP protocol envelope.

### Generated manifests

- `tool generate` prints output but `Generator.Generate` and
  `Generator.GenerateFromOpenAPI` also call `Save`, which writes per-tool JSON
  files (`internal/idp/generator.go:38-87`, `internal/idp/generator.go:135-256`,
  `internal/idp/generator.go:322-339`). The Make target then redirects a second
  YAML representation to `schemas/erp/generated.yaml` and recursively applies
  the directory (`Makefile:65-75`). This creates competing generated and
  refined artifacts.
- Generated descriptions currently contain only a short summary, and basic
  generation does not set cache policy or intent examples
  (`internal/idp/generator.go:50-86`). OpenAPI generation does infer response
  schemas, parameters, and request bodies, but its completeness is only
  lightly asserted (`internal/idp/generator_test.go:12-66`).

### Console and product-level security observations

- `bridgectl web` passes one in-memory `*config.Config` to the Console
  handler (`internal/cli/web.go:37-48`, `internal/web/context_api.go:23-40`),
  while `/contexts` reads that snapshot only (`internal/web/context_api.go:65-80`).
  Config edits made while the Console runs are invisible.
- The frontend chooses a context in React state only and does not persist it
  (`web/src/App.tsx:24-42`). The contexts hook fetches once
  (`web/src/hooks/useConsole.ts:27-54`), while inventory and topology hooks
  also fetch once per context (`web/src/hooks/useObservability.ts:133-174`,
  `web/src/hooks/useTopology.ts:75-99`).
- The BFF emits a safe `{error,message}` shape, but `apiFetch` discards it and
  reports only `Console request failed with status N`
  (`internal/web/safe_dto.go:165-169`, `web/src/lib/api.ts:41-50`).
- `RegisterBuiltinTools` always exposes `system.progress_test` and
  `system.sensitive_log_test` (`internal/mcp/server.go:121-185`). `allowedRoles`
  is optional, so a tool such as `list-employees` can be callable without a
  role guard (`internal/mcp/authz.go:47-83`, `internal/mcp/server.go:868-877`).
  Compose publishes RedisInsight on port 8001 by default
  (`docker-compose.yml:24-29`), and `.env.example` includes a password-looking
  placeholder (`.env.example:19-35`).

### Existing guidance

- The current `bridgectl-ops` onboarding requires context capture, confirmation,
  API testing, generated-manifest review, validation, apply, and readback
  (`skills/bridgectl-ops/SKILL.md:15-66`,
  `skills/bridgectl-ops/references/onboarding.md:1-40`). It does not explain
  Compose credential bootstrapping, `.env` shell-sourcing hazards, context
  registry migration, server-side API testing, control-plane versus MCP URLs,
  generated artifact ownership, or the new structured error codes.
- The active plugin-only skill plan already covers plugin lifecycle and plugin
  error guidance. This plan will add onboarding and cross-cutting diagnosis
  guidance without duplicating that plugin contract.

## Decisions

1. **Use a safe development bootstrap instead of fake production credentials.**
   Add a `make dev-up`/script path that generates ephemeral MockERP credentials
   when neither supported credential source is supplied, passes them directly
   to Compose, validates the rendered configuration without printing it, and
   runs `up --build --force-recreate -d`. Do not write generated credentials to
   `.env`, commit them, or change the server's fail-closed behavior. The
   documented bootstrap will preflight the requirement; direct Compose use
   remains an explicit credentialed path and its documentation will explain
   the prerequisite instead of treating an exited MockERP container as a
   mysterious server failure.

2. **Treat `.env` as Compose input, not a shell script.** Do not ask agents to
   `source .env`. The documented workflow will use Compose's `--env-file` and
   will preserve quoted values containing spaces, such as Frappe token values.
   The bootstrap will not echo environment values.

3. **Scope local API registries by context.** Store new registries below
   `~/.bridgectl/registries/<validated-context-name>.json`; API list, register,
   test, generate, scrub, and migrate commands all use the selected context.
   Never silently ignore the legacy global registry. Provide an explicit,
   confirmed migration command that copies modern entries to a chosen context,
   refuses collisions unless explicitly forced, and requires the existing
   scrub command before migrating raw legacy fields. The migration will never
   create a plaintext backup.

4. **Make registration create-or-update explicit.** Duplicate API names return
   a conflict by default. `api register --force` is the only overwrite path and
   prints that an existing definition was replaced. Declarative server tool
   apply remains versioned/upsert behavior, but reports whether it created or
   updated a definition.

5. **Run API tests in the server by default.** `api test` will send only the
   non-secret API definition and `credentialRef` to an authenticated,
   admin-only server probe endpoint. The server resolves the credential in its
   environment, applies the same ERP URL/transport/redirect policy as tool
   execution, and returns status, content type, latency, and success only—never
   an ERP response body. Preserve the old host-side behavior behind an explicit
   `--local` escape hatch for offline diagnostics.

6. **Normalize the configured MCP root for control-plane calls.** Keep the
   existing `mcp-server` field for compatibility, but add one shared CLI helper
   that accepts the root and strips only an exact trailing `/mcp` or `/mcp/`.
   Reject other non-empty paths with `CONTROL_PLANE_URL_INVALID` and a message
   that distinguishes the MCP transport URL from the control-plane root.
   All tool, plugin, binding, and API-probe CLI calls use this helper.

7. **Use one bounded structured HTTP error envelope.** Server control-plane
   errors and CLI remote errors will use `error`, `message`, `suggestion`, and
   numeric `code` fields, with stable error identifiers such as
   `CONTEXT_NOT_FOUND`, `LEGACY_REGISTRY`, `REGISTRY_CONFLICT`,
   `CONTROL_PLANE_URL_INVALID`, `VALIDATION_FAILED`, `AUTHENTICATION_FAILED`,
   `AUTHORIZATION_DENIED`, `UPSTREAM_UNREACHABLE`, and `INSECURE_TRANSPORT`.
   Responses will contain safe remediation text, not raw upstream bodies,
   URLs with secrets, credentials, or internal stack details. MCP execution
   failures continue to be successful protocol responses with `isError: true`,
   consistent with the MCP tools contract:
   <https://modelcontextprotocol.io/specification/2025-06-18/server/tools>.

8. **Make generation pure and artifact ownership explicit.** Generator methods
   return tools without writing files. An explicit save/output operation may
   persist a selected manifest; `make generate-tools` will generate one
   temporary YAML stream, apply that file once, and remove it. Reviewed
   manifests will live under the single documented `manifests/<module>/`
   directory, while generated output remains clearly labeled as a draft.
   Generated drafts
   will include derived intent metadata and safe method-based cache defaults;
   reviewers remain responsible for correctness and roles.

9. **Reload Console configuration without exposing it.** The local Console BFF
   will use a synchronized config-provider seam, refresh its context snapshot on
   a bounded interval and on manual refresh, and retain the last safe snapshot
   when a later file parse fails. It will materialize the same effective context
   rules used by the CLI and never return tokens, auth values, or raw URLs.
   Frontend selection will persist in `sessionStorage`, validate the saved name
   against refreshed contexts, and fall back to the server-marked current
   context or the first valid context. Existing stale-data indicators will be
   extended to inventory pages; errors will preserve safe BFF codes/messages.

10. **Gate development-only and sensitive tools explicitly.** Demo system tools
    will require `MCP_ENABLE_TEST_TOOLS=true`, defaulting to disabled; only the
    development Compose path sets it. Add a `security.dataClass` field and
    require non-empty `allowedRoles` for `pii` and `restricted` tools, then
    update the employee-shaped test fixture, skill manifest template, and
    onboarding guidance. Use opaque role names
    and do not place personal identifiers in them. Bind RedisInsight to
    loopback or make it opt-in rather than publishing it on all interfaces.
   This is an admission and fixture hardening change; it is not a claim that
   the server can infer PII from arbitrary ERP payloads.

## Scope

### In scope

- First-run Compose/bootstrap behavior, quoted environment values, safe
  recreation, and onboarding documentation.
- Context validation, effective default-context parity, context-scoped API
  registries, explicit legacy migration/scrubbing, duplicate protection, and
  deterministic CLI output.
- Authenticated server-side API probing, exact control-plane URL handling, safe
  response summaries, and structured CLI/server errors.
- Pure generation, one generated-manifest source of truth, richer draft
  metadata, method-aware cache defaults, and duplicate generated-name checks.
- Console config refresh, context persistence, retry/refresh controls, stale
  inventory behavior, and safe structured BFF errors.
- Production gating for demo system tools, declarative PII/role admission,
  RedisInsight exposure defaults, `.env.example` safety, and corresponding
  tests.
- `skills/bridgectl-ops` onboarding, diagnostics, operations, and evaluation
  guidance, plus synchronized in-repo and public documentation.

### Intentionally out of scope

- Changing MockERP's external image implementation or its credential schema.
  The pinned image contract will be tested through Compose, not modified here.
- Automatic deployment, installation, or lifecycle management of external
  plugin processes; the completed plugin skill plan remains the source for
  those operations.
- Inferring whether arbitrary ERP response data is PII at runtime.
- Persisting the Console's selected context back to the CLI config; Console
  selection remains browser-local and read-only.
- Removing all HTTP development exceptions from the local fixture; production
  still requires HTTPS and exact development allowlists remain explicit.
- Deleting existing registry or database data without an explicit migration,
  scrub, or operator confirmation.

## Tasks

- [x] **Task 1: Add a safe, reproducible local-stack bootstrap.**
  - Add `make dev-up` backed by a non-interactive script that generates
    ephemeral MockERP credentials only when neither credential source is set,
    preserves caller-provided quoted values, validates `docker compose config
    --quiet`, runs `docker compose up --build --force-recreate -d`, and polls
    MockERP and ERPBridge health without printing secrets. Add a preflight error
    for direct use without credentials and bind RedisInsight to loopback by
    default (or make it an explicit opt-in profile).
  - **Seam:** `scripts/dev-stack.sh` environment/bootstrap boundary and Compose
    service healthchecks.
  - **Files:** `scripts/dev-stack.sh`, `Makefile`, `docker-compose.yml`,
    `.env.example`, `docs/onboarding.md`, `docs/docker.md`,
    `docs/environment-variables.md`, `CHANGELOG.md`.
  - **Verify:** shell syntax and a no-secret dry-run; `docker compose config
    --quiet` with quoted credential values; a disposable Compose smoke run
    that reaches `/mcp/health`; assert RedisInsight is not bound beyond
    loopback; `git diff --check`.

- [x] **Task 2: Normalize contexts and isolate the local API registry.**
  - Make `config.Load` distinguish a missing file from a parse error, validate
    the effective current context after `--context`/`BRIDGE_CONTEXT` selection,
    and expose one error-producing resolver used by CLI and Console. Sort
    context and API listings deterministically. Move API commands to
    context-scoped registry paths; add explicit global-registry migration and
    all-target scrub handling with collision/legacy safeguards; make duplicate
    registration conflict by default and add `--force` for replacement.
  - **Seam:** `config.Config` effective-context resolver and
    `idp.Registry` load/write/migration boundary.
  - **Files:** `internal/config/config.go`, `internal/config/config_test.go`,
    `internal/cli/root.go`, `internal/cli/context.go`,
    `internal/cli/context_test.go`, `internal/cli/api.go`,
    `internal/cli/api_test.go`, `internal/cli/http.go`, `internal/cli/log.go`,
    `internal/cli/plugin.go`, `internal/cli/token.go`, `internal/cli/tool.go`,
    `internal/idp/registry.go`,
    `internal/idp/registry_test.go`, `docs/cli/bridgectl_context.md`,
    `docs/cli/bridgectl_api_register.md`,
    `docs/cli/bridgectl_api_scrub-credentials.md`, `docs/environment-variables.md`,
    `CHANGELOG.md`.
  - **Verify:** `go test ./internal/config ./internal/idp ./internal/cli
    -run 'Test(Config|Registry|Context|API)' -count=1`; tests for malformed
    YAML, missing/dead current contexts, per-context collisions, deterministic
    ordering, duplicate rejection/forced replacement, legacy migration, and
    secret-free scrub output; generated CLI docs compare cleanly.

- [x] **Task 3: Make API tests server-side and control-plane URLs unambiguous.**
  - Add an authenticated admin-only API probe route and bounded request/response
    types in `internal/mcp`. Resolve `credentialRef` on the server, reuse ERP
    endpoint preparation and outbound transport/redirect rules, return only a
    safe status summary, and add `--local` for the explicit legacy host-side
    path. Add a shared CLI control-plane URL normalizer used by tools, plugins,
    bindings, and the probe command; replace the `/mcp/`-induced session error
    with a typed remediation message.
  - **Seam:** server API-probe handler, `Tool.prepareERPCall`/connector call
    options, and `internal/cli` request URL construction.
  - **Files:** `internal/mcp/api.go`, `internal/mcp/server.go`,
    `internal/mcp/tool.go`, `internal/mcp/api_test.go`,
    `internal/mcp/server_test.go`, `internal/connector/client.go`,
    `internal/connector/client_test.go`, `internal/cli/api.go`,
    `internal/cli/api_test.go`, `internal/cli/errors.go`,
    `internal/cli/errors_test.go`, `internal/cli/http.go`,
    `internal/cli/http_test.go`, `internal/cli/tool.go`,
    `internal/cli/plugin.go`, `internal/cli/plugin_binding.go`, relevant generated CLI
    pages, `docs/onboarding.md`, `docs/docker.md`, `CHANGELOG.md`.
  - **Verify:** `go test ./internal/connector ./internal/mcp ./internal/cli
    -run '(API|Tool|Server|Client)' -count=1`; httptest coverage proves the
    server, not the host, resolves credentials; response bodies and headers do
    not cross the probe boundary; localhost rewriting is exact; redirects do
    not forward credentials; root, `/mcp/`, invalid-path, and unreachable
    control-plane cases return actionable errors.

- [x] **Task 4: Establish one generated-manifest source of truth.**
  - Remove implicit generator file writes and retain an explicit save/output
    seam. Change `make generate-tools` and onboarding to generate one bounded
    draft stream, apply that file once, and clean temporary artifacts. Add
    derived `whenToUse`/examples where OpenAPI evidence supports them, safe
    read-only cache defaults for GET/HEAD, explicit no-cache defaults for
    writes, complete generated structure assertions, and collision detection
    for sanitized operation names.
  - **Seam:** `idp.Generator.Generate`, `GenerateFromOpenAPI`, `Save`, and the
    `tool generate`/Makefile output boundary.
  - **Files:** `internal/idp/generator.go`, `internal/idp/generator_test.go`,
    `internal/cli/tool.go`, `internal/cli/tool_test.go`, `Makefile`,
    `.gitignore`, `docs/onboarding.md`, `docs/faq.md`, `docs/tool-schema.md`,
    `CHANGELOG.md`.
  - **Verify:** `go test ./internal/idp ./internal/cli -run
    '(Generator|DecodeTool|Tool)' -count=1`; generated output contains no
    implicit sibling JSON files, duplicate operation names fail before apply,
    cache/intent fields are deterministic, and a generated YAML file applies
    exactly once in an httptest control-plane flow.

- [x] **Task 5: Introduce safe structured errors across server and CLI.**
  - Add a bounded remote-error decoder in `internal/bridgeclient` and map
    server envelopes/statuses to `AgentActionableError` in the CLI. Replace
    control-plane `http.Error` responses with stable safe codes and suggestions,
    including the legacy-registry, duplicate, URL-root, auth, authorization,
    insecure-transport, validation, health, and upstream-connectivity classes.
    Preserve MCP JSON-RPC and `isError: true` execution semantics. Ensure
    verbose mode still redacts credentials, auth headers, raw bodies, and
    upstream URLs where required.
  - **Seam:** `bridgeclient` response decoder, `internal/cli.handleError`, and
    centralized server error writers.
  - **Files:** `internal/bridgeclient/client.go`,
    `internal/bridgeclient/client_test.go`, `internal/cli/errors.go`,
    `internal/cli/http.go`, `internal/cli/errors_test.go`,
    `internal/cli/plugin.go`, `internal/cli/plugin_binding.go`,
    `internal/cli/cache.go`, `internal/cli/root.go`, `internal/cli/tool.go`,
    `internal/mcp/errors.go`,
    `internal/mcp/errors_test.go`, `internal/mcp/server.go`,
    `internal/mcp/plugin_api.go`, `internal/mcp/auth.go`,
    `internal/mcp/info_api.go`, `internal/mcp/api.go`,
    `internal/mcp/api_test.go`, `internal/mcp/server_test.go`,
    `docs/cli/bridgectl.md`, `docs/faq.md`, `docs/api.md`,
    `CHANGELOG.md`.
  - **Verify:** table and JSON CLI tests assert stable human/machine output;
    server tests assert every covered error has a safe code/message/suggestion;
    malformed, oversized, HTML, and secret-bearing upstream bodies are bounded
    and redacted; `go test ./internal/bridgeclient ./internal/cli ./internal/mcp
    -run '(Error|HTTP|API|Tool|Plugin)' -count=1`.

- [x] **Task 6: Make Console context and data state reloadable and persistent.**
  - Add a config-provider/snapshot seam to the BFF, reload contexts safely on
    interval and manual refresh, preserve the last valid snapshot on parse
    failure, and use the same effective-context resolver as the CLI. Persist
    the selected context in `sessionStorage`, validate it after refresh, and
    retain it across route navigation/reload. Add refresh/retry controls and
    stale timestamps for inventory, topology, deployment, and context data.
    Preserve the read-only threat boundary and parse the BFF's safe error code
    and message in the frontend API client.
  - **Seam:** `HandlerOptions` config provider, BFF context lookup, React
    `ConsoleApp` selection state, and shared `apiFetch`/async hook behavior.
  - **Files:** `internal/web/context_api.go`, `internal/web/observability_api.go`,
    `internal/web/safe_dto.go`, `internal/web/metrics.go`,
    `internal/web/plugin_api.go`, `internal/web/topology.go`,
    `internal/web/context_api_test.go`, `internal/web/observability_api_test.go`,
    `internal/web/integration_test.go`, `internal/cli/web.go`, `web/src/App.tsx`,
    `web/src/hooks/useConsole.ts`, `web/src/hooks/useObservability.ts`,
    `web/src/hooks/useTopology.ts`, `web/src/lib/api.ts`,
    `web/src/components/layout/AppShell.tsx`, `web/src/components/ui/freshness.tsx`,
    `web/src/lib/api.test.ts`, `web/src/routes/Deployments.tsx`,
    `web/src/routes/Overview.tsx`, `web/src/routes/Plugins.tsx`,
    `web/src/routes/Tools.tsx`, `web/src/routes/Topology.tsx`, relevant route/tests,
    `docs/web-console.md`, `docs/cli/bridgectl_web.md`, `CHANGELOG.md`.
  - **Verify:** `go test ./internal/web ./internal/cli -run '(Context|Console|Web)' -count=1`;
    frontend tests cover reload persistence, invalid saved contexts, context
    switch races, config-file refresh, stale retention, retry, and structured
    errors; `npm run typecheck --prefix web`, `npm test --prefix web -- --run`,
    and `npm run lint --prefix web`.

- [x] **Task 7: Gate demo tools and sensitive data access.**
  - Gate `system.*_test` registration behind `MCP_ENABLE_TEST_TOOLS=false` by
    default and enable it only in the development Compose/bootstrap path. Add
    `security.dataClass` validation with `pii`/`restricted` requiring
    `allowedRoles`, update the employee-shaped test fixture,
    `skills/bridgectl-ops/assets/mcp-tool.yaml`, and role guidance, and add
    admission/discovery/invocation tests. Bind RedisInsight to loopback
    or make it opt-in, remove password-like values from `.env.example`, and
    document the local-demo boundary.
  - **Seam:** `Server.RegisterBuiltinTools`, `Server.validateTool`, role
    authorization admission, and Compose exposure configuration.
  - **Files:** `internal/mcp/tool.go`, `internal/mcp/server.go`,
    `internal/mcp/authz.go`, `internal/mcp/authz_schema_test.go`,
    `internal/mcp/server_test.go`, `internal/mcp/api_test.go`,
    `docker-compose.yml`, `.env.example`, `scripts/dev-stack.sh`,
    `scripts/test-dev-stack.sh`,
    `skills/bridgectl-ops/assets/mcp-tool.yaml`, `docs/tool-schema.md`,
    `docs/tokens.md`, `docs/docker.md`, `docs/environment-variables.md`,
    `CHANGELOG.md`.
  - **Verify:** default server discovery excludes demo tools; explicit
    development mode includes them; PII/restricted manifests without roles are
    rejected and role-guarded employee calls are denied before cache/ERP work;
    Compose exposure tests show RedisInsight is loopback/opt-in; secret and
    PII audits pass.

- [x] **Task 8: Improve `bridgectl-ops` onboarding, diagnosis, and evaluations.**
  - Add a deterministic preflight that checks the selected context, stack
    health, credential source, quoted environment handling, control-plane root,
    server-side API-test mode, context-scoped registry, and generated-manifest
    ownership before any mutation. Add recovery branches for each stable CLI
    error code, explicit `--force-recreate`/`--local` semantics, PII-safe role
    guidance, demo-tool and RedisInsight warnings, and a safe verification
    checklist. Add realistic positive/near-miss evaluations for the reported
    failures without embedding credentials or production data. Fold changes
    into the existing plugin references rather than duplicating plugin
    protocol details.
  - **Seam:** skill router, onboarding/diagnostics references, and local
    description/behavior evaluation fixtures.
  - **Files:** `skills/bridgectl-ops/SKILL.md`,
    `skills/bridgectl-ops/references/onboarding.md`,
    `skills/bridgectl-ops/references/diagnostics.md`,
    `skills/bridgectl-ops/references/operations.md`,
    `skills/bridgectl-ops/references/ecosystem.md`,
    `skills/bridgectl-ops/evals/evals.json`,
    `skills/bridgectl-ops/evals/description-trigger-evals.json`,
    relevant skill assets, `docs/onboarding.md`, `docs/cli/`, `CHANGELOG.md`.
  - **Verify:** `skills-ref@0.1.5 validate skills/bridgectl-ops`; JSON eval
    validation; static audits reject raw credentials, PII examples, and
    Console secrets; each reported issue maps to a numbered recovery branch;
    repeated trigger evaluation runs when the local evaluation client is
    available, otherwise records the existing deferred-client limitation.

- [x] **Task 9: Prove the end-to-end onboarding path and synchronize public docs.**
  - Add `scripts/test-onboarding.sh`, a disposable integration path that runs
    the bootstrap, registers an API in a selected context, tests it through the
    server, generates one
    manifest, applies it at the control-plane root, discovers/calls the tool
    with safe fixture data, and verifies expected failure branches without
    leaking bodies or credentials. Regenerate CLI pages, update root guides
    and `CHANGELOG.md`, then synchronize the corresponding pages and changelog
    in `../erpbridge-docs`.
  - **Seam:** Compose smoke fixture plus the public CLI/documentation build.
  - **Files:** `scripts/test-onboarding.sh`, `internal/integration/`,
    generated `docs/cli/`,
    `docs/onboarding.md`, `docs/docker.md`, `docs/faq.md`, `docs/architecture.md`,
    `CHANGELOG.md`, and corresponding `../erpbridge-docs/docs/bridgectl/`,
    `../erpbridge-docs/docs/erpbridge/`, and changelog files.
  - **Verify:** `go test ./...`, `make test`, targeted
    `golangci-lint run` for changed Go directories, web typecheck/tests/lint,
    the disposable Compose onboarding test when Docker is available, generated
    CLI-doc comparison, public `npm run build`, `git diff --check`, and a
    staged-file audit for secrets/raw ERP data. Only after all checks pass may
    the plan be promoted through the active workflow and archived.

## Verification

The completed implementation must satisfy all of the following observable
outcomes:

- A clean checkout's documented development command either starts healthy
  services with ephemeral credentials or stops before container creation with a
  precise credential prerequisite. It never prints a credential, and changing
  a quoted `.env` value is picked up after the documented recreate command.
- API registry entries are isolated by context, sorted deterministically, never
  silently overwritten, and recoverable from the legacy global registry only
  through an explicit, secret-safe migration/scrub workflow.
- `api test` resolves the ERP credential in the server by default and returns a
  bounded status summary with no upstream body. `--local` is explicit.
- `/mcp/` is documented as the MCP transport; the CLI control-plane root is
  normalized or rejected with a stable actionable error, never an `Invalid
  session ID` mystery.
- Generated tools have one owned draft artifact, stable intent/cache defaults,
  and no hidden sibling files. Reviewed manifests remain the only applied
  source.
- CLI and control-plane failures expose stable machine-readable codes plus
  safe remediation text. MCP tool execution failures retain `isError: true`.
- The Console sees valid context-file changes without restart, keeps the user
  selected context across navigation/reload, reports stale data and offers
  retry/refresh, and never exposes credentials, raw URLs, headers, bodies, or
  personal ERP data.
- Production discovery excludes demo system tools. Tools classified as PII or
  restricted require roles, and RedisInsight is not exposed beyond the local
  machine by default.
- `bridgectl-ops` can guide an agent from preflight through verification for
  every reported issue without asking it to source an unsafe `.env`, print a
  secret, use an admin identity to mask a role failure, or apply a manifest to
  the MCP session endpoint.

## Open Questions

None. The plan chooses explicit local bootstrap, context-scoped registries,
server-side API tests, exact control-plane-root normalization, structured
errors, reloadable read-only Console state, and declarative sensitivity/role
admission. These choices can be revised before promotion if product owners
prefer a different compatibility policy, but implementation should not begin
without an approved decision.
