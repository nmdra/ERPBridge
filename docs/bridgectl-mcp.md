# bridgectl MCP management server

`bridgectl mcp --stdio` serves ToolPlane management operations to an MCP
client, such as an AI coding agent.

This command is a **management** server. It manages the local API registry and
the configured ToolPlane deployment. It is not the runtime server. To call
published ERP tools, use `erpbridge-server --stdio` or
`<erpbridge-host>/mcp/`.

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

A session keeps the context and credential resolved at launch.
The adapter captures the context name, management endpoint, MCP endpoint, and
bearer credential once. It passes those fixed values to every operation.

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

The guard also covers empty values. A session that started without a
credential cannot silently acquire a newly configured one.

## Permissions

By default, the session exposes only inspection, local validation, and generation. A write operation requires the operator to approve its group at launch.

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
- Local approval does not grant remote permission. The ToolPlane deployment
  authorizes each request independently. A local group never elevates a token.
- The local trust boundary is the operating-system identity of the process.
  Using stdio does not add a second authentication layer.
- Exact-target confirmation prevents accidental target mismatch. It is not
  proof of human consent. Configure approvals in the agent host as well.

### Approved tools

These tools require the stated group. Each tool requires `confirmContext`
to equal the pinned context. It also requires `confirmTarget` to equal the
canonical target listed below.

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

Registration accepts an explicit empty `authType`, which registers an API with
no authentication.

If a confirmation value does not match, the adapter does not run a child
command. It stages nothing before the confirmation passes. `bridgectl_cache_flush` accepts exactly
one selector: `tool`, `module`, or `all`. `bridgectl_tool_delete`,
`bridgectl_plugin_delete`, and `bridgectl_binding_delete` deactivate by default.
They delete permanently only when `hard` is true.

### Write results

Most resource writes report the target, `effect: "command_completed"`, and
`verified: false`. Registration also reports `replaced`. Cache flush instead
reports `target`, `deleted`, optional `status`, and `verified`; saved-context
selection reports `savedDefault`, `activeContext`, and `verified`. These results
report command completion, not independent reconciliation or verification.
Before you claim that the change is verified, read back the exact target.
Make sure that it has the expected state.

When a launched write fails, times out, is cancelled, exceeds its output limit,
or returns output the adapter cannot interpret, the result is
`outcome_unknown` rather than a specific failure. The change may already
have been applied. Before you retry, read the target resource. The adapter never
retries a write automatically.

Only two failures report a specific code: a rejected argument vector and a
`context_changed` precondition from the session guard. Both stop execution
before the command runs.

A server-side probe that completes and reports a failed result is a finished
observation, not an unknown outcome. It returns `probe_failed` with the HTTP
status, content type, and latency.

### Saved default context

`bridgectl_context_set` changes only the saved default context in the
configuration file, and it accepts only a context that is already saved. It
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

Every tool returns one JSON object. It contains `schemaVersion`, `operation`,
`context`, `success`, and either `data` or a safe `error` object. Failures set the MCP tool
error flag. MCP initialization reports the same release version as
`bridgectl --version`, so clients can identify the installed management server.

If the selected deployment does not hold a resource, a read tool reports
`not_found`. If it rejects a credential, the read tool reports
`authentication_required`. The managed child reads the control-plane status
before it decodes a typed body. The adapter maps these results by code. It
never reads child prose.

Tool, plugin, and binding inventories return `data.items` as an array, including
`[]` when the inventory is empty. A named get returns `data.item` and omits
`data.items`.

### Result redaction

Inspection results contain only allowed fields, not a copy of the command output. They
omit credential fields, literal authentication header fields, schema defaults
and examples, raw endpoint URLs, plugin configuration, API token values, and
raw log messages. Retained descriptions, usage text, and schema enum values
are not arbitrary-content redaction. Before you share or persist those values, review them.
`redacted: true` does not prove that all retained text is safe.

- Endpoint identity is reported as a neutral host label, so an operator can tell
deployments apart without exposing paths or credentials.
- The adapter preserves schema declarations, including a property named
`password` or `token`, so operators can review the manifest. Retained descriptions and enum values
still require review.
- The adapter marks inspection results `redacted: true`. These results cannot
be applied as manifests. Use `bridgectl_tool_generate` to obtain an applicable
manifest.
- Generation returns an executable manifest for application. The adapter refuses to return a manifest that contains a
literal credential value or an unsafe endpoint.

### Argument rules

- MCP tool argument schemas are strict. The adapter rejects unknown arguments
before any command runs. Manifest validation is resource-specific; tool manifests do
not universally reject unknown resource fields. Remove unsupported fields.
Make sure that the reviewed resource passes local validation. Do not rely on
strictness.
- Strings have length limits. A value must not start with `-`. These rules
prevent a caller from injecting an option. Tool deletion encodes the name and version as literal
query values. Characters such as `&` cannot change the confirmed target or turn
a soft delete into a hard delete. Permanent deletion requires `hard: true`.
- Manifests are one inline JSON object. Paths, directories, URLs, YAML streams,
and arrays are rejected.
- Manifest tools do not accept embedded credential values. They reject a
`kind` that does not match the tool, an endpoint with credentials in its
userinfo or query, and a concrete default or example for a credential-like
schema property. A schema declaration is still allowed.
- Endpoint paths require the canonical field spellings `spec`, `execution`, and
`endpoint`. Case aliases such as `Spec` or `Endpoint`, including an alias beside
a canonical field, are rejected before staging.
- Credential screening shares the log redaction key policy. It rejects literal
values under names such as `client_secret`, `access_token`, and `private_key`,
including case variants and underscore-prefixed forms such as
`upstream_access_token`. Logical `credentialRef` and `credentialSource` fields
remain allowed.
- Remote OpenAPI fetch, `$ref` resolution, output directories, and file writes
stay in the manual CLI.

### Validation results

If a manifest fails local validation, the adapter returns `invalid_arguments`
with its own message. It does not forward the command's validation text,
because it never trusts child output. Run the same validation with the
`bridgectl` CLI to see the exact reason.

## Execution limits

- One operation runs at a time. A concurrent call returns a busy result.
- Each operation has a 30-second deadline.
- A child process never receives the MCP input stream, a shell, a caller-chosen
  executable, a caller-chosen working directory, or a caller-chosen file path.
- Captured output, inline manifests, and incoming protocol messages are bounded.
  An oversized stream ends that operation. An oversized protocol message ends
  the session.
- Manifests are passed as a single inline JSON object. The adapter stages them
  in a private temporary directory and removes them afterwards.

## Limits of this release

- Only the catalogue above is exposed. There is no generic command tool, no
  shell, and no filesystem or URL access.
- Reads report what the selected deployment returns. They do not prove that an
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
rolled back and does not retry it. Before you retry, read the target resource.

## Connect a coding agent

Register the management server separately from the runtime server. The runtime
server exposes published ERP tools; the management server exposes the catalogue
above. Use different names so a grant is never ambiguous.

Use an absolute path to the `bridgectl` binary. Keep the token in the host
environment, not in the configuration file. Start with the read-only default.
These examples are documentation only. The adapter never edits a coding-agent
configuration. It never launches an agent.

### Windows clients

See [Windows management MCP examples](./windows.md#management-mcp-on-windows)
for TOML and JSON with escaped absolute `.exe` paths, including paths containing
spaces. Invoke the executable directly with an `args` array, not through a
shell. Forward `BRIDGE_API_TOKEN` from the host environment using the client's
supported settings. Do not place its value in client files. Defaults remain
read-only. Stdout carries only the protocol stream. Restart after context or credential
changes. EOF closes the session; interactive Ctrl+C cleanup is a pending native
Windows manual gate, not a claim based on Unix SIGTERM tests.

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

- It governs only this local process. The ToolPlane deployment authorizes every
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

If a write reports `outcome_unknown`, read the target resource before you retry.
The adapter does not roll back or retry the operation. A second attempt could
repeat an effect that already happened.

## Operations skill

The repository `bridgectl-ops` skill 3.5.0 covers both CLI and management MCP.
Its [MCP workflow](../skills/bridgectl-ops/references/mcp.md) explains discovery,
serial calls, result checking, client setup, grants, and unknown-write recovery.
The [preflight profiles](../skills/bridgectl-ops/references/preflight.md) separate
core and remote checks from local Compose, onboarding, and plugin prerequisites.
Remote log/cache reads do not require local Docker or an API probe.

Use exact-action approval for every mutation or server-side probe. After each
change, use independent readback to make sure that the target has the expected
state. Use separate runtime and management client entries.
Optional memory lookup starts with a relevant index/area search; the terminal
redacted-record checkpoint remains in force even when no lesson matches.
Release binaries install their own embedded skill version, not unreleased
repository edits.

## Related documentation

- [Agentic tools MCP integration](./agent-integrations.md) covers runtime MCP
  access to published ERP tools.
- [Connectivity and transport](./connectivity.md) covers transports, endpoints,
  and authentication.
- [ERPBridge Console](./web-console.md) covers the read-only local web console.
