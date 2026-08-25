# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.3.0-alpha.3] - 2026-08-25

### Added

- Add agent-integration guidance for Codex CLI, OpenCode, OpenClaw, and Hermes
  Agent, including scoped HTTP bearer authentication and stdio credential
  boundaries

### Changed

- Pin the standalone MockERP image and versioned OpenAPI contract to `0.2.1`,
  which includes the SQLite-backed SCP scenario and integration fixtures
- Document the MockERP integration contract, credentials boundary, reset flow,
  and supported fixture groups
- Pass MockERP credential-source settings through Compose and persist its named
  data volume configuration

### Fixed

- Keep ERPBridge stdio stdout reserved for MCP JSON-RPC by routing the startup
  banner to stderr

## [v0.3.0-alpha.2] - 2026-08-23

### Fixed

- Initialize MCP tool invocation and duration metric series at registration so
  cold-start `/metrics` scrapes expose zero-valued samples.

## [v0.3.0-alpha.1] - 2026-08-22

### Added

- Add bounded in-memory caching when Redis is not configured, with full-hash keys, cache timestamps, and backend-aware flushing
- Apply generated YAML sequences and multi-document YAML streams, with exact server-side tool filtering
- Add hashed, scoped API tokens with expiry, revocation, admin roles, and one-time token disclosure
- Add bearer-token precedence and `bridgectl token create`, `list`, and `revoke` commands
- Add optional per-tool role allow-lists with MCP discovery and direct-invoke selectors
- Add the `bridgectl-ops` operations skill for onboarding, maintenance, diagnosis, and sanitized bug reporting

### Changed

- Document MCP result envelopes, registry-only REST invocation, memory-cache behavior, and the current tool schema template
- Document authenticated route policy, role selectors, CORS, token operations, and CLI configuration

### Fixed

- Fail closed for unresolved credential references and redact tool arguments across log handlers
- Hide inactive tools on Stdio, harden registry state hashing and migrations, evict idle rate-limit identities, and validate nested response paths
- Dereference generated output schemas and report unresolved OpenAPI references with operation context
- Gracefully shut down the HTTP listener on SIGTERM and SIGINT

### Security

- Redact authorization headers and prevent denied guarded calls from reaching cache or downstream ERP execution

## [v0.2.0-alpha.5] - 2026-05-10

### Added

- feat(db): add `HardDelete` support to SQLite store for permanent tool removal
- feat(server): support `hard=true` parameter in tool delete API
- feat(cli): enhance `bridgectl tool delete` with `--hard` flag and interactive confirmation
- feat(cli): add `-y, --yes` flag to bypass hard-delete confirmation
- test: add comprehensive tests for hard-delete in database, server, and CLI layers

### Changed

- docs: update `tool delete` documentation and onboarding guide with hard-delete instructions
- style(cli): improve `tool delete` error messaging for missing arguments
- style(cli): improve `tool delete` success output with descriptive states

## [v0.2.0-alpha.4] - 2026-05-10

### Added

- docs: add comprehensive "Onboarding New APIs" guide with troubleshooting and quick reference
- feat(cli): add recursive directory support for `bridgectl tool apply -f`

### Changed

- docs: update README.md index to include onboarding guide
- docs: polish and expand onboarding guide for better developer experience

### Fixed

- fix(cli): resolve issue where `bridgectl tool apply` failed on directory paths

## [v0.2.0-alpha.3] - 2026-05-10

### Changed

- ci: clean up GoReleaser indentation
- docs: add comprehensive GitHub badges to README

## [v0.2.0-alpha.2] (Observability & Hardening) - 2026-05-10

### Added

- feat(mcp): implement Server Hooks for lifecycle telemetry and business logic
- feat(mcp): implement deletion reconciliation (automatic tool deregistration)
- feat(mcp): add tool icons and output schema validation
- feat(metrics): add server lifecycle and session Prometheus metrics
- feat: add ASCII startup banners for server and CLI
- feat: add Makefile for automated setup, build, test, and tool generation
- test: achieve high coverage (>70%) across core packages (MCP, IDP, Cache, CLI, Output)
- ci: modernize GoReleaser config with multi-platform builds, SBOM (Syft), and Signing (Cosign)

### Changed

- refactor(mcp): optimize reconciliation loop with state-hash short-circuiting
- refactor(mcp): migrate built-in system tools to type-safe structured handlers
- chore(ci): harden production Dockerfile with non-root user and pinned Alpine 3.22.4
- chore: ignore schemas directory in git (transition to DB-driven registry)
- ci: update GitHub Actions to latest major versions (checkout@v6, setup-go@v6, goreleaser@v7)
- ci: limit GoReleaser targets to linux/amd64 and standardize configuration quotes

### Fixed

- fix(mcp): ensure graceful shutdown of reconciliation controller via signal-aware context
- fix(logger): safely handle nil context in FromContext to prevent panics
- fix(ci): resolve Cosign version resolution and bundle signing issues

## [v0.2.0-alpha.1] (Declarative V2) - 2026-05-09

### Added

- feat(mcp): implement Declarative Control Plane with SQLite registry
- feat(mcp): add SemVer-based tool versioning and resolution
- feat(mcp): implement background reconciliation controller
- feat(cli): add `bridgectl tool apply`, `get`, `delete`, and `describe`
- feat(cli): add dynamic shell auto-completion for tool resources
- feat(idp): enhance OpenAPI generator with semantic naming and pluralization
- docs: add V2 architecture overview and schema reference guides

### Changed

- refactor(mcp): migrate from file-system hot-reloading to API-driven registry
- refactor(security): decouple secrets from schemas using `credentialRef`
- chore(config): migrate all technical ERP schemas to semantic intent-based V2 format

### Fixed

- fix(cli): resolve lint issues and missing error checks for HTTP body close
- fix(test): update all test suites to align with V2 architecture

## [v0.1.0-alpha.4] (Kotiya) - 2026-05-08

### Added

- feat(mcp): implement custom notification system
- feat(core): add middleware infrastructure and native tool support
- docs: implement wiki-style documentation and docker guide

### Fixed

- fix(mcp): support recursive directory watching for schema hot reloading

### Changed

- refactor(mcp): migrate server logic to middleware-based architecture
- feat(mcp): remove deprecated SSE support in favor of Stdio and Streamable HTTP
- docs: clarify that schema hot reloading supports nested directories

### Improved

- test(mcp): improve test coverage for notifications and endpoints

## [v0.1.0-alpha.3] - 2026-05-08

### Added

- feat(mock-erp): enhance OpenAPI spec for better MCP tool integration
- feat(server): support STDIO transport via --stdio flag and MCP_TRANSPORT env var
- feat(mcp): add client logging, tool notifications, and completion providers
- feat(logger): support redirecting logs to stderr via LOG_TO_STDERR env var
- feat(mcp): implement native streamable http transport support
- docs: add connectivity and transport guide

### Fixed

- fix(docker): resolve build issues and dynamic URL routing
- fix(mcp): resolve tool marshaling conflict by clearing structured schema fields
- fix(logger): use t.Setenv and remove unused os import in tests

### Changed

- refactor(mcp): remove unused resource and prompt completion handlers
- style(cli): standardize error messages and improve context usage
- style(idp): update GenerateFromOpenAPI to accept context
- style(logger): unexport sensitiveKeys to improve encapsulation

### Improved

- docs(cli/mcp/connector): add documentation comments and enable linting
- test: add unit tests for logger, metrics, and cli errors

## [v0.1.0-alpha.2] - 2026-05-07

### Fixed

- fix: include erpbridge-server source and fix gitignore patterns
- fix(goreleaser): update repo owner to nmdra

### Changed

- chore: rename middleware to erpbridge-server
- docs: update README with package details and rename middleware to server

## [v0.1.0-alpha.1] - 2026-05-07

### Added

- feat: add lefthook configuration for automated linting, formatting, and testing
- feat(cli): implement agent-friendly improvements with structured errors and isolated JSON output
- feat: implement schema hot-reloading and bridgectl tool validate
- feat: instrument MCP server with metrics and implement stats endpoint
- feat: implement Prometheus metrics and cache statistics
- feat: add support for MCP Resources and Prompts
- feat: implement circuit breaking and intelligent retries in ERPConnector
- feat: switch embedder to nomic-ai/nomic-embed-text-v1
- feat: implement structured logging with slog, request tracing, and CLI log tailing
- feat: configure cache for finance tools
- feat: add bridgectl cache commands and output formatting
- feat: integrate semantic cache with MCP server and tool execution (internal)
- feat: implement role-aware semantic cache manager
- feat: implement openapi generation and response validation
- feat(cli): implement bridgectl CLI with API and tool management
- feat(middleware): implement MCP bridge middleware and ERP connector
- feat(mock-erp): implement mock ERP server with finance, HR, and inventory modules
- docs: enhance CLI self-documentation and add bridgectl doc generator
- docs: update README with resilience, observability, and DX features
- docs: add .env.example with customizable variables
- docs: add comprehensive guide to README.md
- ci: add GoReleaser release pipeline for middleware and bridgectl

### Fixed

- fix(workflow): downgrade go to 1.24 for golangci-lint compatibility
- fix(cli): resolve numerous lint issues (unchecked errors, body close, etc.)
- fix(idp/connector/config/logger): resolve various lint and stability issues

### Changed

- refactor(monorepo): restructure Go code into services/tools
- chore: prepare for versioning and release
- chore: rename middleware to erpbridge-server

[Unreleased]: https://github.com/nmdra/ERPBridge/compare/v0.3.0-alpha.3...HEAD
[v0.3.0-alpha.3]: https://github.com/nmdra/ERPBridge/compare/v0.3.0-alpha.2...v0.3.0-alpha.3
[v0.3.0-alpha.2]: https://github.com/nmdra/ERPBridge/compare/v0.3.0-alpha.1...v0.3.0-alpha.2
[v0.3.0-alpha.1]: https://github.com/nmdra/ERPBridge/compare/v0.2.0-alpha.5...v0.3.0-alpha.1
