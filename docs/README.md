# ERPBridge Documentation

Welcome to the ERPBridge documentation. This wiki-style guide will help you understand, deploy, and extend the middleware.

## 📖 Core Documentation

- **[Overview & Architecture](../README.md)**: High-level introduction to ERPBridge and its core components.
- **[Docker Deployment Guide](./docker.md)**: Detailed instructions for running the full stack using Docker Compose.
- **[Connectivity & Transport Guide](./connectivity.md)**: Understanding SSE, Streamable HTTP, and Direct API transports.
- **[AI Agent Integration](../../AGENTS.md)**: Specific patterns for connecting Claude, Cursor, and other agents.

## 🛠 Developer Resources

- **[CLI Reference (bridgectl)](./cli/bridgectl.md)**: Comprehensive guide to the developer CLI.
- **[CLI API Management](./cli/bridgectl_api.md)**: How to register and test ERP endpoints.
- **[CLI Tool Management](./cli/bridgectl_tool.md)**: Generating and validating MCP tool schemas.
- **[CLI Cache Management](./cli/bridgectl_cache.md)**: Monitoring and flushing the semantic cache.

## 🔌 Integration Guides

- **[Postman Integration](./connectivity.md#postman-configuration)**: Testing MCP endpoints with Postman.
- **[MCP Client Guide (Python & TypeScript)](./mcp-client-guide.md)**: Build MCP clients for Streamable HTTP and Stdio.
- **[Mock ERP Setup](../mock-erp/README.md)**: Details about the simulated legacy ERP service.

## 🛡 System Features

- **Resilience**: Circuit breakers and retry logic (see [README](../README.md#3-resilience--reliability)).
- **Semantic Caching**: Efficient vector-based search for ERP responses (see [README](../README.md#6-semantic-caching)).
- **Hot Reloading**: Instant schema updates without downtime (see [README](../README.md#8-development-flow)).
