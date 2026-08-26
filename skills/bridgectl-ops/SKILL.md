---
name: bridgectl-ops
description: "Use this skill when operating ERPBridge with bridgectl: onboard ERP APIs, manage MCP tools or external plugins and plugin bindings, administer tokens or roles, inspect caches or logs, troubleshoot runtime or SDK integrations, or prepare a redacted bug report. Reach for it even when the user asks indirectly about plugin endpoints, post-response transformations, plugin authentication, or plugin lifecycle."
license: MIT
compatibility: Requires a current bridgectl binary, an authorized ERPBridge context, and network access to the selected ERPBridge server and ERP endpoint. Git is required for repository diagnosis; GitHub CLI is optional for approved issue creation.
metadata:
  author: ERPBridge Team
  version: 3.1.0
---

# Bridgectl Operations

Use this skill to operate the ERPBridge ecosystem through `bridgectl` and its
repository. Keep the work **evidence-led**: inspect the selected environment,
perform the smallest safe action, verify the result, and report only redacted
facts.

## Establish the operating context

1. Record `bridgectl version`, `bridgectl context list`, and the chosen
   `--context` value. Prefer `--context` over changing the saved active
   context.
2. Identify the required credential and scope before contacting the server.
   `--token`, `BRIDGE_API_TOKEN`, and the context token are resolved in that
   order. Keep token values out of command output, files, and summaries.
3. When working from the repository, record `git rev-parse --short HEAD` and
   preserve unrelated working-tree changes.
4. Read [ecosystem context](references/ecosystem.md) when the task crosses
   the server, MCP client, SDK, source repository, or published docs.

## Select the workflow

| Need | Read |
| --- | --- |
| Register an ERP endpoint and publish an MCP tool | [Onboarding](references/onboarding.md) |
| Inspect, update, retire, or secure APIs and tools | [Operations](references/operations.md) |
| Manage tokens, scopes, or guarded tool roles | [Operations](references/operations.md) |
| Manage external plugins, plugin bindings, plugin auth, or post-response processing | [Plugins](references/plugins.md) |
| Investigate errors, cache behavior, logs, or an SDK integration | [Diagnostics](references/diagnostics.md) |
| Produce a maintainer-ready defect report | [Diagnostics](references/diagnostics.md) and [report template](assets/bug-report.md) |
| Review fields in a tool manifest | [MCP tool template](assets/mcp-tool.yaml) |

## Change gates

Explain the target, environment, and expected effect before any command that
registers an API, applies or deletes a tool, applies or deletes a plugin or
plugin binding, creates or revokes a token, or flushes cache entries. Obtain
explicit confirmation for that exact action. For a hard delete or an all-cache
flush, repeat the target and impact in the confirmation. For a plugin change,
include the exact plugin version or binding name, endpoint/tool reference, and
expected affected-tool cache impact.

Use environment references for downstream ERP and plugin credentials. Tool
manifests use `credentialRef`; plugin manifests use a `PLUGIN_*`
`credentialRef`; neither contains a credential value. Show a newly created API
token only to its intended recipient once, then omit it from all subsequent
output.

## Verify and hand off

After a workflow, verify the observable result at its highest available seam:
the API test, local schema validation, registry readback, plugin or binding
readback, MCP discovery/call, or an affected runtime metric/log. State the
context, versions, commands run, result, and remaining risk. Redact secrets,
authorization headers, plugin payloads and response bodies, and personally
identifying ERP data.
