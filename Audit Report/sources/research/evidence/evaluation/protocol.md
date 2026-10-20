# ToolPlane Architecture-Conformance Validation Protocol

## Purpose and result boundary

This protocol evaluates the ToolPlane architecture through scoped conformance evidence, not an agent, performance, maintenance, or OpenAPI-coverage benchmark. It is a protocol, not a record of completed results.

The result may state only that the evaluated ERPBridge release demonstrated the six invariants under the frozen single-process, loopback ERPNext profile. It does not establish production security, multi-process revocation, multi-tenant isolation, performance, maintenance savings, or generality to other ERP products.

Historical RQ1--RQ5 records, the four-condition pilot, the eight-row MockERP structural probe, and the retained smoke/boundary campaign remain non-final evidence. They are not inputs to a future release decision. `provenance-audit.md` records which historical deficiencies are verified and which source-to-run links remain unknown.

The report has two independent dimensions. **Architecture coverage** states whether the required scenarios and assertions have evidence. **Release integrity** states whether the inputs were authorized, runtime provenance was observed, and the archive was sealed. Missing or synthetic provenance blocks release integrity without being reported as an invariant failure.

## Frozen profile and local authentication

The campaign uses `evaluated-deployment-profile.yaml`: one synthetic ERPNext tenant, one ERPBridge process, Streamable HTTP, approved loopback origins, disabled response processors, disabled protected-tool caching, reads, and fixture-resettable Item writes.

Authentication is deliberately minimal for unattended local runs. Stable disposable ERPNext and ERPBridge test credentials are loaded from Git-ignored `evaluation/erpnext/.env`; they are not rotated between repetitions. The study uses a fixed allowed principal and fixed denied principal. It does not test token lifecycle, token rotation, absence handling, resolution timing, or resistance to disposable-token text in local-only logs. The `.env` file must not be included in the published artifact.

## Oracle governance

Expected outcomes are author-authored and must be committed and included in the authorized freeze before collection. The final campaign records when each expected outcome was last visible to its author. No independent or blind review is claimed. This is a construct-validity limitation, not a reason to alter outcomes after observing a run. Any oracle correction invalidates the affected frozen campaign and requires a preserved failed attempt plus a new freeze.

## Invocation and withdrawal observations

Each invocation trace identifies the selected and serving digests and demonstrates the selected revision at validation, policy, mapping, response processing, and output. A digest label without stage-level behavior is insufficient. The pointer-transition case pauses only after the runtime has cloned its revision; the pause acknowledgement must be visible before the serving pointer changes.

The queued-withdrawal barrier pauses after queue admission but before dispatch commitment. Dispatch commitment is the synchronized final active-revision and authorization decision plus connector handoff. Withdrawal that is acknowledged first must produce zero connector entries. A commitment acknowledged first may complete. Network transmission time is not the linearization point, and the protocol does not claim cancellation after commitment.

## Invariants and required scenario registry

`scenarios/architecture-conformance.json` is the frozen scenario registry. It defines the scenario IDs, invariant mapping, assertions, reset requirements, and repetition count. The following scenarios are required:

| Scenario | Invariant evidence | Repetitions |
| --- | --- | ---: |
| `AC-01-happy-path` | External ERPNext transformation, reviewed activation, discovery, allowed invocation, credential reference, read/list/create/read/delete cleanup | 10 |
| `AC-02-erp-baseline` | Unchanged ERP source, image, dependencies, HTTP contract, authentication interface, and deployment | 10 |
| `AC-03-admission-tamper` | Rejection before exposure after a bound-resource mutation | 10 |
| `AC-04-reconcile-restart` | Reconciliation interruption, restart, measured recovery, rediscovery, and subsequent ERPNext call | 10 |
| `AC-04B-reconcile-no-restart` | At least one full failed reconciliation interval, dependency recovery without restart, rediscovery, and subsequent ERPNext call | 10 |
| `AC-05-withdraw-convergence` | Withdrawal removal, no post-withdrawal commitment, and non-reappearance | 10 |
| `AC-06-authz-denied` | Fixed denied principal and zero ERP dispatch | 10 |
| `AC-07-queued-withdrawal` | Final active-resource check and zero dispatch after withdrawal | 10 |
| `AC-08-serving-transition` | Acknowledged paused v1 call remains v1 while later unbound calls select serving v2 | 10 |
| `AC-09-bound-active-revision` | Distinguishable v1 executes by qualified name or digest while active non-serving v2 remains serving | 10 |
| `AC-10-stale-or-inactive-revision` | Stale, unknown, or inactive exact revision rejection without fall-forward | 10 |
| `AC-11-secret-artifact-exclusion` | Manifest credential reference and exclusion of the Git-ignored `.env` from the artifact | 1 |
| `AC-12-effective-origin` | Approved effective origin succeeds; rewritten or redirected unapproved origin is rejected before connector entry | 10 |

Each dynamic repetition begins by restoring synthetic ERPNext fixtures and clearing ERPBridge desired, runtime, and cache state. A missing or duplicated repetition blocks the release.

## Reconciliation fault schedule

Before freezing the campaign, record the implementation's actual periodic-rescan interval or retry schedule and choose a deadline that includes at least one complete failed interval plus a bounded recovery window. The restart case restores the dependency after restart. The no-restart case first observes a complete failed interval, then restores the dependency while the same process remains alive. Both cases record cycle-local attempt timestamps, failures, restart count, elapsed convergence, repaired discovery, and a subsequent real MCP-to-ERPNext invocation. These measurements may live in traces rather than durable control-plane state. Desired state must remain stable, and withdrawn tombstones must not resurrect through the frozen observation deadline.

## Freeze procedure

After implementation and runner tests pass, populate `release-freeze.md` with the ERPBridge source and image, schema version, ERPNext image digest, MCP SDK version, contract, oracle, profile, scenario registry, reset revision, and runner version. The release gate compares every final run to these frozen inputs.

Changing any frozen input, scenario definition, reset procedure, or local-authentication mode invalidates the freeze. Recreate the affected ten-run set under a new freeze. Do not change source, images, schemas, inputs, or the runner while collecting a final set.

## Run records and assertions

Every record must validate against `run-metadata.schema.json` before it is written. A complete record identifies its invariant IDs, scenario, repetition, frozen inputs, clean reset, immutable resource state, fixed principal class, final authority decision, effective origin, dispatch count, state assertions, and artifact paths. Write artifacts before metadata and never overwrite a run directory.

Required artifacts are redacted structural traces, not raw ERP bodies or credentials. The only secret-indirection check is that the reviewed resource holds an opaque credential reference, a local `.env`-backed invocation succeeds, and no `.env` file enters the published artifact.

## Release decision

The release gate passes only when both dimensions pass: architecture coverage is complete and release integrity is verified. This requires matching frozen inputs, exact scenario repetitions, passing assertions, complete in-archive artifacts, all required invariant witnesses, and exclusion of every `.env` file. The gate fails closed on `partial`, `untested`, `unsupported`, missing, or malformed evidence. An `unknown` historical source-to-run link blocks release but does not retroactively classify an invariant as failed.
