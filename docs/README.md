# ERPBridge Documentation

Welcome to the ERPBridge documentation. This wiki-style guide helps you understand, deploy, and extend the middleware.

## 📖 Core Documentation

- **[Architecture Overview](./architecture.md)**: Understanding the Declarative Control Plane, SQLite registry, and reconciliation loop.
- **[Onboarding New APIs](./onboarding.md)**: Standard workflow for registering APIs and generating MCP tools.
- **[Tool Schema Reference (V2)](./tool-schema.md)**: Detailed guide to creating versioned, intent-based MCP tool definitions.
- **[External Plugin Resource Schema](./plugin-schema.md)**: Define exact plugin versions, bindings, and the bounded HTTP contract.
- **[Docker Deployment Guide](./docker.md)**: Detailed instructions for running the full stack using Docker Compose.
- **[MockERP Integration Contract](./mock-erp.md)**: Pinned image, SQLite seed, credentials, reset, and supported DocTypes.
- **[Connectivity & Transport Guide](./connectivity.md)**: Understanding Streamable HTTP, Stdio, and Direct API transports.
- **[Agentic Tools MCP Integration](./agent-integrations.md)**: Connect Codex CLI, OpenCode, OpenClaw, and Hermes Agent with secure stdio or HTTP configuration.
- **[AI Agent Integration](../AGENTS.md)**: Specific patterns for connecting Claude, Cursor, and other agents.
- **[Environment Variables Reference](./environment-variables.md)**: All server and CLI environment variables.
- **[REST API Reference](./api.md)**: Direct HTTP endpoints of the server.
- **[API Token Guide](./tokens.md)**: Configure bearer authentication and manage scoped tokens.

## 🛠 Developer Resources

- **[CLI Reference (bridgectl)](./cli/bridgectl.md)**: Comprehensive guide to the developer CLI, including bundled Agent Skill installation.
- **[ERPBridge Console](./web-console.md)**: Local read-only monitoring console launched by `bridgectl web`.
- **[CLI API Management](./cli/bridgectl_api.md)**: How to register and test ERP endpoints.
- **[CLI Tool Management](./cli/bridgectl_tool.md)**: Managing the live tool registry using `apply`, `get`, and `validate`.
- **[CLI Plugin Management](./cli/bridgectl_plugin.md)**: Managing external plugins and exact-version bindings with `bridgectl plugin`.
- **[CLI Cache Management](./cli/bridgectl_cache.md)**: Monitoring and flushing the exact match cache.
- **[Benchmarking](./benchmarking.md)**: Run deterministic Go microbenchmarks for cache and request preparation paths.
- **[Public Documentation Site](https://blog.nimendra.xyz/erpbridge-docs/)**: Published Server, Bridgectl, and SDK guides.

## 🔌 Integration Guides

- **[Postman Integration](./connectivity.md#postman-configuration)**: Testing MCP endpoints with Postman.
- **[MockERP Repository](https://github.com/nmdra/mockerp)**: Details about the pinned simulated legacy ERP service and its API contract.
- **[MCP Client Implementation Guide](../docs/mcp-client-guide.md)**: How to implement an MCP client for this server.
- **[FAQ](./faq.md)**: Common questions about configuration, troubleshooting, and upgrades.

## 🛡 System Features

- **Resilience**: Circuit breakers and retry logic (see [Architecture](./architecture.md#security-design)).
- **[Exact Match Caching](./caching.md)**: Backend-independent caching for ERP responses.
- **Secure Logging**: Real-time log streaming with automatic PII/Secret redaction and RFC 5424 level control.
- **Rate Limiting**: Per-session request throttling for infrastructure protection.
- **Declarative Management**: Versioned tool registry with background reconciliation (no restarts needed).
