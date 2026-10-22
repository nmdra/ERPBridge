# ToolPlane Documentation

<a id="erpbridge-documentation"></a>

Toolplane(previously known as ERPBridge) connects existing ERP APIs to MCP
clients. Use these guides to understand, deploy, and extend the control plane.
Commands, configuration keys, API namespaces, package names, and repository
URLs retain their ERPBridge names.

## 📖 Core Documentation

- **[Architecture Overview](./architecture.md)**: The declarative control plane, SQLite registry, and reconciliation loop.
- **[ToolPlane Conformance Mechanisms](./toolplane-conformance.md)**: Admission digests, serving revisions, invocation snapshots, dispatch authority, origin binding, and recovery status.
- **[Onboarding New APIs](./onboarding.md)**: Register APIs and generate MCP tools.
- **[Tool Schema Reference (V2)](./tool-schema.md)**: Create versioned, intent-based MCP tool definitions.
- **[External Plugin Resource Schema](./plugin-schema.md)**: Define exact plugin versions, bindings, and the bounded HTTP contract.
- **[Docker Deployment Guide](./docker.md)**: Run the full stack with Docker Compose.
- **[MockERP Integration Contract](./mock-erp.md)**: The pinned image, SQLite seed, credentials, reset procedure, and supported DocTypes.
- **[Connectivity & Transport Guide](./connectivity.md)**: Streamable HTTP, Stdio, and Direct API transports.
- **[Agentic Tools MCP Integration](./agent-integrations.md)**: Connect Codex CLI, OpenCode, OpenClaw, and Hermes Agent with secure stdio or HTTP configuration.
- **[AI Agent Integration](../AGENTS.md)**: Agent integration and repository development rules.
- **[Environment Variables Reference](./environment-variables.md)**: Server and CLI environment variables.
- **[REST API Reference](./api.md)**: Direct HTTP endpoints of the server.
- **[API Token Guide](./tokens.md)**: Configure bearer authentication and manage scoped tokens.

## 🛠 Developer Resources

- **[bridgectl on Windows](./windows.md)**: Install the ZIP without administrator access. Configure PowerShell, contexts, and management MCP. Read the validation limits.
- **[CLI Reference (bridgectl)](./cli/bridgectl.md)**: CLI commands, including bundled agent skill installation.
- **[Management MCP Server](./bridgectl-mcp.md)**: Expose management tools through `bridgectl mcp --stdio`. Read-only tools are enabled by default. An operator must explicitly enable write groups.
- **[ERPBridge Console](./web-console.md)**: Start the local read-only Console with `bridgectl web`. Monitor local or remote deployments through configured contexts.
- **[CLI API Management](./cli/bridgectl_api.md)**: Register and test ERP endpoints.
- **[CLI Tool Management](./cli/bridgectl_tool.md)**: Manage the live tool registry with `apply`, `get`, and `validate`.
- **[CLI Plugin Management](./cli/bridgectl_plugin.md)**: Manage external plugins and exact-version bindings with `bridgectl plugin`.
- **[CLI Cache Management](./cli/bridgectl_cache.md)**: Monitor and flush the exact-match cache.
- **[Benchmarking](./benchmarking.md)**: Run deterministic Go microbenchmarks for cache and request preparation paths.
- **[Public Documentation Site](https://toolplane.nimendra.online/)**: Published server, `bridgectl`, plugin, and SDK guides.

## 🔌 Integration Guides

- **[Postman Integration](./connectivity.md#postman-configuration)**: Test MCP endpoints with Postman.
- **[Azure Monitor Metrics](./azure-monitor-prometheus.md)**: Send Prometheus `/metrics` data to Azure Monitor. Read the supported collection paths, cost drivers, and limits.
- **[MockERP Repository](https://github.com/nmdra/mockerp)**: The pinned test ERP service and its API contract.
- **[MCP Client Implementation Guide](../docs/mcp-client-guide.md)**: Implement an MCP client for this server.
- **[FAQ](./faq.md)**: Configuration, troubleshooting, and upgrades.
- **[ERPBridge Plugins](https://github.com/nmdra/ERPBridge-Plugins)**: Separately deployed plugin processes and integration examples.
- **[ERPBridge SDK](https://github.com/nmdra/erpbridge-sdk)**: TypeScript MCP client and typed REST clients.
- **[Documentation Repository](https://github.com/nmdra/erpbridge-docs)**: Source for the public documentation site.

## 🛡 System Features

- **Resilience**: Circuit breakers and bounded retries for eligible GET, HEAD, and OPTIONS failures. The connector does not automatically replay POST, PUT, PATCH, or DELETE. See [Connectivity](./connectivity.md#4-protection--limits) and [Architecture](./architecture.md#security-design).
- **[Exact Match Caching](./caching.md)**: Cache ERP responses with Redis or a bounded in-memory backend.
- **Secure Logging**: Stream logs with PII and secret redaction. RFC 5424 levels control log delivery.
- **Rate Limiting**: Limit tool calls by authenticated caller, session, or shared anonymous scope.
- **Declarative Management**: Store versioned tools in a registry. Background reconciliation updates serving tools without a server restart.
