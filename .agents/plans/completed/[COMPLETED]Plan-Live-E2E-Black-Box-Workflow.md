# Plan: Black-box live ERPBridge workflow

## Goal

Run a fresh, end-to-end black-box verification of the current ERPBridge release
using MockERP, `erpbridge-server`, `bridgectl`, the `bridgectl-ops` skill, and the
sample external plugin. Exercise every documented server capability through
its public HTTP, MCP, stdio, CLI, Docker, and plugin boundaries. Produce a
sanitized capability matrix and an issue log that can be used for regression
fixes without exposing credentials, ERP data, plugin payloads, or raw response
bodies.

## Current State

- The published documentation index is available at
  `https://blog.nimendra.xyz/erpbridge-docs/llms.txt` and links the focused
  bridgectl, server, MockERP, transport, authentication, cache, and plugin
  references used by this plan.
- The published REST reference defines the server's observable surfaces as
  tool and plugin registries, direct invocation, cache, logs, token lifecycle,
  MCP, metrics, health, API information, and bounded API probes
  (`../erpbridge-docs/docs/erpbridge/api.mdx:8-16`, `:37-92`, `:94-203`,
  `:205-361`).
- The published connectivity guide documents Streamable HTTP, stdio, direct
  HTTP, rate limiting, retries, timeouts, circuit breaking, and health/metrics
  checks (`../erpbridge-docs/docs/erpbridge/connectivity.mdx:8-110`).
- Authentication includes open/protected HTTP behavior, admin and scoped API
  tokens, outbound ERP credential references, role-protected tools, and
  redaction rules (`../erpbridge-docs/docs/erpbridge/auth.mdx:12-110`).
- The cache supports Redis and bounded in-memory backends, exact-match keys,
  TTL, tool/module/all flushes, and write-triggered invalidation
  (`../erpbridge-docs/docs/erpbridge/caching.mdx:12-116`).
- The documented onboarding workflow is preflight, safe stack startup, API
  registration and server-side probe, temporary generation, reviewed
  validation, explicit apply, and MCP verification
  (`../erpbridge-docs/docs/erpbridge/onboarding.mdx:13-217`).
- External plugins use exact-version resources and bindings, authenticated
  admin admission, allowlists, `after_response` and `raw_response` phases,
  bounded protocol documents, failure policies, cache interaction, and both
  MCP/direct invocation paths (`../erpbridge-docs/docs/erpbridge/plugins.mdx:23-235`).
- The `bridgectl-ops` skill requires explicit-context preflight, safe
  credential handling, readback at the highest available seam, stable-code
  recovery, and sanitized issue reports
  (`skills/bridgectl-ops/references/onboarding.md:10-174`,
  `skills/bridgectl-ops/references/diagnostics.md:3-74`,
  `skills/bridgectl-ops/references/plugins.md:7-195`).
- MockERP is pinned to `ghcr.io/nmdra/mockerp:0.2.1`; its credentials must be
  injected through a documented credential source, and its SQLite volume is
  disposable test state (`../erpbridge-docs/docs/erpbridge/mock-erp.mdx:13-69`).
- The only sample plugin documented by the plugin repository is the
  deterministic `plugins/mock-plugin` fixture
  (`../ERPBridge-Plugins/README.md:1-10`).
- The requested sibling directory `/home/nimendra/Documents/Projects/Erpbridge-demo`
  now exists with its project-scoped `bridgectl-ops` skill installed at
  `.agents/skills/bridgectl-ops/`; it has no prior manifests, schemas, reports,
  or runtime state beyond the preserved direct skill tree under `.agents/`.
  `/home/nimendra/Documents/Projects/Bridgectl-Demo` exists but contains prior
  artifacts and remains explicitly excluded.
- `bridgectl` is available at `/home/nimendra/.local/bin/bridgectl` on the
  current user's PATH and reports version `dev`. Execution must verify that
  this binary is built from the current ERPBridge checkout; `/usr/local/bin`
  is not writable without elevated privileges, so a root-owned install is not
  assumed or attempted silently.

## Decisions

- **Fresh workspace, never the existing demo.** Use only the requested
  `Erpbridge-demo` sibling. Its preinstalled `.agents/bridgectl-ops` files are
  the only allowed initial contents. Do not read, copy, mount, or apply
  anything from `Bridgectl-Demo`, the repository `schemas/` directory,
  previous reports, or an existing Compose project.
- **Black-box evidence only.** Use the published `erpbridge-docs` checkout,
  the `bridgectl-ops` skill, CLI help, Docker health/status, bounded HTTP
  status/headers, MCP JSON-RPC envelopes, plugin protocol observations, and
  sanitized logs. Do not inspect ERPBridge or plugin source code during the
  live run.
- **Separate fresh runtime per stateful mode.** Use unique Compose project
  names, ports, HOME/configuration directories, SQLite paths, Redis volumes,
  plugin state, and manifest directories. Run Redis-backed, in-memory, open,
  protected, and production-like demo-tool-disabled phases as isolated
  projects; tear down only the project created by that phase.
- **Environment-only secrets.** Generate MockERP, bridge, scoped-token, and
  plugin credentials at runtime. Pass them through process environment or a
  temporary Compose env input, never command output, manifests, reports,
  source files, `.env` shell evaluation, or issue evidence. Assertions may
  compare an in-memory sentinel, but must never persist it.
- **Reviewed manifests are the only applied source.** Download the pinned
  OpenAPI document into a new temporary input directory, generate a draft,
  review and sanitize it, save only approved manifests under
  `Erpbridge-demo/manifests/<run>/`, validate them, and apply those files.
  Generated drafts and temporary schemas are deleted after each phase.
- **Record before retry.** Every unexpected result becomes an issue entry with
  its first observed command, expected/actual status, stable error code, and
  redacted evidence before any retry or workaround. A missing fixture
  capability is recorded as `BLOCKED_FIXTURE`, not silently skipped.
- **No destructive cleanup outside the test boundary.** Cleanup may remove
  only containers, networks, and volumes labelled with the unique test project
  name. It must never run global Docker prune, reset a shared MockERP database,
  alter existing contexts, migrate a real registry, or delete user files.

## Scope

### In scope

- Fresh Docker Compose runs containing the current ERPBridge server, pinned
  MockERP, Redis where applicable, and the separately built sample
  `mock-plugin`.
- `bridgectl` version/context/API/tool/plugin/binding/token/cache/log/skill
  workflows with an explicit isolated context.
- Server health, info, metrics, authentication, authorization, API probes,
  registry lifecycle, direct invocation, cache behavior, resilience, rate
  limiting, redaction, Streamable HTTP MCP, stdio MCP, and external plugin
  processing.
- Safe synthetic MockERP records and role slugs created only inside the fresh
  run, including create/update/delete cleanup of records created by the run.
- A sanitized capability matrix, run summary, and one issue report per
  regression or blocked capability under `Erpbridge-demo/reports/`.

### Out of scope

- Reading or changing ERPBridge, MockERP, plugin, SDK, or public-doc source.
- Reusing `Bridgectl-Demo`, checked-in manifests, checked-in schemas, old
  registries, old Docker volumes, or prior logs.
- Production ERP data, real credentials, real plugin endpoints, broad network
  scans, global Docker cleanup, or destructive registry migration.
- Changing product code or public documentation during the test run. Confirmed
  regressions become sanitized reports and a separate implementation plan.
- SDK-specific behavior and Console UI behavior; those are separate clients,
  while the server's HTTP/MCP boundaries used by them are covered here.

## Tasks

- [x] **Task 1: Create the fresh test workspace and evidence contract.**
  Preserve only the already-installed `.agents/bridgectl-ops` tree in
  `/home/nimendra/Documents/Projects/Erpbridge-demo`; add a README that
  records the exact source revisions and no-reuse rule, a `.gitignore` for
  runtime secrets and generated files, a capability matrix, and report
  templates based on the skill's sanitized bug-report format. Add a runner
  library that writes only allowlisted status, name, code, timing, and size
  fields. Build the current CLI and install it to the user-wide executable
  directory `/home/nimendra/.local/bin/bridgectl`; verify it is on PATH and
  document a separate operator command for `/usr/local/bin` only when elevated
  privileges are intentionally supplied. (**Seam:** CLI version output,
  process exit status, installed skill tree, and sanitized report files;
  **Files:** `../Erpbridge-demo/README.md`,
  `../Erpbridge-demo/.gitignore`, `../Erpbridge-demo/reports/capability-matrix.md`,
  `../Erpbridge-demo/reports/issues/README.md`,
  `../Erpbridge-demo/scripts/lib/evidence.sh`; **Verify:**
  `go build -o /home/nimendra/.local/bin/bridgectl ./tools/bridgectl`,
  `bridgectl version`, `test -f ../Erpbridge-demo/.agents/skills/bridgectl-ops/SKILL.md`, and
  `test -d ../Erpbridge-demo && test -f ../Erpbridge-demo/.gitignore &&
  ! grep -RInE '(Authorization:[[:space:]]|Bearer[[:space:]]+[A-Za-z0-9._-]{20,}|password[[:space:]]*[:=]|api[_-]?key[[:space:]]*[:=]|MOCK_ERP_CREDENTIALS_JSON=)' ../Erpbridge-demo/reports`.)

- [x] **Task 2: Build and start isolated live services with new Compose files.**
  Add separate Compose files for the ERPBridge server and pinned MockERP, plus
  the Redis and independently built `plugins/mock-plugin` services needed by
  the test phases. Combine only these new files at runtime; do not reuse the
  repository or `Bridgectl-Demo` Compose files. Use a unique project name and
  non-default host ports per phase, a new SQLite path and named volumes,
  ephemeral environment credentials, and cleanup traps. Verify Compose
  interpolation without sourcing an env file, record image/version names and
  health status only, and confirm RedisInsight is absent or loopback-only.
  (**Seam:** Docker Compose config, health endpoints, and `docker compose ps`;
  **Files:** `../Erpbridge-demo/compose/erpbridge-server.compose.yml`,
  `../Erpbridge-demo/compose/mockerp.compose.yml`,
  `../Erpbridge-demo/compose/redis.compose.yml`,
  `../Erpbridge-demo/compose/mock-plugin.compose.yml`,
  `../Erpbridge-demo/scripts/run-phase.sh`,
  `../Erpbridge-demo/runtime/` (ignored); **Verify:**
  `docker compose --project-name "$PROJECT" -f compose/mockerp.compose.yml
  -f compose/erpbridge-server.compose.yml -f compose/redis.compose.yml
  -f compose/mock-plugin.compose.yml config --quiet`,
  `curl -fsS --max-time 5 "$BRIDGE_URL/mcp/health"`,
  `curl -fsS --max-time 5 "$MOCK_URL/health"`, and a clean trap run that
  leaves no containers, network, or volumes for `$PROJECT`.)

- [x] **Task 3: Execute the `bridgectl-ops` preflight and complete fresh onboarding.**
  Follow the skill's explicit-context sequence: record `bridgectl version`,
  safe context names, stack health, quoted Compose validation, credential-source
  booleans, `api test --help`, control-plane-root validity, and an empty
  context-scoped registry. Register a uniquely named API against MockERP, run
  the authenticated server-side body-free probe, generate a fresh OpenAPI
  draft, save a reviewed manifest, validate it, apply it, and verify exact
  API/tool readback. Exercise duplicate registration and invalid-context,
  invalid-control-plane, and invalid-manifest recovery using stable codes;
  never overwrite or migrate an existing registry. (**Seam:** `bridgectl`
  command exit codes/JSON, registry readback, and API probe summary;
  **Files:** `../Erpbridge-demo/scripts/onboarding.sh`,
  `../Erpbridge-demo/manifests/<run>/`,
  `../Erpbridge-demo/evidence/onboarding.json`; **Verify:**
  `BRIDGE_HOME="$HOME_DIR" ./scripts/onboarding.sh` exits 0, records
  `isSuccess=true` without endpoint/auth metadata, and
  `bridgectl tool get --context "$CTX" -o json` shows only the reviewed
  names and versions.)

- [x] **Task 4: Cover every public control-plane, direct, and MCP transport seam.**
  Use the CLI and redacted `curl`/MCP clients to test tool/plugin/binding
  list/apply/readback/soft-delete/hard-delete behavior, exact name/version
  filters, direct registered-tool invocation and compatibility envelopes,
  `/api/info`, `/mcp/health`, `/metrics`, cache stats/flush, recent logs and
  SSE log streaming. Test Streamable HTTP initialize/version negotiation,
  session ID use, `tools/list`, `resources/list`, `prompts/list`,
  `tools/call`, notification handling, invalid session, unknown tool, and
  protocol errors. Spawn `erpbridge-server --stdio` separately and verify
  JSON-RPC stays on stdout while diagnostics stay on stderr. (**Seam:** HTTP
  response status and bounded JSON fields, MCP envelopes, SSE event names,
  and stdio stream separation; **Files:**
  `../Erpbridge-demo/scripts/test-control-plane.sh`,
  `../Erpbridge-demo/scripts/test-mcp-http.py`,
  `../Erpbridge-demo/scripts/test-mcp-stdio.py`,
  `../Erpbridge-demo/evidence/transports.json`; **Verify:**
  `./scripts/test-control-plane.sh`, `python3 scripts/test-mcp-http.py`, and
  `python3 scripts/test-mcp-stdio.py` each exit 0 and the matrix contains a
  PASS or explicit `BLOCKED_FIXTURE` for every documented route.)

- [x] **Task 5: Cover authentication, authorization, schemas, and safety gates.**
  In a protected phase test missing, invalid, admin, and scoped bearer
  credentials; create/list/revoke tokens for `mcp`, `metrics`, and `logs`,
  test scope denial and expiry, and confirm list output contains metadata but
  neither raw tokens nor hashes. Test public and internal tools plus `pii` and
  `restricted` tools with opaque synthetic roles through MCP and the direct
  `X-ERPBridge-Role` selector; verify denied calls happen before ERP/cache
  work and that a body `role` is rejected for guarded tools. Test outbound
  API-key, bearer, and basic credential references without recording values,
  strict schema rejection, endpoint secret rejection, raw plugin-secret
  rejection, and missing-credential fail-closed behavior. Run a development
  phase with `MCP_ENABLE_TEST_TOOLS=true` to exercise safe `system.*_test`
  tools, then a production-like phase with it disabled and verify those tools
  are absent. Check CORS preflight ordering and RedisInsight binding safety.
  (**Seam:** status codes, safe error envelopes, MCP `isError`, discovery
  membership, and bounded metrics/log evidence; **Files:**
  `../Erpbridge-demo/scripts/test-authz.sh`,
  `../Erpbridge-demo/evidence/authz.json`,
  `../Erpbridge-demo/reports/issues/`; **Verify:**
  `./scripts/test-authz.sh` exits 0 and asserts `401`, `403`, successful
  authorized calls, denied discovery/calls, and no credential or personal-data
  marker in any report.)

- [x] **Task 6: Cover tool execution, caching, protection, and resilience.**
  Call fresh generated GET/list tools and safe synthetic POST/create,
  PUT/update, and DELETE cleanup flows. Test input-schema and output-schema
  success/failure, mapping and `responsePath`, inactive/soft-deleted tools,
  exact API/tool version replacement rules, cache hits/misses, TTL expiry,
  shared read-only and role-scoped cache keys, tool/module/all flushes,
  `flushOn` invalidation, and uncached writes. Repeat the cache suite against
  Redis and the bounded in-memory backend, and verify an unavailable
  configured Redis backend fails rather than silently falling back. Exercise
  rate-limit `429` behavior, upstream `4xx`/`5xx`/network failure, timeout,
  retry/backoff, circuit-open/fail-fast, and recovery using only safe MockERP
  routes or an isolated disposable fault endpoint; use metrics and bounded
  logs to confirm behavior without copying bodies. (**Seam:** direct/MCP
  result envelopes, upstream request counters, cache stats, `Retry-After`,
  and stable error codes; **Files:**
  `../Erpbridge-demo/scripts/test-execution-cache-resilience.sh`,
  `../Erpbridge-demo/evidence/execution.json`,
  `../Erpbridge-demo/evidence/cache-redis.json`,
  `../Erpbridge-demo/evidence/cache-memory.json`; **Verify:**
  `./scripts/test-execution-cache-resilience.sh` exits 0 and records the
  expected hit/miss, flush, retry, circuit, rate-limit, and recovery outcomes.)

- [x] **Task 7: Exercise the sample plugin and both binding phases.**
  Build and run `ERPBridge-Plugins/plugins/mock-plugin` as a separate
  operator-owned service. Validate, apply, and read back a versioned plugin;
  test missing, wrong, and correct API-key admission without recording the
  key; apply an exact-version `after_response` binding and verify transformed
  output through both MCP and direct invocation, cache behavior, priority,
  schema validation, `continue` and `fail` policies, binding deactivation,
  and protected plugin deletion. Add a disposable protocol-fault fixture only
  where the sample plugin cannot produce a documented negative response, and
  mark that fixture explicitly. Test a `raw_response` binding with a safe JSON
  response, then binary, empty, malformed, oversize, redirect, non-2xx, and
  timeout cases where the public fixture contract permits; assert bounded
  tagged capture, immutable status, no retries/redirects, raw-before-
  normalization order, final schema validation, and no leaked payloads,
  credentials, headers, or caller identity. (**Seam:** plugin `/v1/process`
  request shape, binding readback, MCP/direct output, cache stats, and safe
  error code; **Files:**
  `../Erpbridge-demo/scripts/test-plugins.sh`,
  `../Erpbridge-demo/manifests/plugins/`,
  `../Erpbridge-demo/manifests/bindings/`,
  `../Erpbridge-demo/evidence/plugins.json`,
  `../Erpbridge-demo/reports/issues/`; **Verify:**
  `./scripts/test-plugins.sh` exits 0, exact resources read back as active or
  intentionally inactive, and every unsupported sample-plugin mode is logged
  as `BLOCKED_FIXTURE` with a safe reproduction.)

- [x] **Task 8: Finalize sanitized reports, regression evidence, and cleanup.**
  Run `bridgectl log stats` and bounded `log tail` filters for each phase,
  scrape safe metrics, compare all phase results to the matrix, and write one
  report per issue using the skill template: environment, expected/actual,
  stable code, minimal redacted reproduction, evidence, investigation, and
  security review. Record resolved issues as well as unresolved regressions;
  preserve only sanitized reports and aggregate timings, delete runtime env
  files, temporary OpenAPI/draft/schema files, credentials, tokens, and raw
  service logs, then remove only the unique Compose projects. Do not mark the
  run green when a capability is failed or fixture-blocked. (**Seam:** final
  matrix and sanitized issue index; **Files:**
  `../Erpbridge-demo/reports/run-<timestamp>.md`,
  `../Erpbridge-demo/reports/issues/`,
  `../Erpbridge-demo/evidence/`; **Verify:**
  `./scripts/finalize-report.sh` exits 0 only when every in-scope capability
  is PASS or has a linked issue, `grep -RInE` finds no secret/PII/raw-body
  markers, and `docker ps -a --filter label=com.docker.compose.project="$PROJECT"`
  plus `docker volume ls --filter label=com.docker.compose.project="$PROJECT"`
  return no rows.)

## Verification

The run is complete only when the following observable gates are satisfied:

1. The run uses a newly created `Erpbridge-demo` workspace, isolated HOME and
   CLI registry, unique Compose project, unique ports, fresh SQLite/Redis
   state, fresh manifests, and runtime-generated credentials.
2. The selected context, `bridgectl` version, source commit, server image,
   MockERP `0.2.1` image/contract, plugin revision, phase names, and timestamps
   are recorded without credential values or opaque tokens.
3. The skill preflight and onboarding path pass with server-side API probing,
   reviewed manifest validation, explicit apply, exact readback, MCP discovery,
   and a safe tool call.
4. Every REST, CLI, Streamable HTTP, stdio, direct-invoke, token, role,
   cache, resilience, plugin, binding, logging, metrics, health, and
   development-tool gate has a PASS, FAIL, or `BLOCKED_FIXTURE` result.
5. All failures use the documented stable error/recovery path where one
   exists. A retry occurs only after the first failure and its sanitized issue
   record are preserved.
6. No evidence contains credentials, authorization headers, URLs with
   secrets, personal data, ERP records, plugin payloads, raw response bodies,
   invocation IDs, token hashes, or unbounded logs.
7. Cleanup removes only the run's Docker resources and runtime secrets. The
   existing `Bridgectl-Demo`, ERPBridge checkout state, user HOME, global
   bridgectl registries, and protected files remain unchanged.
8. A release decision is based on the final matrix: any product regression is
   a release blocker; fixture limitations are separate documented blockers;
   an all-PASS run is required before claiming live-test completion.

## Open Questions

None. The requested `Erpbridge-demo` workspace now exists with only the
preinstalled skill tree. The existing `Bridgectl-Demo` directory remains
forbidden because it contains prior artifacts. A root-owned `/usr/local/bin`
installation is not part of the automated run unless the operator separately
provides elevated privileges; the user-wide `/home/nimendra/.local/bin`
installation is the default executable installation for this test.
