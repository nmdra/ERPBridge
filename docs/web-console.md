# ERPBridge Console

The ERPBridge Console is a read-only local web interface for `bridgectl`.
It shows configured contexts, deployment state, MCP tools, logs, live metrics,
and API-to-MCP paths. It also shows safe external plugin and binding metadata
when the selected deployment supports the completed plugin contract. The
homepage states this boundary clearly: use the console to monitor ERPBridge and
use `bridgectl` to configure or modify it.
The CLI remains the automation and mutation interface.

## Frontend development

The frontend uses React, TypeScript, Vite, Tailwind CSS, and source-owned UI
components. It uses Lucide for icons, React Flow for topology, and Recharts
for metrics.

Use Node `22.14.0` and npm `10.9.2`. Run these commands from the repository:

```sh
npm ci --prefix web
npm run dev --prefix web
bridgectl web --dev
npm run typecheck --prefix web
npm test --prefix web -- --run
```

The Go console serves the built frontend in production. The `--dev` flag
proxies frontend development traffic to Vite on loopback.
A clean checkout serves a tracked
fallback page until the frontend build creates hashed assets. Release builds
run the frontend build before GoReleaser and reject a fallback asset.

## Themes and accessibility

The console defaults to light mode and supports dark and system themes. It
stores an explicit choice in browser storage and follows the operating system
when the choice is system. The sidebar can be collapsed on wide screens and
keeps icon labels available through accessible names and tooltips. The console
also follows `prefers-reduced-motion`.

Status badges include text and an icon. Color does not carry status meaning by
itself. Graph and chart pages provide an HTML list or table alternative.

## Start the console

Run the console from the repository that contains your `bridgectl` binary:

```sh
bridgectl web
```

The command binds to `127.0.0.1` on an available port. It prints the console
URL and opens the default browser. Use `--no-open` to keep the browser closed.
Use `--url` to print the URL and keep the process active for another client.
The console stops when the command receives an interrupt or termination signal.

The first release does not listen on a remote address. It does not discover
Kubernetes, Docker, or cloud deployments.

## Security boundary

The console server is a local backend-for-frontend. Browser code never receives
ERPBridge bearer tokens, ERP credentials, registry credentials, or raw upstream
responses.

Each launch creates a high-entropy capability. The URL contains the capability
in its fragment. The browser removes the fragment from the address bar and sends
the capability in `X-ERPBridge-Console-Capability` for console API and SSE
requests.

The server accepts requests only for the exact loopback `Host` value that it
created. It rejects cross-origin `Origin` values. It sends a content security
policy, disables framing, prevents MIME sniffing, disables referrer forwarding,
and marks console responses as non-cacheable.

The configured context file is trusted operator input. Browser requests cannot
select an arbitrary upstream URL. The server accepts only known context names
and fixed read operations. It validates configured `Server` and `MCPServer`
URLs, disables redirects, forwards no browser headers, and bounds response
sizes.

The console does not add mutation routes. It cannot apply or delete tools, flush
cache, invoke tools, create or revoke tokens, deploy plugins, or restart a
deployment.

## Context and data sources

Named `bridgectl` contexts are the initial deployment inventory. A context can
contain a management `Server` URL and an MCP `MCPServer` URL. The console keeps
their credentials on the server and returns only display-safe identity and
availability fields.

The upstream route map is fixed:

| Console data | Upstream route | Context URL |
| --- | --- | --- |
| Health | `/mcp/health` | `MCPServer` |
| Metrics | `/metrics` | `MCPServer` |
| Tool inventory | `/apis/erpbridge.io/v1/tools` | `MCPServer` |
| Plugins | `/apis/erpbridge.io/v1/plugins` | `MCPServer` |
| Plugin bindings | `/apis/erpbridge.io/v1/pluginbindings` | `MCPServer` |
| Cache statistics | `/api/cache/stats` | `Server` |
| Recent logs | `/api/logs/recent` | `Server` |
| Live logs | `/api/logs/stream` | `Server` |
| Server metadata | `/api/info` | `MCPServer` |

The console reports `unavailable` when an upstream deployment does not provide
an optional read endpoint or when its configured credential cannot read it.
It does not expose upstream headers, error bodies, raw ERP URLs, credential
references, or registry authentication fields.

Health, tool, cache, recent-log, and live-log requests use the fixed route map.
The server returns safe local error states for upstream failures.

## Local console API

The browser uses these same-origin, read-only routes. Every route except
`/healthz` requires the capability header and the exact local `Host` and
`Origin` values. The `context` query value must name a configured context.
The contexts and deployment routes return safe projections without upstream
URLs or credentials.

| Route | Purpose |
| --- | --- |
| `GET /healthz` | Check the local console process. |
| `GET /api/console/v1/contexts` | List safe context projections. |
| `GET /api/console/v1/deployment?context=<name>` | Read deployment identity and local availability. |
| `GET /api/console/v1/health?context=<name>` | Read safe upstream health. |
| `GET /api/console/v1/tools?context=<name>` | Read the safe tool inventory. |
| `GET /api/console/v1/plugins?context=<name>` | Read safe plugin metadata. |
| `GET /api/console/v1/plugin-bindings?context=<name>` | Read safe binding metadata. |
| `GET /api/console/v1/cache?context=<name>` | Read safe cache statistics. |
| `GET /api/console/v1/logs/recent?context=<name>` | Read projected recent log events. |
| `GET /api/console/v1/logs/stream?context=<name>` | Stream projected log events over SSE. |
| `GET /api/console/v1/metrics?context=<name>` | Read a typed live metric snapshot. |
| `GET /api/console/v1/topology?context=<name>` | Read the API-to-MCP topology. |
| `GET /api/console/v1/server-info?context=<name>` | Read optional server metadata. |

The console returns stable JSON error states for missing contexts, unavailable
endpoints, unauthorized reads, timeouts, malformed upstream data, and bounded
response failures. It does not return an upstream error body. The Tools page
renders the safe tool projection as a filterable table. Tool names link to a
read-only manifest page with descriptive guidance, input fields, execution
paths, security roles, routing hints, cache settings, and lifecycle metadata.
The page does not invoke or mutate tools. Manifest projections omit credential
references, default values, raw output schemas, and full upstream URLs.

## Topology semantics

The topology describes these paths:

```text
MCP client -> MCP tool -> ERP API endpoint
                    `-> plugin binding -> external plugin -> result
```

The server matches a tool execution method and endpoint against sanitized local
API registry entries. It normalizes methods, trailing slashes, default ports,
and configured ERP base substitutions before matching.

Each relationship has one match state:

- `exact`: the method and normalized endpoint match one API entry.
- `base-prefix`: the server inferred a match from a configured base prefix.
  A root API registration can infer paths across HTTP methods, but the edge is
  not authoritative.
- `ambiguous`: more than one API entry matches.
- `unresolved`: no safe API match exists.

The console marks local registry entries as `context matched` or `unassigned`.
The registry has no deployment context field, so the console does not claim
cross-environment ownership. It shows an endpoint path and selected-context
label, not a full upstream URL.

The topology includes MCP transport, MCP tool, ERP API, plugin binding,
external plugin, and unresolved endpoint nodes. Plugin relationships appear only
when both plugin list routes are available. Use the search field and the node
kind, match-confidence, and context-state filters to narrow the graph. The
canvas and accessible relationship list share selection state: selecting a node
highlights its immediate relationships, while selecting an edge shows its source,
target, match confidence, authority, context state, and safe endpoint paths. The
BFF reports when safety caps omit nodes or edges, so an incomplete graph is not
mistaken for a complete one. The BFF reads the local registry server-side and
strips all registry authentication fields. The canvas loads on demand so the
list view remains available on small or keyboard-only clients.

## Logs and metrics

The console projects log events into a fixed safe shape. The shape contains a
timestamp, level, component, tool name, request ID, and redacted summary. The
Logs page sorts events by timestamp with the most recent events first; events
without a valid timestamp appear last. It
omits unknown fields, raw payloads, credentials, personal data, and malformed
or oversized events. Live streams use a capability-protected fetch stream and
stop when the browser disconnects. The server limits concurrent streams.

Metrics are live scrape samples. The console preserves cumulative totals and
calculates rates only from successive samples during the current session. It
uses histogram sum and count for average latency. It does not claim percentile
latency or historical Prometheus data. Unknown metric families do not fail the
snapshot. Charts include a text or table view.

## Plugins

The Plugins page reads the final plugin and `PluginBinding` list routes. Each
plugin entry links to an exact `/plugins/<name>/<version>` detail page. The
page shows exact version, active state, deployment type, timeout,
endpoint/configuration booleans, unknown health, and matching bindings. Plugin
metadata is kept out of the Tools inventory. A deployment that returns `404`
for either route shows an unavailable feature state instead of a failed
console.

The plugin view exposes only `endpointConfigured` and
`configurationPresent` booleans. It does not expose plugin endpoints, static
binding configuration, credentials, or invocation payloads. It reports plugin
health as `unknown` unless the final plugin API provides an approved health
field.

## Limitations

The console has no persistent log or metric store. It keeps a bounded sample
window in browser memory while it runs. Historical dashboards require a future
Prometheus query integration.

The console does not change the active persistent `bridgectl` context when a
user selects a deployment in the browser. The selection applies only to the
current console session. The application routes include Overview,
Deployments, Logs, Metrics, Tools, Plugins, Topology, and Settings.
