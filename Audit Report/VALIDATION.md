# Export validation

## Checks performed

A local Python validation script completed successfully with exit code 0.

| Check | Result |
|---|---|
| Manifest entry count | 116 copied source files |
| Source versus copy SHA-256 | All 116 match, including both external demo files |
| Copy versus manifest SHA-256 | All 116 match |
| Copy size versus manifest byte count | All 116 match |
| Total copied source size | 238,154 bytes |
| Local links in REPORT.md, README.md, and copied demo report | All resolve on the filesystem |
| `.env*` filename exclusion | No matching files packaged |
| Invocation artifacts | 10 files, 50 operation records, all recorded status 200 |
| Historical transformation CSV | 8 rows, 7 passing all structural checks |
| Governance CSV | 3 rows, all recorded decisions match expectations |
| Evolution CSV | 24 rows, all blocked |
| Usability CSV | Zero data rows |
| Demo performance samples | 100 samples; ten batches of ten; 20 calls per tool; all `ok=true`, all `error=null` |
| Demo latency, nearest rank | Recomputed median 361 ms, p95 662 ms, p99 691 ms, maximum 695 ms |
| Demo throughput | Recomputed 8.37 calls/s after rounding, using 11.946 s total wall time |
| Demo per-tool metrics | Row counts, medians, and p95 values match the supplied report |

The demo files were imported without changes from `/home/nimendra/erpbridge-demo/.agents/artifacts/`. Their manifest entries specify the external source root. Numerical checks confirm internal consistency, not an independent execution of the run.

A session diagnostic check reported a missing link under the hidden `.agents` directory. Filesystem checks confirm that the target exists. Such a link warning is not evidence of a missing copied file.

## Scope limits

- Hash checks validate the copy operation, not the original experiment provenance.
- No live ERPNext or ERPBridge operation was run.
- No model API call, new timing experiment, or implementation test was run.
- No external literature was discovered or revalidated.
- No comprehensive secret-value audit was performed. The filename check establishes only that `.env*` files are absent.
- The historical package remains not release-verified.
- The export manifest inventories copied source files, not the newly authored report, README, or this validation note.
