# Plan: Backward-Compatible Agentic Tools MCP Integration

## Goal

Document and verify direct ERPBridge MCP connections for Codex CLI, OpenCode,
OpenClaw, and Hermes Agent without changing the established Streamable HTTP
contract used by `@erpbridge/sdk`. Fix the one Stdio transport violation that
can prevent local MCP clients from initializing: startup output written to
stdout before JSON-RPC starts.

## Current State

- ERPBridge exposes a stateful Streamable HTTP endpoint at `/mcp/`, with a
  30-minute session TTL, authenticated request wrapping, CORS support for MCP
  headers, and an exposed `Mcp-Session-Id` (`internal/mcp/server.go:544-628`).
- ERPBridge starts a Stdio MCP server using `os.Stdin` and `os.Stdout`
  (`services/erpbridge-server/main.go:146-153`), but emits its banner to
  stdout before it determines that Stdio is selected (`main.go:54-66`). MCP
  reserves stdout exclusively for JSON-RPC; diagnostics must use stderr
  ([MCP Stdio guidance](https://ts.sdk.modelcontextprotocol.io/v2/get-started/first-server)).
- `mcp-go v0.57.0` negotiates `2025-03-26` through `2025-11-25`
  (`go.mod:12`; module `mcp/types.go:163-170`). Existing server tests cover
  CORS, authenticated initialization, session IDs, and tool calls
  (`internal/mcp/server_test.go:269-310`, `internal/mcp/auth_test.go:142-194`).
- The SDK uses the official Streamable HTTP client, sends its credential on
  every MCP request, preserves result envelopes, and already has real-server
  MCP compatibility and integration commands (`../erpbridge-sdk/src/mcp.ts:37-92`,
  `../erpbridge-sdk/src/mcp.ts:194-205`,
  `../erpbridge-sdk/package.json:scripts`). It has no Stdio transport, so a
  Stdio-only output fix does not alter its API or behavior.
- All target agents support the standard transports ERPBridge already serves:
  Codex supports Stdio and Streamable HTTP configuration
  ([Codex MCP docs](https://developers.openai.com/codex/mcp/)); OpenCode
  supports local Stdio and remote Streamable HTTP
  ([OpenCode MCP docs](https://opencode.ai/v2/docs/mcp-servers)); OpenClaw
  supports `streamable-http` plus probing and tool filtering
  ([OpenClaw MCP docs](https://docs.openclaw.ai/cli/mcp)); and Hermes supports
  Stdio/HTTP servers with headers and tool filtering
  ([Hermes MCP docs](https://hermes-agent.nousresearch.com/docs/reference/mcp-config-reference/)).

## Decisions

1. **Use standard MCP only.** No agent-specific adapters, endpoints, response
   envelopes, or transport forks are required. The canonical remote endpoint
   remains `https://<host>/mcp/`; `initialize` is a JSON-RPC request sent to
   that endpoint, not an `/mcp/initialize` route.
2. **Preserve the current HTTP contract.** Do not alter tool names, MCP result
   envelopes, session handling, CORS policy, bearer authentication, or
   negotiated versions. Regression tests and the existing SDK live suite are
   mandatory gates for these invariants.
3. **Make only the required Stdio behavior change.** Determine the transport
   before emitting the startup banner, write banner and logs to stderr in
   Stdio mode, and leave stdout for the `mcp-go` Stdio writer alone. HTTP-mode
   startup output stays unchanged.
4. **Recommend transport by deployment boundary.** Documentation recommends
   Stdio for a locally installed agent launching `erpbridge-server`, and
   Streamable HTTP for remote/shared deployments. Remote examples use a scoped
   ERPBridge API token with `Authorization: Bearer`; Stdio examples explain
   that inbound HTTP tokens do not apply and that guarded tools require an
   authenticated HTTP identity.
5. **Treat ERP operations as a least-privilege surface.** Document per-agent
   tool allowlists for OpenClaw and Hermes, and equivalent tool controls where
   offered by Codex and OpenCode. Examples never contain literal credentials.
6. **Do not add OAuth in this work.** ERPBridge's static bearer-token model is
   sufficient for the documented connections. OAuth metadata, dynamic client
   registration, and token refresh remain a separately scoped authentication
   feature.

## Scope

**In:** Stdio stdout safety; focused MCP wire-contract tests; agent setup and
troubleshooting documentation in both ERPBridge and `erpbridge-docs`; SDK
real-server compatibility verification.

**Out:** changes to `@erpbridge/sdk` APIs or dependencies; new MCP protocol
versions; OAuth; a legacy SSE endpoint; agent-specific server plugins; changes
to ERPBridge's role, token, tool-name, CORS, or result-envelope semantics.

## Tasks

- [x] **Task 1: Make the Stdio entrypoint protocol-safe.** Parse flags and
  resolve `MCP_TRANSPORT` before selecting the banner writer. In Stdio mode,
  route the banner to stderr; retain the existing `LOG_TO_STDERR` behavior and
  keep the `mcp-go` filtered writer as the sole stdout writer. Add a
  subprocess-level regression test that starts the built binary with
  `--stdio`, sends `initialize`, and asserts the first stdout line is a valid
  JSON-RPC response with no banner/log prefix. Also assert the banner appears
  on stderr. (**Seam:** actual `erpbridge-server --stdio` process and its
  stdin/stdout/stderr file descriptors; **Files:**
  `services/erpbridge-server/main.go`,
  `services/erpbridge-server/main_test.go`; **Verify:** write the failing
  subprocess test first, then `go test ./services/erpbridge-server`.)

- [x] **Task 2: Lock down the HTTP MCP compatibility contract.** Extend the
  Streamable HTTP tests with a table-driven initialize/list/call sequence for
  protocol versions `2025-03-26` and `2025-11-25`. Assert the server returns a
  session ID, accepts it on follow-up requests, preserves MCP result envelopes,
  and requires a bearer token on every protected request. Keep the existing
  CORS coverage for `Authorization`, `Mcp-Session-Id`, and
  `MCP-Protocol-Version`. (**Seam:** `Server.ServeHTTP` mounted on an
  `httptest` mux; **Files:** `internal/mcp/server_test.go`,
  `internal/mcp/auth_test.go`; **Verify:** write failing cases first, then
  `go test ./internal/mcp`.)

- [ ] **Task 3: Verify the server against the unchanged SDK contract.** Start
  ERPBridge from the built server with an isolated database path, then run the
  SDK's existing real-server protocol probe and integration suite against that
  instance. The expected result is no SDK source change: `mcp.connect()`,
  `listTools()`, and `callTool()` continue to use `/mcp/`, session negotiation,
  bearer headers, and unmodified `CallToolResult` envelopes. If this gate
  fails, stop and diagnose the server regression; do not change the SDK to
  accommodate a changed server contract. (**Seam:** public
  `@erpbridge/sdk` HTTP client; **Files:** no planned source changes in
  `../erpbridge-sdk`; **Verify:** in `../erpbridge-sdk`,
  `ERPBridge_TEST_SERVER=http://localhost:<port> npm run test:mcp-compat`,
  `ERPBridge_TEST_SERVER=http://localhost:<port> npm run test:integration`,
  `npm test`, and `npm run build`.)

Execution note: the MCP compatibility probe, SDK unit tests, and SDK build are
green with no SDK source changes. The seeded live suite otherwise passes, but
retains three existing expectations that are outside this plan: successful MCP
results omit optional isError false, and the log-stream timing case emits no
record in the isolated memory-mode run.

- [x] **Task 4: Add the ERPBridge source-of-truth agent guide.** Create a
  dedicated guide with a shared prerequisites/security section and one
  copy-paste configuration section per agent. Cover Codex TOML (`command` or
  `url` plus `bearer_token_env_var`), OpenCode JSONC (`mcp.servers`, local and
  remote forms with `{env:...}`), OpenClaw `mcp add`/`mcp set` with
  `transport: "streamable-http"` and `doctor --probe`, and Hermes YAML
  (`mcp_servers`, `url`, headers, `tools.include`). Include the exact `/mcp/`
  URL, health check, token scope, HTTPS guidance, Stdio credential boundary,
  agent reload/restart steps, and troubleshooting table. Link it from the
  connectivity guide and docs index; add an Unreleased changelog entry.
  (**Seam:** repository documentation rendered on GitHub and copied into the
  docs site; **Files:** `docs/agent-integrations.md`, `docs/connectivity.md`,
  `docs/README.md`, `CHANGELOG.md`; **Verify:** every snippet has a synthetic
  token only, names `/mcp/`, and agrees with Tasks 1–3 and the cited official
  agent configuration documentation.)

- [x] **Task 5: Mirror the agent guide into the public docs site.** Before
  editing, create or extend a matching documentation-repository plan as
  required by `erpbridge-docs/AGENTS.md`. Add the MDX guide, link it from the
  ERPBridge introduction and connectivity pages, and retain the same
  transport/auth/security wording as the source guide. Since the server docs
  are the developer source of truth, do not introduce alternative connection
  semantics in the site. Add an Unreleased changelog entry in the docs
  repository. (**Seam:** generated ERPBridge section in Docusaurus;
  **Files:** `../erpbridge-docs/docs/erpbridge/agent-integrations.mdx`,
  `../erpbridge-docs/docs/erpbridge/intro.mdx`,
  `../erpbridge-docs/docs/erpbridge/connectivity.mdx`,
  `../erpbridge-docs/CHANGELOG.md`, and that repository's plan file;
  **Verify:** in `../erpbridge-docs`, `npm run build` and review generated
  navigation/links.)

- [ ] **Task 6: Complete cross-repository release checks.** Run focused tests
  after each code task, then the ERPBridge full suite after all server changes.
  Confirm both repositories have clean worktrees and make one Conventional
  Commit per completed task: server fix, server contract tests, ERPBridge docs,
  and docs-site mirror. The SDK is verification-only unless Task 3 finds an
  actual regression. (**Seam:** repository CI-quality gates; **Files:** the
  changed files from Tasks 1, 2, 4, and 5; **Verify:** `make test` in
  ERPBridge, the Task 3 SDK commands, and `npm run build` in erpbridge-docs.)

## Verification

- A Stdio `initialize` request receives JSON-RPC as the first stdout data; the
  banner and logs are only on stderr.
- Both MCP protocol versions complete initialize, session-bound `tools/list`,
  and `tools/call` over `/mcp/` with the same response shapes as before.
- Protected HTTP MCP requests continue to require the existing bearer token
  and `mcp` scope; CORS still admits the headers used by browser-capable MCP
  clients.
- The unmodified SDK real-server suite passes, proving its current HTTP
  transport, authentication, reconnect, and result-envelope behavior remains
  intact.
- ERPBridge and the public documentation site each build successfully and
  expose matching, credential-safe instructions for all four agents.

## Open Questions

None. This plan deliberately does not broaden authentication beyond the
existing bearer-token contract.
