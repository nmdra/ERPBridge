# Plan: Preserve input schema constraints

## Status

Completed source implementation and regression verification on `fix/schema-constraint-preservation`. The user approved source commits, not a live rollout. The retained typed-enum regression was restored to `internal/idp/schema_constraints_test.go`. Direct API invocation now reuses the existing input validator before middleware and ERP execution, matching MCP invocation.

ERPNext now exposes 45 unique names and 46 READY revisions, including five later user/document read tools with explicit empty-string legacy `cmd` guards. New master/order writes, financial reports, asset-master CRUD, and document assignment/unassignment writes remain deferred. Preserve the user's registered API base URL ending in `/api` and the operator-added `get_company@1.0.1` revision. See the infra checkout's `.agents/plans/pending-erpnext-expansion/README.md` for saved scope and read-verification limitations.

## Goal

Preserve JSON enum value types and explicit `additionalProperties` constraints from OpenAPI generation through stored tool definitions, MCP discovery, and argument validation. The user approved this fix after reviewing the failed draft-status/report constraints.

## Current State

- `internal/idp/generator.go:528-532` stringifies all enum values, so integer `[0]` becomes `["0"]` and boolean `[true]` becomes `["true"]`.
- `internal/mcp/tool.go:128-144` models enum values as strings and has no additional-properties field at the root or nested property level.
- Object request bodies are flattened into the MCP input object at `internal/idp/generator.go:303-330`; their explicit closure must move to that input object, while path/query fields remain declared.
- `internal/mcp/server.go:783-801` validates arguments using the selected revision's MCP schema before middleware. Preserve that path rather than add a second validator.
- `internal/web/observability_api.go:245-253` exposes enum display labels through a string-only Console projection. Keep that display API compatible without changing executable enum types.

## Decisions

- Store enums as JSON values, not formatted strings. Existing JSON string enums keep their wire representation.
- Add optional additional-properties metadata at both schema levels. Explicit false must survive; omission retains permissive legacy behavior. Preserve schema-valued additional-properties constraints when present in the resolved OpenAPI schema.
- Retain the current validator, role-selector injection, credential references, revision handling, and execution mapping.
- Test public generation, manifest round trips, MCP discovery/calls, and direct invocation with local test fixtures only. Invalid input must not reach ERP execution, and errors must not echo input values.
- The user authorizes one atomic source-fix commit on a new branch. Typed enums and object closure form one schema-contract repair; implementation and documentation are verification checkpoints within that commit. Synchronize the public tool-schema guide in a separate documentation-repository commit if available. No dependency changes, installed CLI replacement, registry modifications, credential rotation, image publication, or live deployment are authorized.

## Atomic task

- [x] Repair generated schema constraints, complete the checkpoints below, review the complete diff, and commit the cohesive source fix with tests and documentation.

## Implementation and verification checkpoints

- [x] **Task 1: Preserve typed enum values.** Add a failing public generator serialization regression, then change the property model and generator. Adapt role enum construction and existing string-enum expectations; keep Console enum display labels compatible. **Files:** `internal/idp/generator.go`, `internal/idp/schema_constraints_test.go`, `internal/mcp/tool.go`, `internal/mcp/server.go`, existing enum assertions in adjacent tests, `internal/web/observability_api.go`. **Seam:** `Generator.GenerateFromOpenAPI` and JSON tool serialization. **Verify:** `go test ./internal/idp ./internal/mcp ./internal/web -run 'TypedEnum|Enum|SchemaForMCP|RegisterToolMarshaling|OpenAPI' -count=1`.
- [x] **Task 2: Preserve and enforce object closure.** First add a failing generator regression for closed bodies, nested objects and array objects. Preserve resolved additional-properties metadata and transfer object-body metadata to the flat input schema. Add local integration coverage for valid inputs, invalid typed enums, missing required fields, unknown root/nested/array fields, company constraints, string enums, guarded roles, and permissive legacy schemas. **Files:** model/generator above and a new adjacent schema integration test. **Seam:** generation -> stored tool -> MCP discovery/call and direct invocation. **Verify:** `go test ./internal/idp ./internal/mcp -run 'SchemaConstraint|TypedEnum|AdditionalProperties|ClosedInput' -count=1`.
- [x] **Task 3: Document and validate.** Update developer guidance and Unreleased notes. Synchronize the matching public tool-schema guidance if the public checkout is available, preserving unrelated work. Run focused race checks, bounded lint on touched Go packages, `make test`, and whitespace checks. **Files:** `docs/tool-schema.md`, `CHANGELOG.md`, `.agents/plans/README.md`, matching public documentation. **Seam:** documented schema contract and repository quality gates. **Verify:** `go test -race ./internal/idp ./internal/mcp -run 'SchemaConstraint|TypedEnum|AdditionalProperties|ClosedInput' -count=1`, `make test`, `golangci-lint run ./internal/idp/... ./internal/mcp/... ./internal/web/...`, and `git diff --check`.

## Verification results

- Both focused Go test commands passed after observed red/green regressions.
- `make test` passed for all packages.
- Focused race tests passed.
- Bounded `golangci-lint` on idp, mcp, and web passed after fixing one unused test parameter.
- Independent Standards and Spec reviews found no blocking issues.
- Public documentation build and generated AI-readiness artifacts passed.
- No live calls, dependency changes, credential rotation, CLI replacement, image publication, or deployment were performed.

## Acceptance

- Integer 0 and boolean true retain their types in generated/stored/discovered schemas.
- Explicitly closed bodies and nested objects reject extra fields before ERP execution through both invocation transports.
- Fixed company/report enums and required fields remain enforced.
- Existing omitted closure policies remain permissive, and role selection still works with a closed root object.
- The deployed ERPBridge, its 45 tool names and 46 revisions, user registry `/api` base, and client tokens remain unchanged. A live rollout requires separate approval and a state/token preservation plan.
