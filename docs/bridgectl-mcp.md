# bridgectl MCP management server

`bridgectl mcp --stdio` serves ERPBridge management operations to an MCP
client, such as an AI coding agent.

This command is a **management** server. It manages the local API registry and
the configured ERPBridge deployment. It is not the runtime server: use
`erpbridge-server --stdio` or `<erpbridge-host>/mcp/` to call published ERP
tools.

## Start a session

```sh
bridgectl mcp --stdio
bridgectl mcp --stdio --context staging
bridgectl mcp --stdio --context staging --allow-write registry,resources
```

The command requires `--stdio` and rejects positional arguments. Start one
process per agent configuration. The process stops when its input ends or it
receives an interrupt or termination signal.

## Stream contract

Stdout carries the MCP JSON-RPC stream. Every line on stdout is one complete
MCP message.

Diagnostics and startup failures use stderr. This rule also applies when the
client sets the global `-o json` flag: the JSON error document is written to
stderr, so stdout always stays a valid protocol stream.

## Session pinning

A session is pinned to the context and credential that were resolved at launch.
The adapter captures the context name, the management endpoint, the MCP
endpoint, and the bearer credential once, then passes those fixed values to
every operation it runs.

Consequences:

- Changing the saved default context with `bridgectl context set` does not
  retarget a running session. The session keeps using its launch-time endpoints
  and credential.
- Changing an endpoint or credential in the configuration file, or through
  `BRIDGE_*` environment variables, does not affect a running session.
- Restart the process to adopt changed endpoints or credentials. There is no
  active-session credential refresh.
- A child operation that cannot load the pinned context fails with
  `MCP_CONTEXT_CHANGED`. It never falls back to another saved context.

The guard covers empty values as well. A session that started without a
credential cannot silently acquire a newly configured one.

## Permissions

By default the session exposes inspection, local validation, and generation
only. A write operation requires the operator to approve its group at launch.

| Group | Allows |
| --- | --- |
| `registry` | API registration and credential-reference changes |
| `resources` | Applying tool, plugin, and binding manifests |
| `destructive` | Deletion, deactivation, token revocation, and cache flush |
| `probe` | Server-side registered API tests, which can cause downstream effects |
| `context` | Changing the saved default context |

`--allow-write` is repeatable and accepts a comma-separated list. There is no
wildcard. An unknown group name is a startup error.

Approval has these limits:

- A tool from a group that was not approved is neither advertised nor callable.
- Local approval does not grant remote permission. The ERPBridge deployment
  authorizes each request independently, and a local group never elevates a
  token.
- The local trust boundary is the operating-system identity of the process.
  Using stdio does not add a second authentication layer.
- Exact-target confirmation prevents accidental target mismatch. It is not
  proof of human consent. Configure approvals in the agent host as well.

### Approved tools

These tools require the stated group. Each one also requires `confirmContext`
to equal the pinned context and `confirmTarget` to equal the canonical target
listed below.

| Tool | Group | Canonical target |
| --- | --- | --- |
| `bridgectl_api_register` | `registry` (and `destructive` when `force` is true) | the registration name |
| `bridgectl_api_set_credential_ref` | `registry` | the API name |
| `bridgectl_api_test` | `probe` | the API name |
| `bridgectl_tool_apply` | `resources` | the manifest name, or `name@version` |
| `bridgectl_tool_delete` | `destructive` | `name@version` |
| `bridgectl_plugin_apply` | `resources` | the manifest name, or `name@version` |
| `bridgectl_plugin_delete` | `destructive` | `name@version` |
| `bridgectl_binding_apply` | `resources` | the manifest binding name |
| `bridgectl_binding_delete` | `destructive` | the binding name |
| `bridgectl_token_revoke` | `destructive` | the token identifier |
| `bridgectl_cache_flush` | `destructive` | `tool:<name>`, `module:<name>`, or `all` |
| `bridgectl_context_set` | `context` | the requested context name |

A mismatch performs no child execution. `bridgectl_cache_flush` accepts exactly
one selector: `tool`, `module`, or `all`. `bridgectl_tool_delete`,
`bridgectl_plugin_delete`, and `bridgectl_binding_delete` deactivate by default
and only delete permanently when `hard` is true.

### Write results

A completed write reports the target, `effect: "command_completed"`, and
`verified: false`. Registration also reports `replaced`. These results report
command completion, never reconciliation or remote verification.

When a launched write fails, times out, is cancelled, exceeds its output limit,
or returns output the adapter cannot interpret, the result is
`outcome_unknown` rather than a specific failure, because the change may already
have been applied. Read the target resource before retrying. The adapter never
retries a write automatically.

Only two failures report a specific code, because both are provably refused
before the command runs: a rejected argument vector, and a
`context_changed` precondition from the session guard.

A server-side probe that completes and reports a failed result is a finished
observation, not an unknown outcome. It returns `probe_failed` with the HTTP
status, content type, and latency.

### Saved default context

`bridgectl_context_set` changes only the saved default context in the
configuration file, and it only accepts a context that is already saved. It
preserves environment placeholders, stored values, and unknown fields, and
writes the replacement with mode 0600. If the file changed while the update was
being prepared, the update is refused instead of overwriting the other change.

The running session keeps its pinned context, so this tool cannot retarget it.

## Tool catalogue

A session without `--allow-write` exposes these tools:

| Tool | Operation |
| --- | --- |
| `bridgectl_context_list` | List saved contexts, marking the one pinned to this session |
| `bridgectl_api_list` | List the APIs registered in the local registry |
| `bridgectl_tool_get` | Read one tool, or list tools |
| `bridgectl_tool_generate` | Generate the tool manifest for a registered API |
| `bridgectl_tool_validate` | Validate one tool manifest locally |
| `bridgectl_plugin_get` | Read one plugin, or list plugins |
| `bridgectl_plugin_validate` | Validate one plugin manifest locally |
| `bridgectl_binding_get` | Read one plugin binding, or list bindings |
| `bridgectl_binding_validate` | Validate one plugin binding manifest locally |
| `bridgectl_token_list` | List API token metadata |
| `bridgectl_cache_stats` | Read tool cache statistics |
| `bridgectl_log_stats` | Read counts by log level and by tool |

Every tool returns one JSON object with `schemaVersion`, `operation`, `context`,
`success`, and either `data` or a safe `error` object. Failures set the MCP tool
error flag.

### Result redaction

Inspection results are an allow-list, not a copy of the command output. They
never include credential values, literal authentication header values, schema
defaults or examples, raw endpoint URLs, plugin configuration values, API token
values, or raw log messages.

- Endpoint identity is reported as a neutral host label, so an operator can tell
deployments apart without exposing paths or credentials.
- Schema declarations are preserved, including a property named `password` or
`token`, so a manifest stays reviewable. Only concrete values are removed.
- Inspection results are marked `redacted: true` and are not round-trippable. Use
`bridgectl_tool_generate` to obtain an applicable manifest.
- Generation returns the executable manifest, because applying it is the purpose
of the operation. The adapter refuses to return a manifest that contains a
literal credential value or an unsafe endpoint.

### Argument rules

- Schemas are strict: an unknown property is rejected before any command runs.
- Strings are length-bounded, and a value may not start with `-`, so a caller
cannot inject an option.
- Manifests are one inline JSON object. Paths, directories, URLs, YAML streams,
and arrays are rejected.
- Manifest tools do not accept embedded credential values. They reject a
`kind` that does not match the tool.
- Remote OpenAPI fetch, `$ref` resolution, output directories, and file writes
stay in the manual CLI.

### Validation results

A manifest that fails local validation returns `invalid_arguments` with an
adapter-owned message. The command's own validation text is not forwarded,
because child output is never trusted. Run the same validation with the
`bridgectl` CLI to see the exact reason.

## Execution limits

- One operation runs at a time. A concurrent call returns a busy result.
- Each operation has a 30-second deadline.
- A child process never receives the MCP input stream, a shell, a caller-chosen
  executable, a caller-chosen working directory, or a caller-chosen file path.
- Captured output, inline manifests, and incoming protocol messages are bounded.
  An oversized stream ends that operation; an oversized protocol message ends
  the session.
- Manifests are passed as a single inline JSON object. The adapter stages them
  in a private temporary directory and removes them afterwards.

## Limits of this release

- Only the catalogue above is exposed. There is no generic command tool, no
  shell, and no filesystem or URL access.
- Reads report what the pinned deployment returns. They do not prove that an
  ERP endpoint is reachable.
- Log statistics summarise the deployment's recent events. Log messages are
  never returned.
- Token creation stays in the manual CLI, so a new credential value never enters
  an agent conversation.
- Local validation reports that a resource failed, not why. Run the same
  validation with the `bridgectl` CLI to see the exact reason.
- Concurrent context edits from another process can still race the final
  rename. Do not run two context edits at the same time.

## Interrupted writes

A write operation that is cancelled, times out, or cannot be interpreted is
reported as `outcome_unknown`. The adapter does not claim that the operation was
rolled back and does not retry it. Read the target resource before retrying.

## Connect a coding agent

Register the management server separately from the runtime server. The runtime
server exposes published ERP tools; the management server exposes the catalogue
above. Use different names so a grant is never ambiguous.

Use an absolute path to the `bridgectl` binary, keep the token in the host
environment rather than in the configuration file, and start with the read-only
default. These examples are documentation only: the adapter never edits a
coding-agent configuration and never launches an agent.

### Codex CLI

```toml
[mcp_servers.erpbridge_manage]
command = "/usr/local/bin/bridgectl"
args = ["mcp", "--stdio", "--context", "staging"]
env_vars = ["BRIDGE_API_TOKEN"]
enabled_tools = ["bridgectl_context_list", "bridgectl_api_list", "bridgectl_tool_get"]
```

`env_vars` forwards host environment variables to the child process, so the
credential stays outside the configuration file. Add write groups to `args`
only for the deployment where they are intended:

```toml
[mcp_servers.erpbridge_manage]
command = "/usr/local/bin/bridgectl"
args = ["mcp", "--stdio", "--context", "staging", "--allow-write", "registry,resources"]
env_vars = ["BRIDGE_API_TOKEN"]
default_tools_approval_mode = "prompt"
```

For the runtime server, keep a separate entry that starts `erpbridge-server
--stdio` or points at `https://<erpbridge-host>/mcp/`.

### Claude Code

```sh
claude mcp add --transport stdio erpbridge_manage -- \
  /usr/local/bin/bridgectl mcp --stdio --context staging
```

With an explicit write grant:

```sh
claude mcp add --transport stdio erpbridge_manage -- \
  /usr/local/bin/bridgectl mcp --stdio --context staging --allow-write registry,resources
```

Keep `BRIDGE_API_TOKEN` in the host environment. Claude Code also supports an
`--env` option; prefer the host environment when a secret store already provides
the value.

### What the grant does and does not do

- It governs only this local process. The ERPBridge deployment authorizes every
  request again on its own.
- It is not human approval. Confirmation fields prevent a target mismatch; they
  do not prove that a person reviewed the call. Configure approvals in the agent
  host as well.
- It does not cover token creation, OpenAPI import, file output, or arbitrary
  commands. Those stay in the manual CLI.
- It does not survive a restart of the MCP process. Adding or removing a group
  requires a new process.

## Restart and recovery

Endpoints, the credential, and the write grants are captured when the process
starts. After you change an endpoint, a credential, or a grant, restart the MCP
process. A running session keeps its pinned selection and never falls back to
another saved context.

If a write reports `outcome_unknown`, read the target resource before retrying.
The adapter does not roll back and does not retry, so a second attempt could
repeat an effect that already happened.

## Related documentation

- [Agentic tools MCP integration](./agent-integrations.md) covers runtime MCP
  access to published ERP tools.
- [Connectivity and transport](./connectivity.md) covers transports, endpoints,
  and authentication.
- [ERPBridge Console](./web-console.md) covers the read-only local web console.
