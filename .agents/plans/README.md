# ERPBridge Plan Index

This directory is organized by execution state. Read this index before
starting implementation work.

## Active

- [`Plan-Mock-ERP.md`](./active/Plan-Mock-ERP.md) — approved execution plan
  for extracting Mock ERP to `nmdra/mockerp`, publishing a pinned GHCR image,
  and implementing the ERPNext-aligned SQLite service.
- [`Plan-Agentic-Tools-MCP-Integration.md`](./active/Plan-Agentic-Tools-MCP-Integration.md)
  — approved execution plan for backward-compatible Stdio hardening, agent
  interoperability verification, and Codex/OpenCode/OpenClaw/Hermes
  documentation.
- [`Plan-SDK-Integration-Testing.md`](./active/Plan-SDK-Integration-Testing.md)
  — approved SDK/ERPBridge integration remediation plan.

## Upcoming

- [`Plan-Generic-External-Plugins.md`](./upcoming/Plan-Generic-External-Plugins.md)
  — proposed generic external-plugin control-plane plan. It is not execution
  authorization.
- [`Plan-MCP-Upgrade.md`](./upcoming/Plan-MCP-Upgrade.md) — deferred MCP
  specification evaluation and migration. It is not authorization to begin
  implementation.

Reference evidence for the SDK integration plan:

- [`rca-sdk-integration-testing.md`](./upcoming/rca-sdk-integration-testing.md)

Upcoming plans must first be refreshed against current dependencies, protocol
versions, compatibility requirements, and operational constraints before
promotion to `active/`.

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
