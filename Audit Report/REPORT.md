# ToolPlane presentation evidence report

## 1. Executive summary

**The retained evidence supports integration feasibility and selected boundary behavior in one historical ERPNext deployment profile. A separate demo performance record adds 100 successful read calls with client-observed latency measurements. Neither evidence set establishes a completed conformance campaign, production reliability, unchanged ERP implementation, lower maintenance effort, or improved agent task success.**

This package copies existing repository records for presentation use. No live ERP call, model trial, performance experiment, or implementation test was run during export. The export verifies copy integrity, not the truth or provenance of the original experiments.

The most useful presentation evidence is:

1. Ten historical sequence records report 50 successful MCP operation outcomes across five ERPNext operations.
2. Seven of eight historical API-to-tool probe rows passed all structural checks. The eighth row exposes a response-mapping limitation and an eligibility-oracle inconsistency.
3. Three recorded role-based access cases matched their expected allow/deny outcomes.
4. Selected boundary records report rejection, convergence, authorization, withdrawal, and revision behavior, with important measurement gaps.
5. A separate demo run reports 100/100 successful read calls in 11.946 seconds, with 361 ms median and 662 ms p95 client-observed latency. The raw samples reproduce these values.

**Required qualification:** Historical observations are not release-verified. Individual executions are not bound to an observed ERPBridge binary or container image. Repeated record counts are not independently numbered replications.

## 2. Package contents and interpretation

- `REPORT.md`: this detailed interpretation and presentation guidance.
- `README.md`: navigation and verification instructions.
- `MANIFEST.json`: original path, package path, byte length, and SHA-256 for each copied source file; also records export time and repository HEAD.
- `sources/`: byte-identical copies with paper repository-relative paths preserved. External demo files are under `sources/erpbridge-demo/.agents/artifacts/`; their manifest entries specify the external source root.

All evidence links below point into `sources/`. Original source files were left unchanged. Copied documents may contain their own links to files outside this curated package. Those links are retained unchanged; the package is not a complete executable reproduction environment.

The manifest's repository HEAD identifies the paper repository at export time. It does not identify the binary that ran a historical experiment. File hashes establish only that the copied bytes match their sources at export time.

### Evidence classes

| Class | Meaning | Permitted use |
|---|---|---|
| Historical observation | A retained result file reports an outcome | Describe the recorded outcome with its scope and missing measurements |
| Implementation documentation | Architecture notes or manuscript describe mechanisms | Explain the documented design; do not treat it as a new live test |
| Profile or oracle requirement | A document specifies expected behavior | Explain planned acceptance criteria, not completed coverage |
| Documentation comparison | Pinned external sources are compared in an existing matrix | Position features, not overall quality or performance |
| Missing or blocked evaluation | No qualifying measurement is present | Show an evaluation gap, not zero success or a negative finding |

## 3. Evidence E1: ERPNext integration feasibility

### Recorded result

The manuscript describes ten retained sequence records. Each sequence requested five operations:

- `list_companies`
- `list_items`
- `get_item`
- `create_item`
- `delete_item`

These records report 50 successful MCP outcomes. Separate direct checks observed creation of a synthetic Item and its absence after deletion. The representative invocation artifact lists all five operations with status 200 and overall status `completed`.

### Sources

- [Manuscript evaluation and observation table](sources/paper/sections/evaluation.tex)
- [Ten retained sequence directories](sources/research/evidence/evaluation/results/task11/happy-path/)
- [Representative invocation](sources/research/evidence/evaluation/results/task11/happy-path/ac-01-happy-path-10/invocation.json)
- [Representative run metadata](sources/research/evidence/evaluation/results/task11/happy-path/ac-01-happy-path-10/metadata.json)
- [Representative state checks](sources/research/evidence/evaluation/results/task11/happy-path/ac-01-happy-path-10/state.json)

### Supported interpretation

The records support feasibility of exposing and invoking these ERPNext operations through MCP in the retained historical profile. State checks add evidence beyond protocol status for creation and deletion.

### Limits

- The sequence records are labelled `smoke` and `nonFinal`.
- All repetition identifiers are 1. Ten directories mean ten retained records, not independently numbered replications.
- The runner assigned five planned dispatches per sequence. It did not separately measure connector entries or upstream HTTP requests for these calls.
- Not every list/read result was compared with the full expected semantics.
- Historical source/image identifiers and six baseline hashes were generated placeholders.
- A successful invocation does not prove that ERP source, contract, or deployment remained unchanged.

### Presentation wording

> Ten retained smoke-test sequence records report 50 successful MCP operation outcomes across five ERPNext operations. This supports integration feasibility in one historical deployment profile.

Avoid: “100% reliable,” “50 independently verified ERP HTTP requests,” or “proved zero ERP changes.”

## 4. Evidence E2: API-to-MCP structural transformation

### Recorded result

The eight-row historical probe covers six MockERP operations and two sanitized ERPNext operations.

| Structural check | Passing rows | Denominator |
|---|---:|---:|
| Syntactic validity | 8 | 8 |
| Tool name | 8 | 8 |
| Input schema fidelity | 8 | 8 |
| Parameter locations | 8 | 8 |
| HTTP method | 8 | 8 |
| HTTP path | 8 | 8 |
| Credential reference | 8 | 8 |
| Response mapping | 7 | 8 |
| All structural checks | 7 | 8 |

Valid-call status is `unavailable` for all eight rows. These are structural observations, not eight successful live invocations.

### Failure interpretation

`GET /integration/echo/last` failed `response_mapping_correct`. The transformation-profile audit explains that the expected `data` object used `allOf` and `nullable`, which were outside the historical declared subset. The generator emitted `responsePath: data` without an output schema. The audit preserves this as an **eligibility-oracle inconsistency and response-mapping limitation**, not a supported-pass or a corrected rerun.

Keep the historical 7/8 denominator visible. Do not remove the failed row to produce a new 7/7 supported-subset claim.

### Sources

- [Row-level transformation results](sources/research/evidence/evaluation/results/transformation.csv)
- [Failure record](sources/research/evidence/evaluation/results/transformation-failures.md)
- [Historical eligibility audit and proposed profile](sources/research/evidence/evaluation/transformation-profile.md)
- [Generated descriptive summary](sources/research/evidence/evaluation/results/summary.md)

### Presentation wording

> In an eight-operation historical structural probe, seven rows passed all checks. Tool naming, input schemas, parameter locations, HTTP methods, paths, and credential references passed in all eight rows. One response-mapping case exposed a profile/oracle limitation.

Avoid: “87.5% general API conversion accuracy” or “all generated tools work.” The sample is small, heterogeneous, and not a generalization study.

## 5. Evidence E3: Role-based access behavior

### Recorded result

| Case | Principal label | Expected | Recorded |
|---|---|---|---|
| `reader-allowed` | `erpnext_reader` | Allow | Allow |
| `reader-wrong-role` | `inventory_reader` | Deny | Deny |
| `reader-missing-role` | `erpnext_reader` | Deny | Deny |

All three rows record HTTP 200. The allowed case has `is_error=false`; the denied cases have `is_error=true`. Therefore HTTP status alone is not the authorization decision.

### Sources

- [Governance CSV](sources/research/evidence/evaluation/results/governance.csv)
- [Collection-status caveat](sources/research/evidence/evaluation/results/governance-blockers.md)

### Supported interpretation and limits

The three recorded decisions match expectations for those cases. They do not demonstrate complete role coverage, argument-scoped authorization, tenant isolation, or general security. The collection-status note says raw run provenance was unavailable during that workspace check and explicitly does not treat existing CSV rows as newly collected results.

### Presentation wording

> Three historical role-based access cases matched expectations: one allow and two denies. Denial was represented in the error result, not by a non-200 HTTP status.

## 6. Evidence E4: Selected architecture boundaries

The retained boundary artifact contains ten records for each of seven scenarios, 70 boundary records in total. Some scenarios support more than one architectural requirement. The count is not 70 independent proofs or complete requirement coverage.

| Requirement | Retained observation | Missing evidence or restriction |
|---|---|---|
| Exact-resource admission | Changed same-version content returned HTTP 500 in 10/10 records; queried digest stayed unchanged | Generic failure does not identify the rejection rule, reviewer authority, or every bound field |
| Discovery convergence | Controller status reported convergence after injected failure and restart in 10/10 records | No retained recovery duration, attempt delta, repaired-discovery check, or later ERP call |
| Authorization | Denied calls recorded zero connector entries in 10/10 records | One fixed-principal path; no broad identity, cache-release, or argument-policy coverage |
| Queued withdrawal | Instrumented queued calls recorded zero connector entries in 10/10 records | Pause arrival was not acknowledged; no proof of distributed revocation or the current commitment boundary |
| Revision consistency | Serving transition, serving-revision digest binding, and mutated-digest rejection are represented | No active non-serving call, accepted call during transition, or independent inactive-revision witness |
| Credential indirection | Resources contain an opaque credential reference; authenticated calls succeeded | No comprehensive leakage scan, credential lifecycle evaluation, or separate credential-reference boundary witness |

### Sources

- [Retained boundary artifact](sources/research/evidence/evaluation/results/task12/task12-final-06/boundary.json)
- [Conformance summary](sources/research/evidence/evaluation/results/invariant-conformance.md)
- [Detailed conformance status](sources/research/evidence/evaluation/results/invariant-conformance.json)
- [Manuscript's qualified interpretation](sources/paper/sections/evaluation.tex)

### Interpretation rule

The summary labels all six invariants `observed`, but also marks architecture coverage `partial`, historical attribution `unknown`, and overall status `not-release-verified`. “Observed” is not “fully validated.” Prefer the manuscript table's more specific statements over a green six-of-six pass chart.

The historical deployment used behavior-affecting instrumentation. Do not assume these records validate current-source mechanisms. The manuscript describes current focused tests separately, but those tests were not run during this export and their source files are not included as new test evidence.

## 7. Evidence E5: Discovery/version consistency example

The version-consistency CSV contains two descriptive rows under reverse registration order (`v2_then_v1`):

| Row | Advertised v2-only argument | Resolved version | Agreement |
|---|---|---|---|
| Before fix | Absent | `2.0.0` | False |
| After fix | Present | `2.0.0` | True |

The before-fix row records HTTP 200 for the v2-only argument call. That field is blank after the fix. A blank is missing evidence, not a successful post-fix invocation.

Source: [Version-consistency CSV](sources/research/evidence/evaluation/results/version-consistency.csv).

Suggested use: a small defect-and-repair case study showing a recorded discovery/resolution inconsistency. Do not describe it as a complete regression suite or conclusive revision-safety test.

## 8. Evidence E6: Exploratory overhead records

Both rows concern `GET /resource/Company` in the `real-sanitized` corpus. Each row is labelled 30 runs.

| Configuration | Direct p50, ms | ToolPlane p50, ms | Direct p95, ms | ToolPlane p95, ms | p95 difference, ms |
|---|---:|---:|---:|---:|---:|
| Cache disabled, auth disabled | 20.352 | 14.498 | 23.115 | 24.738 | +1.623 |
| Cache disabled, admin authenticated | 21.020 | 15.355 | 24.005 | 23.664 | −0.341 |

Sources: [Overhead CSV](sources/research/evidence/evaluation/results/overhead.csv) and [collection caveat](sources/research/evidence/evaluation/results/governance-blockers.md).

These are sparse summary measurements, not a trial-level trace in this package. The negative difference is not evidence that the bridge generally accelerates ERP requests. No uncertainty analysis or general overhead bound is established. Keep these results in an appendix rather than as the main contribution slide.

## 9. Evidence E7: Architecture and capability positioning

The existing capability matrix compares AutoMCP arXiv:2507.16044v4, FastMCP 4.0.3, IBM MCP Context Forge 1.0.9, and the ERPBridge/ToolPlane reference prototype across eight criteria:

1. API-derived tools.
2. Selective exposure.
3. Explicit revision targeting.
4. Approval-bound exact contract-and-mapping resources.
5. Persisted desired-state reconciliation.
6. Per-call revision consistency.
7. MCP client authentication.
8. Upstream API credential mediation outside model arguments.

Source: [Version-specific capability matrix](sources/research/evidence/comparison/system-capability-matrix.md).

Use the matrix's definitions: `D` means directly documented, `P` means partial or profile-specific, and `NE` means not established by the reviewed source. **NE does not mean absent.** Ratings are not comparable quality scores.

This is an existing documentation-based comparison. No external source was retrieved or independently revalidated during export. ToolPlane rows describe a reference-prototype profile and must not be promoted to universal guarantees or assumed to match every current revision. In particular, the manuscript notes that admission and activation occur in one operation; the mechanism does not independently prove prior human review or a separate approval authority.

## 9A. Evidence E8: Demo read performance check

This is a separate run supplied from `/home/nimendra/erpbridge-demo/.agents/artifacts/`. It is not part of the earlier ERPNext conformance records or the direct-HTTP overhead comparison in Section 8. Do not combine the measurements or transfer deployment assumptions between them.

### Sources and run conditions

- [Original performance report, copied unchanged](sources/erpbridge-demo/.agents/artifacts/erpbridge-perf-report.md)
- [Raw per-call evidence, copied unchanged](sources/erpbridge-demo/.agents/artifacts/erpbridge-perf-evidence.json)

The recorded start time is `2026-10-18T17:47:18.724Z`. The report identifies the connection as the configured `mcp__erpnext_bridge` runtime MCP. The target deployment version and server resource metrics were unavailable.

The workload comprised ten batches of ten concurrent calls. Batch starts were paced to at least one second apart. This is a burst workload, not an evenly spaced 10-request-per-second stream. The five read tools each received 20 calls. Each call requested only `name`, with a 100-row page cap. The report specifies no writes, retries, or response-body logging. The stop rule was five failed calls; the evidence records no stop and no failed call.

### Overall result

| Metric | Recorded or recomputed value |
|---|---:|
| Calls | 100 |
| Successful calls | 100/100 |
| Tool errors | 0 recorded |
| Total elapsed time | 11.946 s |
| Completed calls / total wall time, including pacing | 8.37 calls/s |
| Median client latency, nearest rank | 361 ms |
| p95 client latency, nearest rank | 662 ms |
| p99 client latency, nearest rank | 691 ms |
| Maximum client latency | 695 ms |

### Per-tool result

| Tool | Success / calls | Rows per response | Median, ms | p95, ms |
|---|---:|---:|---:|---:|
| `list_items` | 20/20 | 56 | 337 | 628 |
| `list_sales_orders` | 20/20 | 5 | 351 | 643 |
| `list_purchase_orders` | 20/20 | 10 | 352 | 656 |
| `list_assets` | 20/20 | 3 | 370 | 662 |
| `search_records(Item)` | 20/20 | 56 | 390 | 634 |

During import, the raw JSON was checked for 100 samples, ten samples per batch, 20 samples per tool, `ok=true`, and `error=null`. Overall and per-tool latency percentiles were recomputed using nearest rank. Throughput was recomputed as 100 divided by 11.946 seconds. All values match the supplied report after rounding throughput to two decimal places. This verifies internal numerical consistency, not independent execution or deployment identity.

### Supported interpretation and limits

The record supports successful completion of this bounded read-only burst workload and describes client-observed latency under its stated conditions.

- It is one short run, not a maximum-capacity or long-duration reliability study.
- Client timings include client/transport effects and first-batch connection warm-up. They are not isolated ERPBridge server processing times.
- The evidence records duration and batch labels, but not per-call timestamps or batch-start timestamps. Batch pacing and actual concurrent overlap cannot be independently reconstructed from the JSON alone.
- CPU, memory, network, database load, and impact on other users were not measured.
- Collection sizes were at most 56 rows; the 100-row cap was never reached. Large-result serialization was not stressed.
- There is no paired direct-HTTP baseline. These latencies do not measure bridge overhead or show an improvement over direct ERP access.
- The files do not establish an immutable target source/image identity, hardware profile, or complete cache/auth configuration. The earlier historical deployment profile must not be assumed to apply.
- Success flags and row counts do not constitute a complete semantic correctness check of returned business records.
- These read-only results do not validate write behavior or the earlier conformance invariants.

### Presentation wording

> A separate demo read test completed 100/100 calls across five tools in 11.946 seconds. Client-observed latency was 361 ms median and 662 ms p95 under ten-call burst batches. This is a bounded observation, not a sustained-capacity or server-resource benchmark.

Avoid: “sustains 10 RPS,” “maximum throughput is 8.37 RPS,” “100% production reliability,” “low server memory use,” or “662 ms ERPBridge processing overhead.”

## 10. Deployment scope

The manuscript describes the retained historical profile as ERPNext 16.17.0 / Frappe 16.17.5, Streamable HTTP, loopback targets, one ToolPlane process, SQLite desired state, disposable fixed principals, environment-backed credentials, and disabled protected-tool caching. Boundary instrumentation included pause/fault injection and connector-entry counting.

Sources: [Manuscript profile](sources/paper/sections/evaluation.tex) and [evaluated deployment profile](sources/research/evidence/evaluation/evaluated-deployment-profile.yaml).

These are descriptions retained in the repository, not newly verified runtime identifiers. They do not extend the findings to distributed deployments, other ERP products, all OpenAPI constructs, or production workloads. They also do not identify the target of the separate demo performance run in Section 9A.

## 11. Provenance and release limitations

The provenance audit identifies these blockers:

- Historical metadata does not bind each run to an observed binary/image.
- Source/image identifiers and ERP baseline hashes include synthetic placeholders.
- The release-freeze record is incomplete and collection is not authorized under that record.
- The archive manifest records a `not-authorized` freeze with empty freeze fields.
- The expected results were author-authored, not independently reviewed.
- The retained subset does not complete the frozen scenario registry.

Sources: [Provenance audit](sources/research/evidence/evaluation/provenance-audit.md), [release-freeze record](sources/research/evidence/evaluation/release-freeze.md), and [original archive manifest](sources/research/evidence/evaluation/archive-manifest.json).

These defects block release integrity. They do not prove that the recorded calls never occurred or that an invariant failed. Preserve the distinction between “unknown attribution” and “disproved behavior.”

The new `MANIFEST.json` is an export inventory only. It does not repair these historical provenance defects or authorize collection.

## 12. Missing evidence and unsupported claims

| Proposed claim | Available record | Correct conclusion |
|---|---|---|
| Better agent task success | Usability CSV has only its header; no model was configured or run | Not measured |
| Lower maintenance effort | 24 evolution rows, all blocked across four conditions | Not measured |
| Faster API adaptation | No completed repair/replay measurements | Not measured |
| ERP source/deployment preserved | Historical baseline hashes are placeholders | Not demonstrated by before-and-after measurements |
| Production reliability | Bounded smoke/boundary records and one separate 100-call read run | Not established |
| Maximum sustainable RPS or server resource efficiency | Short paced burst run without CPU/memory metrics | Not established |
| Complete OpenAPI support | A stricter transformation profile defines requirements | Requirements are not completed execution results |
| Complete security | Selected role and boundary cases | Not established |
| Verified current release | Incomplete freeze; unknown run-to-binary attribution | Not release-verified |

Supporting files: [Usability status](sources/research/evidence/evaluation/results/agent-usability.md), [usability CSV](sources/research/evidence/evaluation/results/agent-usability.csv), [evolution CSV](sources/research/evidence/evaluation/results/evolution.csv), [repair log](sources/research/evidence/evaluation/results/repair-log.md), and [OpenAPI support matrix](sources/research/evidence/evaluation/openapi-support-matrix.md).

## 13. Recommended presentation sequence

### Slide 1: ERP integration feasibility

Show the five operation names and the recorded 10-sequence / 50-outcome count. Include a cropped, readable extract of the representative invocation JSON. State that the results are smoke records and do not prove unchanged ERP code.

### Slide 2: Transformation fidelity and an honest failure

Show the structural-check table from Section 4. Highlight the response-mapping row as 7/8. Explain the unsupported-schema/oracle inconsistency. Keep structural checks separate from live invocation success.

### Slide 3: Governance behavior

Show the three allow/deny cases. If space permits, add the historical denied-call connector-entry observation with its fixed-principal and instrumentation limits. Do not label the system “secure” based on these checks.

### Slide 4: Bounded demo read performance

Show the 100/100 call count, 11.946 s wall time, 361 ms median, and 662 ms p95. State that the workload used ten batches of ten concurrent reads across five tools, with paced batch starts. Label 8.37 calls/s as achieved completion rate including pacing, not server capacity. Keep this run separate from the earlier 50-outcome smoke records and direct-HTTP overhead table.

Use this slide-specific footer:

> One short read-only demo run; client-observed timings. Deployment version and server resource metrics were not recorded. Not a capacity or overhead benchmark.

### Optional appendix slides

- Boundary observation and evidence-gap table.
- Before/after discovery-consistency example.
- Exploratory overhead table.
- Version-pinned feature comparison with the D/P/NE legend.
- Evaluation gaps and the next measurement campaign.

### Common footer

> Preliminary historical observations from a bounded ERPNext profile; not release-verified. Historical run-to-binary attribution is unknown.

### Speaker response to “What does this prove?”

> The historical records support feasibility and selected behavior in one earlier profile. A separate 100-call read run adds bounded client-latency observations. Together they do not establish complete conformance, unchanged ERP implementation, production reliability, maintenance savings, or better agent task success.

## 14. Next evidence needed

These are recommendations, not work completed by this export:

1. Authorize a pre-run freeze with observed source, image, schema, SDK, contract, oracle, profile, scenario, and runner identifiers.
2. Collect uniquely identified repetitions under that freeze and retain trial-level artifacts.
3. Measure ERP baseline preservation before and after the campaign.
4. Use direct connector and HTTP instrumentation where dispatch counts are claimed.
5. Retain timed recovery, repaired discovery, and a successful post-recovery ERP call.
6. Cover active non-serving revisions and accepted calls during serving transitions.
7. Obtain independent oracle review.
8. Run the model usability matrix and explicit evolution repair/replay procedures before claiming usability or maintenance benefits.

## 15. Export acceptance criteria

The export is acceptable when every manifest entry exists, source and copy hashes match, the report's local file links resolve, and no `.env` file is packaged. These checks concern this copy operation only. They do not validate the historical experiments or constitute a comprehensive secret-value audit.
