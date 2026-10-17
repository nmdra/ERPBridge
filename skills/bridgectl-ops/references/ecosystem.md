# ERPBridge ecosystem context

## Components and boundaries

| Component | Role | Use it when |
| --- | --- | --- |
| `erpbridge-server` | Runtime that exposes MCP and authenticated HTTP endpoints, executes registered tools, enforces roles, and owns cache/log behavior. | Diagnosing server behavior or MCP availability. |
| `bridgectl` | Control CLI for contexts, API registration/testing, tool resources, tokens, logs, cache, and bundled skill installation. | Performing supported operations against an ERPBridge environment or refreshing the local operations skill. |
| MCP client | Discovers and calls tools through stdio or `/mcp/`. | Verifying user-visible tool discovery and calls. |
| `bridgectl mcp --stdio` | Local management MCP server for AI coding agents. It serves the `bridgectl` management catalogue, not published ERP tools, and pins the session to its launch-time context. | Verifying management tool discovery, grants, and agent-facing results. |
| `@erpbridge/sdk` | TypeScript facade for MCP tools, registry/direct invoke, logs, metrics, health, and cache. | Diagnosing application integration or envelope-handling behavior. |
| External plugin process | Separately operated HTTP process or container that receives the bounded `/v1/process` contract and returns a result envelope. | Verifying post-response processing or diagnosing plugin endpoint behavior. ERPBridge stores and invokes it but does not deploy it. |
| ERPBridge source repository | Source of truth for the checked-out revision, tests, and server implementation. | Reproducing, tracing, or fixing a defect. |

## Documentation source order

The published documentation site is <https://toolplane.nimendra.online/>.
Its complete published documentation reference is the
[`llms.txt`](https://toolplane.nimendra.online/llms.txt) index. When
published guidance may have changed, fetch that index and then only the
focused Server, Bridgectl, or SDK pages. Use
[`llms-full.txt`](https://toolplane.nimendra.online/llms-full.txt) only
when the focused pages do not answer the question.

Compare the published version with `bridgectl version` and the checked-out
repository revision. The local source and generated `docs/cli/` reference own
the current checkout; the site owns published user guidance. If the versions
do not match, state that fact and do not transfer destructive instructions
between versions without verification. If the site is unavailable, use the
local `erpbridge-docs` checkout, then in-repo docs and source.

## Bundled skill distribution

The repository copy under `skills/bridgectl-ops/` is authoritative. The
`bridgectl` binary embeds its `SKILL.md`, references, and assets; it does not
embed local evaluation files. Installation is local and does not contact an
ERPBridge server.

Use `bridgectl skill install --project` for the current project, omit selectors
for the default global destination, or pass `--dir <path>` to materialize a
specific destination. `--force` replaces the existing skill tree; confirm the
exact destination before using it. Never hand-edit the installed copy.

For normal CLI upgrades, use the operator's chosen installation channel. For a
GitHub release, verify the platform asset against the release checksum before
replacement. A release binary embeds the skill from that release, not newer
source edits. To validate an unreleased skill change, build a temporary helper
from the approved checkout and install to a fresh temporary directory first.
Keep an existing release CLI in place unless binary replacement is approved.
Refresh a derived skill installation only after its destination is approved.

## Project-local operational knowledge

`.agents/skill-memory/bridgectl-ops/` is optional project-local operational
and evaluation state. Its append-only execution evidence, maintained knowledge
entries, and append-only skill-impact history are not authoritative skill
content and are not included by the normal `bridgectl skill install` bundle.
The repository tree under `skills/bridgectl-ops/` remains the source of truth.

Memory can support a repository skill-change proposal, but only an accepted,
validated source change enters the normal build, embedding, and installation
process. Never hand-edit an installed skill to apply a memory lesson, and do
not persist credentials, authorization headers, ERP/plugin bodies, personal
data, unrestricted logs, or private reasoning in the memory tree.

## Management MCP server

`bridgectl mcp --stdio` is separate from `erpbridge-server --stdio` and exposes
a bounded management catalogue, not published ERP tools or every CLI command.
Read [Management MCP](mcp.md) for discovery, serial execution, result handling,
client setup, grants, and recovery. The skill's per-action confirmation gates
apply through both interfaces. Preserve separate runtime access.

## Protocol boundary

MCP clients and the SDK must preserve the MCP result envelope. A text content
item can contain a JSON-encoded ERPBridge compatibility result; it is not a
replacement for the complete MCP response. REST direct invocation addresses
registered tools only, not built-in MCP system tools. Plugin processing is
shared by MCP and direct tool invocation; the plugin payload is not a pass-through
of caller arguments or credentials.
