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

## Interrupted writes

A write operation that is cancelled, times out, or cannot be interpreted is
reported as `outcome_unknown`. The adapter does not claim that the operation was
rolled back and does not retry it. Read the target resource before retrying.

## Related documentation

- [Agentic tools MCP integration](./agent-integrations.md) covers runtime MCP
  access to published ERP tools.
- [Connectivity and transport](./connectivity.md) covers transports, endpoints,
  and authentication.
- [ERPBridge Console](./web-console.md) covers the read-only local web console.
