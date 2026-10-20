# ToolPlane Architecture-Conformance Release Gate

## Status

Final collection is **not authorized** until every field and check below is complete. This document becomes immutable when the accountable reviewer authorizes the frozen campaign.

## Preconditions

- [ ] The scenario registry in `scenarios/architecture-conformance.json` is frozen and hashes into this record.
- [ ] ERPNext fixture reset and ERPBridge state/cache clearing succeed twice with fixed local `.env` credentials.
- [ ] ERPBridge implementation and runner tests pass.
- [ ] One dry run produces complete schema-valid artifacts without packaging `evaluation/erpnext/.env`.
- [ ] The release gate rejects an altered frozen input and an incomplete scenario set.

## Freeze procedure

1. Record the approved ERPBridge source/image and database schema.
2. Record the pinned ERPNext image, MCP SDK, contract, oracle, profile, scenario registry, reset revision, and runner version.
3. Verify that `evaluation/erpnext/.env` remains Git-ignored and is absent from the artifact inventory. Do not rotate local credentials between repetitions.
4. Copy this completed record into the evidence archive before collecting final runs. Do not overwrite it after collection starts.

## Final release record

| Field | Required value |
| --- | --- |
| Release | `toolplane-conformance-v1.0` |
| ERPBridge commit | Full Git commit SHA |
| ERPBridge image digest | Immutable OCI digest |
| Database-schema version | Migration or schema identifier |
| ERPNext image digest | Immutable OCI digest |
| MCP SDK/version | Module and resolved version |
| Contract SHA-256 | 64-character SHA-256 |
| Oracle SHA-256 | 64-character SHA-256 |
| Evaluated-deployment-profile SHA-256 | 64-character SHA-256 |
| Scenario-definition SHA-256 | 64-character SHA-256 |
| Reset revision | Immutable reset implementation identifier |
| Runner version | Immutable runner identifier |
| Freeze timestamp and approver | UTC timestamp and accountable reviewer |

## Machine-readable companion

`archive-manifest.json` contains a `freeze.fields` object with the same camel-case
identifiers as run metadata. A live launcher may write the non-secret observed
values to `runtime-identifiers.json` (or pass them with
`--runtime-identifiers`). The release gate compares these values with this
record and recomputes the contract, oracle, profile, and scenario hashes before
accepting any evidence. The runtime file is operational input and is not part
of the published archive.

## Change rule

A source, image, schema, contract, oracle, profile, scenario registry, reset, runner, or local-authentication-mode change invalidates this freeze. Recreate every affected ten-run set under a new release record. The historical RQ/pilot evidence does not satisfy this gate.
