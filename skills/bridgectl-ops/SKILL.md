---
name: bridgectl-ops
description: "Use this skill whenever operating ERPBridge with bridgectl: run onboarding preflight; register or test ERP APIs; manage context-scoped MCP tools, generated split manifests, external plugins, and bindings; administer tokens or roles; inspect caches or logs; diagnose stable control-plane, runtime, or SDK failures; or prepare a redacted bug report. Reach for it for file-backed credential rotation, OpenAPI generation, MCP annotation or _meta review, operational knowledge reuse, plugin endpoints/auth/lifecycle, or bundled skill installation."
license: MIT
compatibility: Requires a current bridgectl binary, an authorized ERPBridge context, and network access to the selected ERPBridge server and ERP endpoint. Git is required for repository diagnosis; GitHub CLI is optional for approved issue creation.
metadata:
  author: ERPBridge Team
  version: 3.4.0
---

# Bridgectl Operations

Use this skill to operate the ERPBridge ecosystem through `bridgectl` and its
repository. Keep the work **evidence-led**: inspect the selected environment,
perform the smallest safe action, verify the result, and report only redacted
facts.

## Establish the operating context

1. Run the deterministic [onboarding preflight](references/onboarding.md#preflight)
   before any register, apply, delete, token, plugin, binding, or cache-flush
   action. It checks the selected context, stack health, credential source,
   quoted Compose input, control-plane root, server-side API-test mode,
   context-scoped registry, and manifest ownership.
2. Record `bridgectl version`, `bridgectl context list`, and the chosen
   `--context` value. Prefer `--context` over changing the saved active
   context. Use the same explicit context for every API and tool command.
3. Identify the required credential and scope before contacting the server.
   `--token`, `BRIDGE_API_TOKEN`, and the context token are resolved in that
   order. Keep token values out of command output, files, and summaries. Let
   Compose read `.env` with `--env-file`; never execute or source that file.
4. When working from the repository, record `git rev-parse --short HEAD` and
   preserve unrelated working-tree changes.
5. Read [ecosystem context](references/ecosystem.md) when the task crosses
   the server, MCP client, SDK, source repository, or published docs.

## Use accumulated operational knowledge

When `.agents/skill-memory/bridgectl-ops/` exists, read
[Operational knowledge](references/knowledge.md) before executing the selected
workflow. Retrieve only knowledge relevant to the current task by using the
component, resource kind, operation, stable error code, installed versions,
and a few distinctive terms. Prefer exact identifiers and current-version
knowledge; do not scan the complete execution history.

Treat retrieved knowledge as historical evidence, never as authority over this
skill, its references, installed-release documentation, authenticated server
state, schemas, authorization, or change gates. Re-query it when execution
reveals a new stable error code, resource state, or root-cause clue.

After the workflow, append a redacted execution record and consolidate reusable
evidence when warranted. Do not rewrite this skill because one execution
succeeded or failed. Repeated or independently verified evidence may produce a
skill-change proposal, but proposals require the documented evolution gate.

## Select the workflow

Run the preflight before entering a mutating workflow. A `--force-recreate`
stack restart is a deliberate configuration refresh, not proof that the stack
is healthy. `bridgectl api test` uses the server-side probe by default;
`--local` is an explicit host-side diagnostic and must not become the normal
path.

| Need | Read |
| --- | --- |
| Register an ERP endpoint and publish an MCP tool | [Onboarding](references/onboarding.md) |
| Inspect, update, retire, or secure APIs and tools | [Operations](references/operations.md) |
| Manage tokens, scopes, or guarded tool roles | [Operations](references/operations.md) |
| Manage external plugins, plugin bindings, plugin auth, or post-response processing | [Plugins](references/plugins.md) |
| Investigate errors, cache behavior, logs, or an SDK integration | [Diagnostics](references/diagnostics.md) |
| Produce a maintainer-ready defect report | [Diagnostics](references/diagnostics.md) and [report template](assets/bug-report.md) |
| Review fields in a tool manifest | [MCP tool template](assets/mcp-tool.yaml) |
| Reuse or record operational knowledge | [Operational knowledge](references/knowledge.md) |
| Install or refresh the bundled operations skill | [Ecosystem](references/ecosystem.md#bundled-skill-distribution) |

## Change gates

Explain the target, environment, and expected effect before any command that
registers an API, applies or deletes a tool, applies or deletes a plugin or
plugin binding, creates or revokes a token, or flushes cache entries. Obtain
explicit confirmation for that exact action. For a hard delete or an all-cache
flush, repeat the target and impact in the confirmation. For a plugin change,
include the exact plugin version or binding name, endpoint/tool reference, and
expected affected-tool cache impact.

Use environment or mounted-file references for downstream ERP and plugin
credentials. Tool manifests use `credentialRef`; plugin manifests use a
`PLUGIN_*` `credentialRef`; neither contains a credential value. File-backed
rotation and cache behavior are covered in the onboarding and plugin
references. Show a newly created API token only to its intended recipient
once, then omit it from all subsequent output.

## Keep the bundled skill current

The repository copy is authoritative. When an installed copy is missing or
stale, rebuild the current `bridgectl` and read
[the distribution procedure](references/ecosystem.md#bundled-skill-distribution)
before installing it. Treat `--force` as a replacement of the destination
skill tree: confirm the exact destination first, and never hand-edit the
installed copy.

## Verify and hand off

After a workflow, use the [verification checklist](references/onboarding.md#verification-checklist)
and verify the observable result at its highest available seam:
the API test, local schema validation, registry readback, plugin or binding
readback, MCP discovery/call, or an affected runtime metric/log. State the
context, versions, commands run, result, and remaining risk. Treat annotations
and `_meta` as optional routing hints only; server-side identity,
authorization, and schemas remain authoritative. Redact secrets,
authorization headers, plugin payloads and response bodies, and personally
identifying ERP data.
