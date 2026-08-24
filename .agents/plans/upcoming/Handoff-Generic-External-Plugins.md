# Handoff: Minimal Generic External Plugin System

## Status

**Planning complete. Implementation is not started.**

The source of truth is
[`Plan-Generic-External-Plugins.md`](./Plan-Generic-External-Plugins.md).
This handoff is an execution briefing, not a substitute for that plan.

## Goal

Implement a backward-compatible generic external-plugin system. Customers run
plugins separately. ERPBridge stores only `Plugin` and `PluginBinding`
resources, calls a bound plugin after a successful normalized tool response,
and returns the transformed response through the existing MCP and direct API
contracts.

The first implementation uses a deterministic `mock-plugin`. It must not
implement OCR, PII masking, image handling, plugin deployment, queues, or
request hooks.

## Authorization Boundary

Do not start implementation until the source plan is promoted from
`upcoming/` to `active/` and execution is explicitly authorized.

When authorized, begin with Task 0:

```bash
git status --short
git switch -c feat/generic-external-plugins
git branch --show-current
git diff --cached --name-only
```

The initial worktree contains unrelated untracked files. Do not modify, stage,
or delete them.

## Fixed Design Decisions

- Use exactly two control-plane resources: versioned `Plugin` and named
  `PluginBinding`.
- ERPBridge never starts, installs, downloads, upgrades, or schedules plugin
  code. The deployment operator owns the separate process/container.
- Initial protocol: synchronous HTTP JSON `POST /v1/process` returning
  `{ "result": <JSON> }`.
- A plugin receives only protocol version, invocation ID, exact tool identity,
  normalized successful result, and binding configuration. It receives no ERP
  credential, caller token, inbound header, role identity, or original
  arguments.
- Bindings are exact-version and only support `after_response`.
- Default failure policy is `continue`; `fail` produces only a generic tool
  error. Never leak plugin endpoint, credential, payload, or response body.
- The response pipeline runs only on cache misses, inside the existing cache
  middleware. Cache the final transformed response and flush an affected tool
  cache whenever plugin/binding lifecycle changes.
- Preserve every existing behavior for tools with no active binding, including
  MCP envelopes, direct `{ "result": ... }` envelopes, authorization, cache,
  retry, and tool discovery behavior.
- Revalidate transformed output against the existing tool output schema.
- Limit plugin request and response JSON to 1 MiB; use the call context plus
  resource timeout, disable redirects, and do not retry plugin calls.

## Ordered Work

1. Create the dedicated branch and preserve the worktree.
2. Define Plugin/PluginBinding models, validation, generic request/response,
   and bounded HTTP client with red-first tests.
3. Add SQLite tables, CRUD, desired-state hash, soft deletion, and protected
   hard plugin deletion with red-first tests.
4. Add admin-only management APIs and reconciliation into an immutable active
   binding snapshot; flush affected caches on lifecycle changes.
5. Extract one shared MCP/direct invocation seam; run ordered bindings after a
   successful `Tool.Execute`; test failure/cache/schema invariants.
6. Add `bridgectl plugin ...` and `bridgectl plugin binding ...` command
   families with typed YAML/JSON handling and command tests.
7. Add a separate simple `mock-plugin`, a non-PII `Plugin Fixture` mock ERP
   endpoint/OpenAPI entry, and an isolated Compose integration target that
   verifies both MCP and direct invocation paths.
8. Update in-repo docs, generated CLI reference, changelog, and matching
   `erpbridge-docs` pages/plan/changelog. Run all quality gates and make one
   Conventional Commit for each completed source-plan task.

## Core Files and Seams

| Concern | Primary files |
| --- | --- |
| Resource models and plugin HTTP boundary | `internal/mcp/plugin.go`, `internal/mcp/plugin_client.go` |
| Persistence and migrations | `internal/mcp/store.go`, `internal/mcp/plugin_store.go` |
| Reconciliation, APIs, shared invocation | `internal/mcp/server.go`, `internal/mcp/plugin_registry.go` |
| Base result validation | `internal/mcp/tool.go` |
| Existing cache boundary | `internal/mcp/middleware.go`, `internal/cache/flush.go` |
| CLI resource management | `internal/cli/plugin.go`, `internal/cli/plugin_binding.go` |
| Integration fixtures | `mock-erp/routers/inventory.py`, `mock-plugin/`, `docker-compose.plugin-test.yml`, `internal/integration/plugin_system_test.go` |

## Required Test Order

Each code task follows red → green → refactor:

1. Add focused failing tests.
2. Run the task's specified focused command and capture failure.
3. Implement only the task's minimum behavior.
4. Run the focused command until green.
5. Run lint only on directories changed by that task.
6. Commit only that task using the stated Conventional Commit message.

Final gates:

```bash
go test ./internal/mcp ./internal/cli ./mock-plugin
golangci-lint run ./internal/mcp ./internal/cli ./mock-plugin
make test-plugin-integration
make test
git diff --check
```

Also run `npm run build` in `../erpbridge-docs` after documentation updates.

## Stop-and-Ask Conditions

Stop and ask the user before changing any fixed decision or scope item,
including:

- adding a second execution phase, request mutation, retries, or async work;
- adding OCR, PII/DLP, image/binary/XML support, plugin deployment, or secrets;
- changing cache position or any legacy MCP/direct response contract;
- binding wildcards/modules instead of exact tool versions;
- modifying, staging, or deleting the pre-existing untracked worktree files;
- any task verification failure that requires a design change rather than a
  local correction.
