# ERPBridge read performance check

## Result

**100/100 requests succeeded** with no tool errors. The run lasted 11.946 seconds, averaging 8.37 completed requests per second. Client-observed latency was 361 ms median, 662 ms p95, 691 ms p99, and 695 ms maximum.

## Conditions

- Started: 2026-10-18T17:47:18.724Z; duration: 11946 ms.
- Connection: configured `mcp__erpnext_bridge` runtime MCP. Target deployment version and server resource metrics were not available.
- Workload: 10 batches of 10 concurrent calls, with batch starts paced to at least one second apart. This is a burst workload, not an even 10-RPS stream.
- Read-only calls: `list_items`, `list_sales_orders`, `list_purchase_orders`, `list_assets`, and `search_records(Item)`; 20 calls each.
- Each list/search requested only `name`, with a 100-row page cap. No writes, retries, or response-body logging.
- Stop rule: stop after five failed calls. Actual stop: none.

## Measurements

| Tool | Success / calls | Rows per response | Median ms | p95 ms |
| --- | ---: | ---: | ---: | ---: |
| `list_items` | 20/20 | 56 | 337 | 628 |
| `list_sales_orders` | 20/20 | 5 | 351 | 643 |
| `list_purchase_orders` | 20/20 | 10 | 352 | 656 |
| `list_assets` | 20/20 | 3 | 370 | 662 |
| `search_records(Item)` | 20/20 | 56 | 390 | 634 |

Latency percentiles use the nearest-rank method over client-side request durations. Throughput is completed calls divided by total wall time, including pacing. The first batch includes connection warm-up, so this small sample is not a stable capacity estimate.

## Evidence and limits

- Raw evidence: [erpbridge-perf-evidence.json](erpbridge-perf-evidence.json), one status, duration, batch, and row count per call. It contains no record names, response bodies, or credentials.
- The test did not record CPU, memory, network use, database load, or other-user latency. It cannot establish server memory performance or a maximum sustainable RPS.
- Small collections returned at most 56 rows. The 100-row cap was never reached, so this did not stress large-result serialization.
