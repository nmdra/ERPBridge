# Plan: Concurrent Cache Benchmark Stress Test

## Goal

Add a deterministic concurrent cache-hit stress benchmark, measure it locally,
and publish its scope and results in both ERPBridge documentation repositories.

## Current State

- `make bench` runs the current in-process cache and MCP benchmarks without
  ordinary tests (`Makefile:47-55`), and the developer guide explicitly states
  that those values are not production-capacity claims (`docs/benchmarking.md:1-24`).
- Cache hits run through `Server.CacheMiddleware`, which reads the cache under a
  lifecycle read lock and returns the unmarshaled result without invoking the
  next handler (`internal/mcp/middleware.go:424-500`).
- The in-memory backend serializes all LRU access through a mutex and copies
  the cached value before returning it (`internal/cache/memory_backend.go:17-80`).
- Existing `BenchmarkCacheMiddleware/hit` seeds a memory cache and confirms the
  next handler is not called (`internal/mcp/middleware_bench_test.go:17-52`).
- The public documentation registers ERPBridge server guides in
  `sidebars.ts` and currently has no benchmarking page
  (`../erpbridge-docs/sidebars.ts:6-30`). Its contribution instructions require
  a corresponding root-doc update, `CHANGELOG.md` entry, and `npm run build`
  (`../erpbridge-docs/AGENTS.md:21-25`).

## Decisions

1. Stress only the cache-hit path in process with a shared, pre-seeded
   `MemoryManager` and `Server.CacheMiddleware` instance. It is the existing
   safe hot path and isolates lock contention without making claims about ERP,
   Redis, HTTP, plugin, or database throughput.
2. Use `b.RunParallel`, `-cpu=1,8`, `-benchtime=5s`, and `-count=5`. Record
   `ops/s`, `ns/op`, bytes, and allocations. Test with `-race` separately;
   race-mode timing is excluded from reported performance data.
3. Publish measured medians and the exact hardware/runtime configuration. State
   the benchmark's limits explicitly; do not add a CI threshold, persisted raw
   output, or a production-capacity assertion.

## Tasks

- [x] **Task 1: Add the bounded concurrent benchmark and runner target.** Add
  a `hit_parallel` sub-benchmark to `BenchmarkCacheMiddleware`; seed one shared
  memory cache entry, use immutable request arguments, verify all iterations
  return a result without calling the next handler, and report `ops/s`. Add
  `make stress` to run only that benchmark with the selected CPU counts and
  duration. **Seam:** `Server.CacheMiddleware` over `cache.MemoryBackend`.
  **Files:** `internal/mcp/middleware_bench_test.go`, `Makefile`.
  **Verify:** `make stress` and `go test -race -run '^$' -bench
  '^BenchmarkCacheMiddleware/hit_parallel$' -benchtime=1s ./internal/mcp`.

- [x] **Task 2: Collect repeatable stress measurements.** Run the stress
  command five times without other workload, capture the `-cpu=1` and
  `-cpu=8` results, calculate medians, and retain no raw output artifact.
  **Seam:** Go benchmark runner. **Files:** no files expected. **Verify:**
  `go test -run '^$' -bench '^BenchmarkCacheMiddleware/hit_parallel$' -benchmem
  -cpu=1,8 -benchtime=5s -count=5 ./internal/mcp`.

- [x] **Task 3: Publish benchmark scope and results.** Add median benchmark
  results and explicit microbenchmark limitations to the root guide and root
  Unreleased changelog. Create a public ERPBridge benchmarking page, register
  it in the server sidebar, mirror the command, scope, and results, and update
  the public Unreleased changelog. **Seam:** root developer guide to public
  Docusaurus documentation. **Files:** `docs/benchmarking.md`,
  `CHANGELOG.md`, `../erpbridge-docs/docs/erpbridge/benchmarking.mdx`,
  `../erpbridge-docs/sidebars.ts`, `../erpbridge-docs/CHANGELOG.md`.
  **Verify:** `make test`, `git diff --check`, and `npm run build --prefix
  ../erpbridge-docs`.

## Verification

1. The concurrent benchmark has no race detector finding and never reaches its
   fallback handler.
2. Results identify CPU count, duration, allocation values, and throughput.
3. Both documentation sites clearly describe an in-process cache-hit stress
   benchmark rather than a production load test.
