# ToolPlane conformance mechanisms

## Status

This document describes the ToolPlane changes in the current ERPBridge source tree. It does not describe a published release.

The changes bind an admitted MCP tool revision to one runtime invocation. They also define a process-local boundary between withdrawal and ERP connector entry.

## Scope

The implementation adds these mechanisms:

1. An immutable admission digest for each tool name and version.
2. A separate serving pointer for unqualified MCP calls.
3. Exact revision names and digest-bound MCP calls.
4. One cloned tool resource for all stages of one invocation.
5. A synchronized boundary between withdrawal and connector entry.
6. Effective ERP origin validation before connector entry.
7. Process-local reconciliation status and no-restart recovery.

The implementation does not provide distributed revocation. It does not provide final-authority checks for protected cache hits.

## Admission binding

`POST /apis/erpbridge.io/v1/tools` is the admission operation. The authenticated caller is the reviewing actor.

ERPBridge validates the resource and calculates a SHA-256 digest. The canonicalization identifier is `erpbridge-tool-json-v1`.

The digest includes executable fields:

- API version and resource kind.
- Tool name, version, module, and description.
- Input schema, output schema, and annotations.
- Authorization, cache, rate, concurrency, and routing policy.
- HTTP method, endpoint, approved origins, argument mapping, and response mapping.
- Credential reference and lifecycle configuration.

The digest excludes runtime and review fields:

- `metadata.status`.
- `metadata.isActive`.
- `metadata.isServing`.
- `metadata.resourceDigest`.
- `metadata.admission`.
- Native Go handlers.

The server writes these fields after admission:

```json
{
  "resourceDigest": "<sha256>",
  "admission": {
    "actor": "<authenticated-principal>",
    "approvedAt": "<RFC3339 timestamp>",
    "decision": "approved",
    "canonicalization": "erpbridge-tool-json-v1"
  }
}
```

An identical apply is idempotent. Changed executable content with the same name and version returns HTTP `409` and `REGISTRY_CONFLICT`.

Create a new semantic version when executable content changes. ERPBridge does not replace an admitted revision.

## Activation and serving selection

Activation and serving selection are separate states.

- `metadata.isActive: true` makes an exact revision eligible for execution.
- `metadata.isServing: true` selects the revision for future unqualified calls.
- The first active version of a tool becomes serving when no serving version exists.
- A later active version remains non-serving unless the apply request selects it.
- Soft deletion removes activation and serving state for that revision.

The store persists serving state in the tool resource. `ToolRegistry.SetServing` keeps one in-memory serving revision for each base name.

An unqualified call resolves only the serving revision. It does not select the highest semantic version.

## Exact revision discovery and calls

ERPBridge registers every active revision with a protocol-safe exact name:

```text
<tool-name>.rev_<base64url-semver>
```

The unqualified serving tool and each exact tool include these MCP `_meta` fields:

```json
{
  "toolplane.resourceDigest": "<sha256>",
  "toolplane.resourceVersion": "1.2.0",
  "toolplane.serving": true
}
```

A client can bind an unqualified `tools/call` request to a discovered digest:

```json
{
  "name": "list_employees",
  "arguments": {},
  "_meta": {
    "toolplane.resourceDigest": "<sha256>"
  }
}
```

An exact-name or digest-bound request executes that active revision. An unknown, stale, or inactive revision fails and requests rediscovery. ERPBridge does not fall forward to another revision.

The direct compatibility endpoint accepts `name@version` for an exact version.

## Per-call resource snapshot

`ToolRegistry.Add` stores a clone of the tool resource. `ToolRegistry.Resolve` returns a new clone.

The MCP handler resolves one serving, exact-name, or digest-bound revision before it constructs middleware. ERPBridge validates arguments against that selected clone rather than against a mutable unqualified alias in the MCP SDK. The same clone controls:

- Argument handling and schema validation.
- Role authorization.
- Rate and concurrency policy.
- Revision-scoped cache policy and keys.
- Credential resolution.
- ERP request mapping.
- Response processing.
- Output validation.

A concurrent serving-pointer change does not change an in-flight invocation.

## Withdrawal and dispatch commitment

`Server.authorityMu` serializes withdrawal and ERP dispatch commitment in one process.

The connector wrapper performs these actions:

1. It constructs the final ERP request.
2. It validates the effective origin.
3. It acknowledges the `before_commit` barrier when a test hook exists.
4. It obtains the authority read lock.
5. It resolves the exact selected version again.
6. It compares the active digest with the snapshot digest.
7. It records `dispatch_committed`.
8. It enters the ERP connector.

`DeregisterTool` obtains the authority write lock before it removes activation. It records `withdrawal_committed` while it owns this boundary.

The ordering rules are:

- If withdrawal obtains the write lock first, the call records `commit_denied` and makes no connector entry.
- If dispatch obtains the read lock first, the call can complete.
- Withdrawal acknowledgment waits until that committed connector call returns.

The implementation holds the read lock for the connector call. This is a conservative process-local design. It does not cancel an ERP request after commitment.

Authority events contain only sequence, event type, tool name, tool version, digest, and time. They do not contain arguments, headers, credentials, paths, or ERP data. The sequence is assigned at commitment. Observer callbacks run after the authority lock is released, so a callback can safely perform a follow-up withdrawal; consumers must use the sequence field as the ordering authority.

## Effective origin binding

`spec.execution.approvedOrigins` contains normalized origins in this form:

```text
scheme://host[:port]
```

If `approvedOrigins` is absent, admission derives one origin from `endpoint` and the current `ERP_BASE_URL`.

Before connector entry, ERPBridge calculates the final origin again. The calculation includes these rules:

- `ERP_BASE_URL` prefixes a relative endpoint.
- `ERP_BASE_URL` replaces the origin of a local absolute endpoint.
- The default origin for a relative endpoint is `http://localhost:8081` when `ERP_BASE_URL` is absent.

The connector wrapper rejects a final origin that is not in `approvedOrigins`. The rejection occurs before the underlying connector call.

The authority wrapper forces redirect suppression at the production HTTP connector boundary, including uncredentialed requests. Thus, a redirect response cannot move execution to another origin. Custom connectors must either implement `ERPResponseConnector` and honor `DisableRedirects` or provide equivalent no-redirect behavior.

## Reconciliation status and recovery

The controller still runs immediately and every 10 seconds. A failed interval does not stop later intervals.

`GET /api/info` includes this process-local status:

```json
{
  "reconciliation": {
    "attemptCount": 3,
    "lastAttempt": "<timestamp>",
    "lastSuccess": "<timestamp>",
    "lastError": "",
    "desiredStateHash": "<sha256>",
    "observedGeneration": 2,
    "converged": true
  }
}
```

SQLite stores the desired resources and soft-delete tombstones. Attempt counters and timestamps are process-local. They reset after a restart.

A focused test injects a dependency failure for more than one complete interval. The same process then recovers, discovers the tool, and completes an ERP call.

## Test and evaluation hooks

`AuthorityHooks` supports deterministic ordering tests:

- `BeforeCommit` provides an acknowledged barrier before the authority lock.
- `OnEvent` receives sanitized ordered authority events.

`ReconciliationHooks.BeforeAttempt` supports deterministic dependency failures in reconciliation tests.

Production code leaves these hook fields unset. The hooks do not add HTTP administration endpoints.

## Errors

The control plane returns stable errors for admission conflicts. Runtime failures use the existing safe MCP error result.

| Condition | Result |
| --- | --- |
| Changed same-version content | HTTP `409`, `REGISTRY_CONFLICT` |
| Unknown or inactive exact revision | Rediscovery-required protocol error |
| Withdrawal before commitment | MCP conflict error, zero connector entries |
| Unapproved effective origin | MCP permission-denied error, zero connector entries |

## Migration behavior

At store initialization, ERPBridge migrates legacy tools that lack admission fields. It binds approved origins using the current endpoint rewrite rules, calculates the canonical digest, and records the actor as `legacy-migration`. For a legacy tool with no serving flag, migration persists the latest stable semantic version, or the latest prerelease when no stable revision exists. This preserves the previous unqualified resolver behavior.

Migration does not create a fallback for already-admitted tools whose serving revision was withdrawn. Their active exact revisions remain callable, while the unqualified name remains unavailable.

Built-in native tools do not use the HTTP connector boundary. The new withdrawal and origin rules apply to declarative ERP HTTP tools.

## Known limits

- The authority lock is process-local. Multiple ERPBridge processes do not share it.
- Protected cache hits do not use the connector commitment boundary.
- The implementation does not cancel or roll back a committed ERP operation.
- Reconciliation counters and timestamps are not durable.
- The origin check does not replace DNS, network, TLS, or ERP authorization controls.
- The conformance campaign must still collect live ERPNext evidence.

## Source map

| Mechanism | Source | Focused tests |
| --- | --- | --- |
| Canonical admission | `internal/mcp/admission.go`, `internal/mcp/store.go` | `internal/mcp/admission_test.go` |
| Serving and exact revisions | `internal/mcp/registry.go`, `internal/mcp/server.go` | `internal/mcp/registry_test.go`, `internal/mcp/revision_binding_test.go` |
| Dispatch authority and origin | `internal/mcp/authority.go`, `internal/mcp/plugin_pipeline.go` | `internal/mcp/authority_test.go` |
| Reconciliation status | `internal/mcp/reconciliation_status.go`, `internal/mcp/info_api.go` | `internal/mcp/reconciliation_status_test.go` |
| Resource fields and request preparation | `internal/mcp/tool.go` | `internal/mcp/tool_test.go` |

## Verification commands

```sh
go test -race ./internal/mcp -run 'Admission|Canonical|Conflict|Serving|Qualified|Digest|Snapshot|Transition' -count=1
go test -race ./internal/mcp -run 'FinalAuthority|Barrier|Withdrawal|Origin|ZeroDispatch|DispatchCommit' -count=1
go test -race ./internal/mcp -run 'Reconcile|Recovery|Restart|Tombstone|NoResurrection' -count=1
go test -race ./internal/mcp ./internal/connector -count=1
```
