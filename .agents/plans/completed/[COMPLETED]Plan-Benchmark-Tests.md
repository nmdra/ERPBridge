# Plan: Deterministic Go Benchmark Tests

## Goal

Establish a small, repeatable Go microbenchmark suite for ERPBridge's
in-process per-tool hot paths. It must measure latency and allocations without
requiring Redis, MockERP, credentials, Docker, or a live MCP connection.

## Current State

- The repository's only Go test target is `make test`, which runs `go test
  ./...`; it has no benchmark target (`Makefile:47-50`). There are no
  repository-owned benchmark files or uses of `testing.B` (`glob **/*bench*`
  and `grep Benchmark|testing.B` scoped to owned Go sources, 2026-08-31).
- The active and upcoming plan index has no benchmark work item, so this is a
  separate upcoming plan (`.agents/plans/README.md:6-25`). It must not change
  the unrelated active E2E remediation or API-key propagation plans.
- Exact-cache keys canonically sort argument names, JSON encode key/value
  entries, and SHA-256 hash the result (`internal/cache/manager.go:111-137`).
  Cache `Get` and `Set` also exercise the selected backend plus response
  envelope serialization (`internal/cache/manager.go:59-106`); the default
  local backend is a mutex-protected bounded LRU that copies values on reads
  and writes (`internal/cache/memory_backend.go:17-80`).
- ERP request preparation sorts input arguments, maps path/query/header/body
  fields, serializes bodies, applies base-URL rules, and resolves credentials
  before constructing the connector configuration (`internal/mcp/tool.go:213-329`).
- The cache middleware wraps tool calls with cache lookup, result
  serialization, lifecycle generation protection, and cache population;
  successful cache hits bypass the next handler (`internal/mcp/middleware.go:424-500`).
- Prometheus metrics are process-global and the full middleware chain records
  metric series and structured logs (`internal/mcp/middleware.go:348-421`), so
  an initial suite should measure the cache middleware in isolation rather
  than publish a noisy end-to-end throughput figure.
- Go's testing package provides `ResetTimer` to exclude fixture construction
  and `ReportAllocs` to include allocation statistics
  (<https://pkg.go.dev/testing#B.ResetTimer>,
  <https://pkg.go.dev/testing#B.ReportAllocs>).

## Decisions

1. Use standard Go `Benchmark*` functions in adjacent `*_bench_test.go` files.
   Adding a third-party runner or a baseline-comparison dependency is rejected:
   the first goal is stable local measurements, not CI performance gates.
2. Benchmark only deterministic in-process work: canonical cache key creation,
   memory-cache hit/set operations, request preparation, and cache-middleware
   hit/disabled paths. Redis, HTTP, MockERP, plugins, SQLite reconciliation,
   rate-limit rejection, and full server throughput are rejected because their
   I/O, timers, global metrics, or scheduling effects would obscure a local
   regression signal.
3. Every benchmark will create immutable fixtures before timing, call
   `b.ReportAllocs()`, and use `b.ResetTimer()` before its `b.N` loop. The
   benchmarks will not mutate shared argument maps or rely on wall-clock TTL
   expiry.
4. Add an explicit opt-in `make bench` target that invokes `go test -run '^$'
   -bench . -benchmem` only for the benchmarked packages. `make test` and CI
   remain unchanged, so routine correctness checks neither run benchmarks nor
   use their machine-specific numbers as pass/fail thresholds.
5. Document the command, the benchmark scope, and comparison protocol:
   run the same command on an otherwise idle machine, collect at least five
   samples with `-count=5`, and use `benchstat` only as an optional local
   comparison tool. Do not commit benchmark output or claim absolute
   production capacity from microbenchmark results.

## Scope

### In scope

- Go microbenchmarks for exact-key hashing, memory cache operations, ERP request
  preparation, and cache-middleware fast paths.
- An opt-in Make target and developer-facing benchmark instructions.
- Validation that normal tests still execute without running benchmarks.

### Intentionally out of scope

- Production code changes, cache behavior changes, or performance targets.
- Benchmark result snapshots, CI regression thresholds, dashboards, or a new
  external dependency.
- Redis, ERP, network, plugin, SQLite, browser, or full-server load testing.
- Concurrent throughput claims and `b.RunParallel`; these need a separate plan
  with CPU-affinity, contention, and acceptance-threshold decisions.

## Tasks

- [x] **Task 1: Add the opt-in benchmark entry point and usage guide.** Add a
  `bench` phony Make target that runs only `./internal/cache` and
  `./internal/mcp` with `-run '^$' -bench . -benchmem`; leave `make test`
  unchanged. Add a concise developer guide that describes the commands, scope,
  `-count=5` comparison practice, and the fact that microbenchmark values are
  not CI thresholds or production-capacity claims. **Seam:** Make target to Go
  benchmark runner. **Files:** `Makefile`, `docs/benchmarking.md`, and
  `docs/README.md`. **Verify:** `make bench`, `go test -run '^$' -bench .
  -benchmem ./internal/cache ./internal/mcp`, and `make test`.

- [x] **Task 2: Benchmark exact-cache work with the memory backend.** Add
  sub-benchmarks for empty, small, and nested/reordered argument maps through
  `argsHash`/`exactKey`; add manager-level memory-cache hit and set cases with
  a pre-seeded entry and fixed TTL-disabled configuration. Construct maps,
  manager, context, and response payload before timing; report allocations;
  retain returned values in a package-level sink if required to prevent
  compiler elimination. Do not use Redis or miniredis. **Seam:**
  `argsHash`/`exactKey` and `Manager.Get`/`Manager.Set` backed by
  `MemoryBackend`. **Files:** `internal/cache/manager_bench_test.go`.
  **Verify:** `go test -run '^$' -bench 'Benchmark(ArgsHash|ExactKey|MemoryManager)' -benchmem ./internal/cache` and `go test ./internal/cache -count=1`.

- [x] **Task 3: Benchmark deterministic MCP request and cache-hit processing.**
  Add `prepareERPCall` cases covering legacy GET query mapping, generated
  path/query/header/body mapping, and a JSON body. Use `b.Setenv` before
  timing where a base URL or credential reference is necessary, and avoid any
  outbound connector call. Add `CacheMiddleware` cases for a pre-seeded exact
  hit and a cache-disabled no-op handler; confirm the hit handler cannot run
  and the disabled path runs exactly once during fixture validation before the
  timer starts. **Seam:** `Tool.prepareERPCall` and `Server.CacheMiddleware`
  with `cache.NewMemoryManager`. **Files:** `internal/mcp/tool_bench_test.go`,
  `internal/mcp/middleware_bench_test.go`. **Verify:** `go test -run '^$' -bench 'Benchmark(PrepareERPCall|CacheMiddleware)' -benchmem ./internal/mcp` and `go test ./internal/mcp -count=1`.

- [x] **Task 4: Verify repeatability and preserve the normal quality gate.**
  Run each benchmark package five times without changing the environment, save
  results only as local terminal output, and investigate any case whose result
  is invalid or whose fixture unexpectedly performs I/O. Run the full existing
  test gate after benchmark additions; add no numeric assertion, checked-in
  baseline, or CI workflow change. **Seam:** the `make bench` command and the
  existing Go test suite. **Files:** no additional files expected. **Verify:**
  `go test -run '^$' -bench . -benchmem -count=5 ./internal/cache ./internal/mcp`
  and `make test`.

## Verification

1. `make bench` discovers and runs all new benchmarks, reports `ns/op`,
   `B/op`, and `allocs/op`, and does not run ordinary tests because of
   `-run '^$'`.
2. Each benchmark is deterministic and self-contained: no Docker service,
   Redis server, MockERP instance, network request, persisted database, or
   credential value is required.
3. Cache-hit and cache-disabled middleware cases preserve their stated handler
   behavior, and request-preparation cases exercise the current mapping
   contracts rather than a synthetic duplicate implementation.
4. `make test` remains green and has the same target behavior as before the
   benchmark work.

## Open Questions

None. A future CI performance gate or concurrent/load benchmark requires a
separate plan with a dedicated runner, machine-normalization strategy, and
explicit regression policy.
