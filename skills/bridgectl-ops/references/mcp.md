# Use bridgectl management MCP

Use this reference for management tool calls or agent configuration. For
published ERP business tools, use runtime MCP instead. These are separate
connections and can need different credentials and permissions.

## Discover and inspect

1. Identify the configured management connection. Do not assume a namespace
   such as `bridgectl-controller`; names are chosen by the client operator.
2. Discover its live tools and read the input schema for each needed tool.
   `bridgectl mcp --stdio --context <context>` defaults to inspection, local
   validation, and generation. An absent write tool is not a transport error.
3. Call `bridgectl_context_list` and check which context is pinned. Record the
   server version from MCP initialization when available, or CLI version when
   local shell access is available. Mark unavailable version evidence unknown.
4. Use one relevant read, such as `bridgectl_tool_get`, to check authenticated
   deployment access. Context listing and API listing are local operations;
   they do not prove remote authentication or ERP connectivity.

One management operation runs at a time, with a 30-second operation deadline.
Serialize calls to the same management process, even when they are independent.
A `busy` result means the concurrent call was refused before child execution.
Wait for the in-flight call to finish, then retry only that refused call.

## Read results

Each call returns an MCP result envelope. Its text content contains a JSON
object with `schemaVersion`, `operation`, `context`, `success`, and either
`data` or `error`. Preserve the envelope and inspect `isError`; then parse the
JSON and inspect `success` and `error.code`. A resolved tool promise is not
proof of success. Use stable codes, not message-text matching.

For example, after discovering the host's actual namespace, a Pi codemode
script can make two sequential reads. Replace `mcp__management` with that
namespace before execution:

```javascript
const contextResult = await tools.mcp__management__bridgectl_context_list({});
const contextBody = JSON.parse(contextResult.content.find(
  item => item.type === "text"
).text);
if (contextResult.isError || !contextBody.success) {
  throw new Error(contextBody.error?.code ?? "invalid_result");
}
text({ context: contextBody.context });

const inventoryResult = await tools.mcp__management__bridgectl_tool_get({});
const inventoryBody = JSON.parse(inventoryResult.content.find(
  item => item.type === "text"
).text);
if (inventoryResult.isError || !inventoryBody.success) {
  throw new Error(inventoryBody.error?.code ?? "invalid_result");
}
text({ toolCount: inventoryBody.data.items.length });
```

An inventory returns `data.items`, including `[]` for an empty collection.
A named get returns `data.item`. Inspection views are marked `redacted: true`
and omit credential fields, literal authentication header fields, raw endpoint
URLs, plugin configuration, schema defaults/examples, and log messages.
Allow-listed descriptions, usage text, schema declarations, and enum values can
remain visible. `redacted: true` is a projection marker, not proof that arbitrary
text is secret-free: review retained values before sharing or persistence.
Logical credential references can remain visible. **Do not apply an inspection
view as a manifest.** Generate a draft with `bridgectl_tool_generate` or use the
reviewed source manifest, then validate it. Generation intentionally returns
an executable manifest; review it before sharing or applying it.

## Configure a client

Start with an absolute executable path, explicit context, and no write grants:

```sh
/absolute/path/to/bridgectl mcp --stdio --context staging
```

A Pi user-level `~/.pi/agent/mcp.json` entry can use:

```json
{
  "mcpServers": {
    "bridgectl-controller": {
      "command": "/absolute/path/to/bridgectl",
      "args": ["mcp", "--stdio", "--context", "staging"],
      "exposure": "codemode",
      "description": "ERPBridge management operations; write groups disabled"
    }
  }
}
```

Merge the entry into the existing object after approval. Keep runtime MCP and
other servers intact. Keep token values out of JSON and command arguments.
Use the authorized saved context or the host's secret-backed environment;
`BRIDGE_API_TOKEN` overrides the context token. Check presence/source, never
print the resolved value. Do not copy a runtime-scoped token into a management
session without checking its intended scope.

In Pi, use `pi mcp list` to verify connections. Run `/reload` after changing
configuration. Reconnect or restart the management process to adopt changed
credentials, endpoints, or grants; changing only a saved value does not refresh
a running process. Stdout is exclusively the MCP stream; diagnostics use stderr.

## Approved changes

Before a mutation or server-side probe, complete the relevant preflight,
validate any manifest, and obtain exact-action approval under `SKILL.md`.
The operator must separately approve the required launch-time group:

| Group | Capability |
| --- | --- |
| `registry` | Register APIs or change credential references |
| `resources` | Apply tool, plugin, or binding manifests |
| `destructive` | Delete/deactivate resources, revoke tokens, or flush cache |
| `probe` | Run server-side API tests that can cause downstream effects |
| `context` | Change the saved default context |

`--allow-write` accepts named groups, repeated or comma-separated, not a
wildcard. Forced API replacement additionally needs `destructive`. Obtain
approval before changing these launch arguments. A local group does not grant
remote authorization. For each approved call, read its schema and supply
`confirmContext` equal to the pinned context and `confirmTarget` equal to the
canonical resource target. These fields prevent mismatches, not prove consent.
Canonical targets include `name@version` for versioned deletes, the binding
name for bindings, and `tool:<name>`, `module:<name>`, or `all` for cache flush.
Use the discovered schema and matching release documentation for other targets.

Manifests are one inline JSON object, not a file path, URL, array, or YAML
stream. MCP tool argument schemas reject unknown arguments; manifest validation
is resource-specific and is not strict unknown-field rejection for every
resource. Credential screening rejects recognized literal credential fields and
unsafe endpoints, not every possible secret in free text. Remove unsupported
fields, keep values secret-free, and validate the exact reviewed manifest.
Remote OpenAPI fetch, filesystem output, and token creation remain manual CLI
operations. For exact validation diagnostics, use local CLI validation and
sanitize its output before sharing.

## Verify and recover

| Result | Next action |
| --- | --- |
| `authentication_required` | Check the pinned bridge credential's source and intended scope without showing its value. After an authorized correction, restart/reconnect the process and retry one read. Do not substitute ERP credentials or escalate identity. |
| `context_changed` | Stop. Confirm the intended context and restart the process to adopt an approved change. Never fall back to another context. CLI child code is `MCP_CONTEXT_CHANGED`. |
| `not_found` | Check the exact context, name, and version; list before changing state. |
| `invalid_arguments` | Correct the input against the discovered schema. For manifest details, run local CLI validation. |
| `busy` | Wait for the active call, then retry the refused call serially. |
| `probe_failed` | Preserve status/type/timing evidence. A completed failed probe is an observation, not an unknown write. |
| `outcome_unknown` | The write may already have happened. Read the exact target before deciding whether to retry. Do not assume rollback or retry automatically. |

Most resource writes report `target`, `effect: "command_completed"`, and
`verified: false`. Cache flush instead reports `target`, `deleted`, optional
`status`, and `verified`; context selection reports `savedDefault`,
`activeContext`, and `verified`. None establishes independent verification.
Perform separate readback of the exact target and verify its expected state;
use a safe runtime
call only when needed and authorized. If readback cannot establish the result,
report it as unverified and stop rather than repeat the change.

Sessions pin context, management endpoint, runtime endpoint, and credential at
launch. Changing the saved default does not retarget a session. Recovery must
preserve the selected environment, existing approval boundaries, and redaction.
