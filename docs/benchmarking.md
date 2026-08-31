# Benchmarking

ERPBridge includes deterministic Go microbenchmarks for per-tool cache and
request-preparation paths. They run in process and do not require Redis,
MockERP, Docker, credentials, or a live MCP connection.

Run the suite:

```bash
make bench
```

The command uses `-run '^$'`, so it does not run ordinary tests. It reports
latency and allocations with `ns/op`, `B/op`, and `allocs/op`.

For a local comparison, keep the machine otherwise idle and collect repeated
samples:

```bash
go test -run '^$' -bench . -benchmem -count=5 ./internal/cache ./internal/mcp
```

Use benchmark values to identify local regressions in the covered code paths.
Do not treat them as production-capacity claims or CI pass/fail thresholds.

## Local baseline

The following five-sample medians were measured on 2026-08-31 with Go 1.26.2
on Linux/amd64, using an 11th Gen Intel Core i5-1135G7. They are a local
baseline only.

| Benchmark | Median | Allocation |
| --- | ---: | ---: |
| `ArgsHash/empty` | 2.5 ns/op | 0 B/op, 0 allocs/op |
| `ArgsHash/small` | 2348 ns/op | 384 B/op, 7 allocs/op |
| `ArgsHash/nested` | 4352 ns/op | 656 B/op, 12 allocs/op |
| `ExactKey` | 2910 ns/op | 528 B/op, 11 allocs/op |
| `MemoryManager/hit` | 8500 ns/op | 1082 B/op, 22 allocs/op |
| `MemoryManager/set` | 5548 ns/op | 977 B/op, 18 allocs/op |
| `CacheMiddleware/hit` | 17287 ns/op | 2662 B/op, 58 allocs/op |
| `CacheMiddleware/disabled` | 236 ns/op | 144 B/op, 3 allocs/op |
| `PrepareERPCall/legacy_get_query` | 2193 ns/op | 613 B/op, 11 allocs/op |
| `PrepareERPCall/generated_locations` | 5211 ns/op | 1625 B/op, 24 allocs/op |
| `PrepareERPCall/complete_body` | 1329 ns/op | 240 B/op, 6 allocs/op |

## Concurrent cache-hit stress test

The stress benchmark shares one pre-seeded in-memory cache entry across
parallel callers. It exercises cache-key creation, the cache middleware,
JSON decoding, the lifecycle read lock, and the memory LRU lock. It does not
call Redis, an ERP, plugins, or an HTTP server.

```bash
make stress
```

This command runs the `CacheMiddleware/hit_parallel` benchmark for five seconds
at one and eight CPUs. On the same local environment, five-sample medians were:

| CPUs | Median | Throughput | Allocation |
| ---: | ---: | ---: | ---: |
| 1 | 12952 ns/op | 77206 ops/s | 2656 B/op, 58 allocs/op |
| 8 | 5653 ns/op | 176903 ops/s | 2664 B/op, 58 allocs/op |

The eight-CPU samples varied from 171757 to 255476 ops/s because callers
contend for the shared in-memory LRU lock. Use the median for local regression
comparison. Do not use these values to size a production deployment.
