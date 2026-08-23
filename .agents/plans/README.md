# ERPBridge Plan Index

This directory is organized by execution state. Read this index before
starting implementation work.

## Active

There are currently no active implementation plans. The `active/` section is
reserved for work that has been explicitly approved for execution.

## Upcoming

- [`Plan-MCP-Upgrade.md`](./upcoming/Plan-MCP-Upgrade.md) — deferred MCP
  specification evaluation and migration. This is the next candidate, not an
  authorization to begin implementation.

The upcoming plan must first be refreshed against the current `mcp-go`
release, deployed protocol version, compatibility requirements, and migration
risks. Create a new finalized execution plan after that review.

## Completed

- [`[COMPLETED]Plan-Issue-8.md`](./completed/[COMPLETED]Plan-Issue-8.md) — fix
  cold-start MCP invocation metric families.
- [`[COMPLETED]AuthN-AuthZ-Plan.md`](./completed/[COMPLETED]AuthN-AuthZ-Plan.md)
  — API-token authentication and per-tool authorization.
- [`[COMPLETED]Plan-main.md`](./completed/[COMPLETED]Plan-main.md) — cache,
  security, correctness, CLI, and documentation hardening.
- [`[COMPLETED]Plan-bridgectl-ops.md`](./completed/[COMPLETED]Plan-bridgectl-ops.md)
  — production-grade ERPBridge operations skill.
- [`[COMPLETED]Plan-docs.md`](./completed/[COMPLETED]Plan-docs.md) — public
  Docusaurus documentation site.
- [`[COMPLETED]Plan-Lint-Fixes.md`](./completed/[COMPLETED]Plan-Lint-Fixes.md)
  — repository lint remediation.

Completed plans are historical records. Do not execute their unchecked task
lists as new work.

## Stalled and superseded

- [`Plan-Auth.md`](./stalled/Plan-Auth.md) — superseded by the completed
  AuthN/AuthZ plan.
- [`[DRAFT]Plan-RBAC.md`](./stalled/[DRAFT]Plan-RBAC.md) — superseded by the
  completed AuthN/AuthZ plan.

These files preserve earlier decisions and review context. They are not
implementation entry points.

## Plan workflow

1. Read this index and the relevant status section.
2. For new work, create or revise a plan under `upcoming/`.
3. After approval, move the plan to `active/` and execute its tasks in order.
4. Require each task's stated verification before closing it.
5. Prefix the filename with `[COMPLETED]` and move it to `completed/` when the
   entire plan is finished.

Every behavior change must retain focused tests, an atomic Conventional Commit,
and synchronized documentation where applicable.
