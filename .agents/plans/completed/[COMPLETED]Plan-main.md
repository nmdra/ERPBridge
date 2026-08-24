# Plan: Hardening — Cache, Security, Correctness, CLI & Docs

> **Status: COMPLETED** — implemented and verified in `c6b894f` and the subsequent authentication, documentation, and skill synchronization commits. Retained as the historical hardening execution record.

## Goal

Remediate the non-auth findings from the repository analysis: caching works without Redis, cache keys cannot collide, cache hits preserve MCP results, outbound credentials and logs cannot leak, correctness defects fix, and CLI/docs artifacts stop drifting from reality. HTTP token work was **out of scope** (see `../stalled/Plan-Auth.md`).

## Current State (evidence)

**Cache**

- No Redis ⇒ no cache at all: `cacheMgr` stays nil when `REDIS_URL` empty (services/erpbridge-server/main.go:55,77-90); `CacheMiddleware` bypasses on `s.cache == nil` (internal/mcp/middleware.go:137). Docs say "cache disabled" (docs/caching.md:94, docs/faq.md:40).
- Keys truncated to 32 bits: `argsHash` uses `h[:4]` → 8 hex chars (internal/cache/manager.go:96-113).
- `CachedAt` fabricated at read time; `EnsureIndex` is a no-op (manager.go:43-46); `scanAndDelete` DELs one key per round-trip (internal/cache/flush.go:76-83); `handleCacheList`/`handleCacheInspect` are 501 stubs (internal/mcp/server.go:812-818).

**Security**

- Silent credential fallback: `os.Getenv(CredentialRef)` falls back to the literal ref string as the credential (internal/mcp/tool.go:199-203).
- Redaction only on the MCP stream handler (internal/logger/mcp_handler.go); `broadcastHandler` (internal/logger/logger.go:52-87) and `LoggingMiddleware` (internal/mcp/middleware.go:91 — logs full `req.Params.Arguments`) emit raw values.
- Hardcoded credential in docker-compose.yml:44 (the committed value was removed).

**Correctness**

- Ghost tools: deactivated tools hidden only by the HTTP SSE line filter (internal/mcp/server.go:370, 477-531); Stdio transport advertises them (main.go:111 uses `mcp_server.ServeStdio`).
- Weak state hash: count+activeSum+maxUpdated (internal/mcp/store.go:121).
- Migration ignores errors: `_, _ = s.db.Exec("ALTER TABLE tools ADD COLUMN is_active ...")` (store.go:60).
- Unbounded per-session rate limiter map (internal/mcp/middleware.go:35-51).
- ResponsePath: top-level map key only; missing path silently returns full response (internal/mcp/tool.go:226-233).

**CLI & docs**

- `log tail` filters via `strings.Contains` on raw JSON (internal/cli/log.go:146-160).
- `tool get`/`describe` fetch the whole tool list and filter client-side (internal/cli/tool.go:106-235); server `handleToolList` has no query filters (internal/mcp/server.go:620).
- idp registry is a plain JSON file, no locking (internal/idp/registry.go:39, 62-66).
- Parse errors silently ignored in `tool validate` (internal/cli/tool.go:286-291).
- Drift: Postman + mcp-client-guide use `finance.list_invoices_api_v1_finance_invoices_get`; skill `mcp-tool.yaml` uses `spec.endpoint.ref/method` (actual: `spec.execution.method/endpoint`); docs list stale `SCHEMAS_DIR`/`EMBEDDER_URL`; generated `outputSchema` carries unresolvable `$ref` to OpenAPI components (internal/idp/generator.go:258-267).
- Integration evidence (2026-08-21): `bridgectl tool apply` rejects the multi-document YAML emitted by the generator ("sequence was used where mapping is expected"), while applying the individual JSON schemas succeeds. `POST /api/tools/invoke` resolves registered tools only, so MCP built-ins such as `system.progress_test` return 404 through REST. MCP text results can contain a JSON-encoded ERPBridge response and therefore appear nested to SDK consumers.

## Decisions

| # | Decision | Rationale / rejected |
| --- | --- | --- |
| B-D1 | Cache backend interface: `Backend` includes `FlushAll`; `RedisBackend` is used whenever `REDIS_URL` is set, even if unreachable; `MemoryBackend` is a mutex-protected LRU used only when Redis is not configured. `CACHE_MEMORY_MAX_ENTRIES` defaults to 10,000, invalid values warn and use the default, and zero disables memory caching. `ttlSeconds: 0` means no expiry. | Do not silently mask Redis deployment errors; no `CACHE_BACKEND` override is needed. |
| B-D2 | Full SHA-256 hex (64 chars) cache keys. | Old short keys become harmless misses; no migration. |
| B-D3 | Store `{response, cachedAt}` envelopes and treat decode failure as a miss. Cache hits restore the same MCP result semantics as uncached calls. | Rewrapping cached content as a text result changes the protocol contract. |
| B-D4 | Remove `EnsureIndex` + call site (main.go:85) + tests; remove 501 stub handlers + their routes (server.go:812-818, 562-563); batch `UNLINK` (100/iter) in `scanAndDelete`. | Dead code; unused surface; flush speed. |
| B-D5 | Tool and resource credential references fail closed: a non-empty unresolved reference errors only for that execution; empty references mean no auth. Warn at startup and after apply/reconciliation, but retain declarative registration. Relative resource URLs honor `ERP_BASE_URL`. | Stops literal-reference leaks without making unrelated tools unavailable. |
| B-D6 | Shared redaction: extract masq `ReplaceAttr` from mcp_handler.go into `logger.RedactAttr`; apply in `broadcastHandler`; add `logger.RedactArgs(map)` (sensitive keys: password/token/api_key/secret/key/ssn/authorization…) used by `LoggingMiddleware`. | Console/broadcast logs stay leaky otherwise. |
| B-D7 | Compose credential → `${ERP_PRIMARY_KEY:-}` interpolation (docker-compose.yml:44); dev value only in `.env.example`. | No secrets in tracked config. |
| B-D8 | Ghost tools: extract `filterToolsList` into one helper; Stdio wraps the pinned `mcp-go v0.57.0` JSON-line writer. Transport tests cover partial writes, notifications, and pass-through records. | Keeps HTTP and Stdio consistent without forking `mcp-go`. |
| B-D9 | State hash: SHA-256 over sorted `(name, version, is_active, updated_at)` tuples from `SELECT name, version, is_active, updated_at FROM tools ORDER BY name, version`. | Renames/timestamp ties no longer collide. |
| B-D10 | Migration: `PRAGMA table_info(tools)` check before `ALTER TABLE ADD COLUMN is_active`; add only if missing; propagate real errors (store.go:40-64). | Hides real failures today. |
| B-D11 | Rate limiter: track lastSeen; when map > 10k, sweep entries idle > 15 min. Use the authenticated principal supplied by `../stalled/Plan-Auth.md` for HTTP and retain session/process identity for Stdio and open mode. | No background goroutine; opening HTTP sessions cannot bypass a token’s limit. |
| B-D12 | ResponsePath: dotted paths + `[i]` array indexes; missing path ⇒ tool-call error (behavior change from silent full-response, tool.go:226-233). | Deterministic; schema drift becomes visible. |
| B-D13 | CLI: parse SSE lines as JSON; use server-side tool filters; mutate the IDP registry under a cross-platform lock file with reload-under-lock and atomic rename; surface validation parse errors. | Prevents false matches, corruption, and cross-process lost updates. |
| B-D14 | Docs/artifacts: fix stale names/flags/template fields/environment variables. Output schemas must be fully dereferenced; unresolved references fail generation with operation/path context. | Never emit a partial schema that appears valid but fails at runtime. |
| B-D15 | Generator-to-apply is a supported CLI workflow: `bridgectl tool apply` accepts the multi-document YAML emitted by `bridgectl tool generate`. Documentation also states that REST direct invoke is registry-only and that SDK consumers must preserve the MCP result envelope. | Per-tool JSON is a temporary verification workaround, not a usable onboarding contract; silently flattening MCP output would change the protocol response contract. |

## Scope

**In:** cache (backend interface, memory fallback, result parity, full keys, envelopes, correct invalidation), security (tools/resources credentials, redaction, compose), correctness (ghost tools, state hash, migration, limiter, ResponsePath), CLI + docs drift.

**Out:** auth/tokens (`../stalled/Plan-Auth.md`), OAuth, mock ERP improvements (tests/latency/persistence), cache `list`/`inspect` implementation, `CACHE_BACKEND` override, the `invalidateOn` compatibility alias, and notification renaming.

## Tasks

Execute tasks in the global order in `../README.md`: S1 → S3 → S2, then the auth plan, then C1 → C5, K2 → K5 → K1, and D1 → D5.

- [x] **C1 — Cache backend interface + memory fallback** (**Seam:** all cache methods consumed by `CacheMiddleware` and cache HTTP handlers; **Files:** `internal/cache/{manager,redis_backend,memory_backend}.go`, `services/erpbridge-server/main.go`, cache tests; **Verify:** `go test ./internal/cache/...`; no-Redis opt-in cache hits, zero capacity disables memory, and configured-but-unreachable Redis does not use memory).
- [x] **C2 — Full-hash cache entries and result parity** (**Seam:** `argsHash`, exact get/set, and cache-hit middleware path; **Files:** `internal/cache/{manager,exact}.go`, `internal/mcp/middleware.go`, cache/middleware tests; **Verify:** `go test ./internal/cache/... ./internal/mcp/...`; legacy raw entries miss and cache-hit/miss MCP results are semantically equal).
- [x] **C3 — Cache hygiene** (**Files:** `internal/cache/{manager,flush}.go`, `internal/mcp/server.go`, `services/erpbridge-server/main.go`, cache tests; **Verify:** package-scoped `golangci-lint run ./internal/cache/... ./internal/mcp/... ./services/erpbridge-server/...`; remove `EnsureIndex`, 501 routes, and batch Redis `UNLINK`).
- [x] **C4 — Run write invalidation independently of cached reads** (**Seam:** `CacheMiddleware`; **Files:** `internal/mcp/middleware.go`, middleware tests; **Verify:** `go test ./internal/mcp/ -run TestCacheMiddleware`; a write tool with `enabled:false` and `flushOn` invalidates cached reads).
- [x] **C5 — Correct all/module cache flushing** (**Seam:** flush handler and store tool lookup; **Files:** `internal/cache/{manager,flush}.go`, `internal/mcp/{store,server}.go`, cache/server tests; **Verify:** `go test ./internal/cache/... ./internal/mcp/...`; all clears every key and module clears every stored active/inactive tool version in that module).
- [x] **S1 — credentialRef fail-closed** (**Seam:** `Tool.Execute`, `Resource.Execute`, `handleToolApply`, and `Reconcile`; **Files:** `internal/mcp/{tool,resource,server}.go`, `services/erpbridge-server/main.go`, MCP tests; **Verify:** `go test ./internal/mcp/ -run 'Test(Tool|Resource|Server)'`; missing references warn at startup/apply/reconcile and fail only the affected call).
- [x] **S2 — Shared log redaction** (**Seam:** `logger.Init` multi-handler wiring (logger.go:106-118) + `LoggingMiddleware`; **Files:** internal/logger/mcp_handler.go (extract redaction to `RedactAttr`), internal/logger/logger.go:52-87 (use it in broadcastHandler), internal/logger/redact.go (new: `RedactArgs(any) any` recursive map redactor), internal/mcp/middleware.go:91 (redact args), internal/logger/logger_test.go, internal/mcp/middleware_test.go; **Verify:** `go test ./internal/logger/... ./internal/mcp/...`; broadcast output and tool-args logs contain `[REDACTED]`, never the values; mcp_handler tests still pass).
- [x] **S3 — Compose credential** (**Files:** docker-compose.yml:44 → `ERP_PRIMARY_KEY=${ERP_PRIMARY_KEY:-}`, .env.example stays as documented dev value; **Verify:** `docker compose config | rg ERP_PRIMARY_KEY` shows the interpolation, no literal secret).
- [x] **K1 — Ghost tools on Stdio** (**Seam:** `DeregisterTool`/`RegisterTool` and Stdio startup; **Files:** `internal/mcp/{server,filter_writer}.go`, `services/erpbridge-server/main.go`, MCP transport tests; **Verify:** `go test ./internal/mcp/ -run TestStdio`; active/inactive lists, partial writes, notifications, and non-tool-list pass-through are covered).
- [x] **K2 — Robust state hash** (**Seam:** `Store.GetStateHash` (store.go:121); **Files:** internal/mcp/store.go, store_test.go (add: same count/sum but renamed tool ⇒ hash changes); **Verify:** `go test ./internal/mcp/ -run TestStore_GetStateHash`).
- [x] **K3 — Safe migration** (**Seam:** `Store.init` (store.go:40-64); **Files:** internal/mcp/store.go (PRAGMA check), store_test.go (fixture: create table without `is_active`, assert init adds it and preserves rows); **Verify:** `go test ./internal/mcp/ -run TestStore_NewStore`).
- [x] **K4 — Limiter eviction and identity** (**Seam:** `RateLimitMiddleware.getLimiter`; **Files:** `internal/mcp/middleware.go`, middleware tests; **Verify:** `go test ./internal/mcp/ -run TestRateLimitMiddleware`; idle eviction, authenticated-principal HTTP keys, and Stdio/session fallback pass).
- [x] **K5 — ResponsePath hardening** (**Seam:** tool.go:226-233; **Files:** internal/mcp/tool.go (resolve dotted paths + `[i]`; error when path missing), tool_test.go (nested, array, missing-path cases); **Verify:** `go test ./internal/mcp/ -run TestTool_Execute`).
- [x] **D1 — Structured log tail** (**Seam:** `shouldPrint` (cli/log.go:146-160); **Files:** internal/cli/log.go (json.Unmarshal each `data:` message, filter on `component`/`tool_name`/`level`/`request_id` fields; malformed lines pass through), log_test.go; **Verify:** `go test ./internal/cli/ -run TestLogTail`; substring false-positives gone).
- [x] **D2 — Server-side tool filter** (**Seam:** `handleToolList` (server.go:620); **Files:** internal/mcp/server.go (`?name=` exact + `?version=`; empty = all), internal/cli/tool.go (get/describe pass name@version parsed by `mcp.ParseToolIdentifier`), api_test.go, tool_test.go; **Verify:** `go test ./internal/mcp/ ./internal/cli/`; `get` sends query params).
- [x] **D3 — Registry locking + validation errors** (**Seam:** `Registry.Register/Delete/save` and `tool validate`; **Files:** `internal/idp/registry.go`, a cross-platform lock abstraction/tests, `internal/cli/tool.go`, IDP/CLI tests; **Verify:** `go test ./internal/idp/... ./internal/cli/...`; reload-under-lock retains concurrent updates and invalid schemas fail visibly).
- [x] **D4 — Docs/artifact drift + strict generator dereference** (**Files:** `internal/idp/generator.go`, generator tests, `erpbridge_postman_collection.json`, `docs/{mcp-client-guide,environment-variables}.md`, `skills/bridgectl-add-api/{SKILL.md,references/COMMANDS.md,assets/mcp-tool.yaml}`, `CHANGELOG.md`; **Verify:** `go test ./internal/idp/...`; stale-term audit is empty, generated schemas contain no `$ref`, and unresolved references fail with operation/path context. Use `writing-for-agents` when editing the skill).
- [x] **D5 — Generated YAML + documented response boundaries** (**Seam:** generator output → CLI apply → invocation contract; **Files:** `internal/cli/tool.go`, CLI tests, `docs/{onboarding,api}.md`, `CHANGELOG.md`, matching `erpbridge-docs` pages; **Verify:** `go test ./internal/cli/...`; generated YAML applies every tool, `system.*` is MCP-only, and both documentation repositories describe MCP result envelopes).

## Verification

1. Focused tests and package-scoped lint are green before each atomic Conventional Commit; `make test` is green before handoff and commit.
2. **No-Redis smoke**: server without `REDIS_URL`; repeat cached tool call ⇒ second call cache hit (log + metric); `/api/cache/stats` live.
3. **Ghost-tool smoke**: delete a tool ⇒ `tools/list` omits it over both Stdio and HTTP; re-apply restores.
4. **Redaction smoke**: tool call with `password`-keyed arg ⇒ `bridgectl log tail` and console show `[REDACTED]`.
5. **Docs audit**: `rg` from D4 clean; regenerated schemas `$ref`-free.
6. **Docs release:** D4/D5 include matching commits in `~/Documents/Projects/erpbridge-docs`.

## Open Questions

None — decisions B-D1..B-D15 were refreshed in the 2026-08-22 grilling session. Follow the global order in `../README.md`; authentication work is in `../stalled/Plan-Auth.md`.

## Review Addendum (2026-08-20) — code-vs-plan audit

All claims verified against the codebase. Findings below.

### Line-number drift

Files have grown since the plan was written. Locate seams by code pattern, not line number:

| File | Plan max reference | Actual line count |
| --- | --- | --- |
| `internal/mcp/server.go` | ~818 | 873 |
| `internal/mcp/store.go` | ~121 | 177 |
| `internal/mcp/middleware.go` | ~176 | 176 (unchanged) |
| `internal/mcp/tool.go` | ~233 | 267 |
| `internal/cache/manager.go` | ~113 | 151 |
| `internal/cache/flush.go` | ~83 | 84 (unchanged) |
| `services/erpbridge-server/main.go` | ~111 | 128 |

Most references are off by 1–10 lines. Search by function name or string literal.

### Verified claims (all confirmed)

- ✅ `argsHash` uses `h[:4]` → 8 hex chars (manager.go:112)
- ✅ `EnsureIndex` is a no-op, still called from main.go:85
- ✅ credentialRef fallback sends env var *name* as credential (tool.go:199-203) — **real vulnerability**
- ✅ `LoggingMiddleware` logs raw `req.Params.Arguments` with no redaction (middleware.go:91)
- ✅ Hardcoded ERP_PRIMARY_KEY value in docker-compose.yml:44 was removed
- ✅ Stdio path uses `mcp_server.ServeStdio()` with no tool filter (main.go:111)
- ✅ State hash is `count-activeSum-maxUpdated` — renamed tools don't change hash (store.go:121-130)
- ✅ Migration error swallowed: `_, _ = s.db.Exec("ALTER TABLE ...")` (store.go:60)
- ✅ Rate limiter map never evicts (middleware.go:35-51)
- ✅ ResponsePath is top-level map key only; missing path silently returns full response (tool.go:226-233)

### Risk assessments

- **K1 (Ghost tools on Stdio) — HIGH RISK**: The `filterWriter` approach parses streaming JSON-RPC output from the MCP SDK's internal wire format. If `mcp-go` changes serialization (buffering, framing), this breaks silently. **Mitigation**: pin `mcp-go` version tightly; add an integration test that verifies the wire format assumption; test partial writes and buffered lines.
- **S1 (credentialRef fail-closed) — HIGHEST PRIORITY**: This is the most impactful security fix in either plan — a typo in `credentialRef` silently sends the env var name as a password. Simple fix, zero dependencies. **Recommend implementing first.**
- **C1 (cache backend interface) — LARGEST TASK**: Consider splitting into two commits: (1) extract the `Backend` interface + `RedisBackend`, (2) add `MemoryBackend`.

### Superseded audit findings

- **C6 (FlushModule patterns)** is now active task C5: use explicit `FlushAll` and resolve module membership through the store.
- **K6 (resource relative URLs)** is folded into active task S1.

### Recommended cross-plan ordering

Run security fixes first regardless of plan priority:

```
1. S1 (credentialRef fail-closed)     — highest-impact security fix, zero deps
2. S3 (compose credential)            — one-liner security fix
3. S2 (shared log redaction)          — security, before auth logs more data
4. ../stalled/Plan-Auth.md A1–A7                 — auth core and principal limiting
5. ../stalled/Plan-Auth.md A8–A9                 — bridgectl token + api-token plumbing
6. C1 → C2 → C3 → C4 → C5            — cache
7. K2 → K3 → K4 → K5                 — correctness (low-risk)
8. K1 (ghost tools on stdio)          — high-risk, do after K2-K5 stable
9. D1 → D2 → D3                      — CLI (build on A8's newRequest helper)
10. ../stalled/Plan-Auth.md A10, A11 + D4/D5     — CORS and documentation release
```

## Historical Audit Addendum (2026-08-19) — Do Not Execute

Verified against code. Its task list is superseded by the active tasks above: C4 is active, legacy `invalidateOn` remains deferred, C6 is active C5, resource URLs are in S1, and the shadowed CLI output flag remains deferred.

- **C4:** promoted to active task C4.
- **C5:** legacy `invalidateOn` alias remains deferred; documentation uses `flushOn`.
- **C6:** promoted and renumbered as active task C5.
- **K6:** folded into active task S1.
- **D6:** the shadowed `tool get --output` flag remains deferred.
