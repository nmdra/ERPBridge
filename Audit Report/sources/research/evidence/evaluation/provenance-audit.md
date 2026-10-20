# Historical provenance audit

Audited on 2026-09-12. This record distinguishes facts preserved in this repository from source identities named by the manuscript. It does not infer which binary executed a historical run.

## Compared identities

| Item | Preserved identity | Verified relationship |
| --- | --- | --- |
| Paper repository inspected | `7f7e4198d24ff67226db17e418bc73b30d6345fe` | Local `HEAD` at audit time. |
| Evidence snapshot | `72622d48868bdc537816da60186cd6cb70367c58` | Present in local Git history and an ancestor of the inspected paper revision. |
| ERPBridge source named by the manuscript | `b5a951a98091d1037e94ed267479fd9707835903` | The manuscript names this revision. The retained run metadata does not bind executions to it. |
| Current sibling ERPBridge checkout | `1942a2c6f584a007be2de270d02f7078f7314e04` (session-supplied identity) | Different from the manuscript revision. It is relevant to new implementation work only and cannot establish historical behavior. Direct sibling-repository inspection was unavailable in this audit environment. |

The historical runner, boundary runner, aggregate, release gate, release-freeze record, representative happy-path metadata, and retained boundary result in the working tree were byte-identical to the files at evidence snapshot `72622d4` before Task 1 edits. The comparison used complete SHA-256 output; no abbreviated digest is treated as a verification value.

## Verified historical deficiencies

- The retained happy-path metadata labels the run `smoke`, uses repetition `1`, and records `not-collected-smoke` for the database schema, MCP SDK, and reset revision.
- `architecture_validation.py` constructs the recorded ERPBridge commit, image digests, and six ERP baseline hashes with `synthetic_sha`; these are placeholders rather than observed runtime or before-and-after measurements.
- `release-freeze.md` says collection is not authorized and retains unchecked preconditions and placeholder fields.
- `archive-manifest.json` records `freeze.status` as `not-authorized` with an empty `freeze.fields` object.
- The retained aggregate covers ten happy-path records and seven selected boundary scenarios. This is bounded architectural observation, not completion of the current frozen scenario registry or a provenance-verified release.

These defects block **release integrity**. They do not prove that an architectural invariant failed, that the named ERPBridge revision was not used, or that the recorded live calls did not occur.

## Unknown linkage

Historical run-to-binary attribution is **unknown**. Source inspection at `b5a951a` and inspection of a later sibling checkout can describe those source trees, but neither observation identifies the binary or image used by each retained run. A future release must record observed source/image identifiers in an authorized pre-run freeze and repeat them in immutable run metadata.

## Reproduction commands

```sh
git rev-parse HEAD
git merge-base --is-ancestor 72622d48868bdc537816da60186cd6cb70367c58 HEAD
git show 72622d4:research/evidence/evaluation/release-freeze.md
git show 72622d4:research/evidence/evaluation/results/task11/happy-path/ac-01-happy-path-01/metadata.json
python3 -m evaluation.runner.release_gate --conformance --json
python3 -m evaluation.runner.release_gate --json
```

Both gate modes must block the historical package. The JSON output reports architecture coverage separately from release integrity; historical attribution remains `unknown`.
