# ERPBridge Plan Index

This directory is organized by execution state. Read this index before
starting implementation work.

## Active
- [`Plan-Release-v0.5.0-alpha.1.md`](./active/Plan-Release-v0.5.0-alpha.1.md) — candidate validation, release-note cut, and approved prerelease publication.

## Upcoming
- [`Plan-MCP-Upgrade.md`](./upcoming/Plan-MCP-Upgrade.md) — deferred MCP
  specification evaluation and migration. It is not authorization to begin
  implementation.

Reference evidence for the SDK integration plan:

- [`rca-sdk-integration-testing.md`](./upcoming/rca-sdk-integration-testing.md)

Upcoming plans must first be refreshed against current dependencies, protocol
versions, compatibility requirements, and operational constraints before
promotion to `active/`.

## Completed

- [`[COMPLETED]Plan-Console-Topology-and-Pagination.md`](./completed/%5BCOMPLETED%5DPlan-Console-Topology-and-Pagination.md) — context-correct topology, compact React Flow redesign, and operational-table pagination.
- [`[COMPLETED]Plan-Agentic-Tools-MCP-Integration.md`](./completed/%5BCOMPLETED%5DPlan-Agentic-Tools-MCP-Integration.md)
  — backward-compatible Stdio hardening, agent interoperability verification,
  and Codex/OpenCode/OpenClaw/Hermes documentation.
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
- [`[COMPLETED]Plan-Mock-ERP.md`](./completed/[COMPLETED]Plan-Mock-ERP.md)
  — standalone MockERP extraction, pinned image consumption, and the
  ERPNext-aligned SQLite service.
- [`[COMPLETED]Plan-Generic-External-Plugins.md`](./completed/[COMPLETED]Plan-Generic-External-Plugins.md)
  — generic external-plugin control plane, response processing, CLI, and
  black-box integration fixture.
- [`[COMPLETED]Plan-Raw-Response-Plugin-Phase.md`](./completed/%5BCOMPLETED%5DPlan-Raw-Response-Plugin-Phase.md)
  — media-aware pre-normalization plugin processing, secure admission, and
  stable MCP output contracts.
- [`[COMPLETED]Plan-Bridgectl-Ops-Plugin-Details.md`](./completed/%5BCOMPLETED%5DPlan-Bridgectl-Ops-Plugin-Details.md)
  — external plugin operations, secure lifecycle guidance, and trigger
  evaluations.
- [`[COMPLETED]Plan-Plugin-Endpoint-Authentication.md`](./completed/[COMPLETED]Plan-Plugin-Endpoint-Authentication.md)
  — plugin authentication, secure credential transport, redaction, runtime
  invariants, fixture authentication, and legacy ERP credential migration.
- [`[COMPLETED]Plan-Onboarding-Reliability.md`](./completed/%5BCOMPLETED%5DPlan-Onboarding-Reliability.md)
  — reliable Compose onboarding, context-scoped registries, structured errors,
  reloadable Console state, security gates, agent guidance, and end-to-end docs.
- [`[COMPLETED]Plan-Hot-Credential-Update.md`](./completed/%5BCOMPLETED%5DPlan-Hot-Credential-Update.md)
  — environment-default credentials with optional mounted-file rotation.
- [`[COMPLETED]Plan-Erpbridge-Infra.md`](./completed/%5BCOMPLETED%5DPlan-Erpbridge-Infra.md)
  — minimal native-HCL Azure Container Apps demo deployment.

Completed plans are historical records. Do not execute their unchecked task
lists as new work.

## Stalled and superseded

- [`[STALLED]Plan-SDK-Integration-Testing.md`](./stalled/[STALLED]Plan-SDK-Integration-Testing.md)
  — incomplete authenticated SDK/ERPBridge integration remediation plan,
  archived before implementation completed.
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
