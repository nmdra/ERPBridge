# Agentic Tools MCP Integration

ERPBridge implements standard Model Context Protocol (MCP). Codex CLI,
OpenCode, OpenClaw, and Hermes Agent can connect without an ERPBridge-specific
adapter.

Use stdio when the agent and erpbridge-server run on the same machine. Use
Streamable HTTP when the server is remote, shared, containerized, or managed
separately from the agent.

The canonical remote endpoint is https://<erpbridge-host>/mcp/. The trailing
slash is part of the endpoint. The first request is a JSON-RPC initialize
request sent to POST /mcp/, not to a separate /mcp/initialize route. HTTP
sessions return Mcp-Session-Id for subsequent requests.

## Prerequisites and security

1. Start ERPBridge and confirm GET https://<erpbridge-host>/mcp/health returns
   {"status":"ok"}.
2. Register and activate the MCP tools that the agent should use.
3. For protected HTTP deployments, create a token with the mcp scope and
   expose it to the agent through its environment or secret store.
4. Use HTTPS for anything beyond a trusted local machine. Never commit a raw
   bearer token to an agent config.

ERPBridge accepts Authorization: Bearer <token> on protected HTTP MCP
requests. The administrator credential has implicit access. A scoped token
needs the mcp scope. A token for metrics or logs alone cannot initialize MCP.

Example token provisioning:

    curl -X POST https://<erpbridge-host>/api/auth/tokens -H "Authorization: Bearer <admin-token>" -H "Content-Type: application/json" -d '{"name":"agent-mcp","scopes":["mcp"]}'

The raw token is returned once. Keep it outside shell history and config files,
then expose it to the agent host:

    export ERPBRIDGE_MCP_TOKEN='<token-from-secure-provisioning>'

API_AUTH_TOKEN controls inbound HTTP authentication. ERP_PRIMARY_KEY and
credentialRef values are server-side settings for outbound ERP calls. Never
copy an upstream ERP credential into an agent config.

## Codex CLI

Codex stores MCP servers in ~/.codex/config.toml or a trusted project-local
.codex/config.toml. It supports stdio and Streamable HTTP.

### Local stdio

    [mcp_servers.erpbridge]
    command = "erpbridge-server"
    args = ["--stdio"]
    env_vars = ["ERP_PRIMARY_KEY"]
    enabled_tools = ["list_employees", "list_departments"]

Stdio has no inbound HTTP bearer-auth exchange. The child process is the
security boundary. API_AUTH_TOKEN does not authenticate a stdio client; the
server process still needs its upstream ERP credentials.

### Remote Streamable HTTP

    [mcp_servers.erpbridge]
    url = "https://<erpbridge-host>/mcp/"
    bearer_token_env_var = "ERPBRIDGE_MCP_TOKEN"
    enabled_tools = ["list_employees", "list_departments"]

bearer_token_env_var sends the environment value as a bearer token. Avoid
static Authorization headers for secrets that belong in the environment.
Restart Codex after changing config.toml. Use codex mcp list and /mcp to
inspect the connection.

## OpenCode

OpenCode uses mcp.servers in opencode.json or opencode.jsonc. Its local form
starts a child process and its remote form uses Streamable HTTP.

### Local stdio

    {
      "$schema": "https://opencode.ai/config.json",
      "mcp": {
        "servers": {
          "erpbridge": {
            "type": "local",
            "command": ["erpbridge-server", "--stdio"],
            "environment": {
              "ERP_PRIMARY_KEY": "{env:ERP_PRIMARY_KEY}"
            }
          }
        }
      }
    }

### Remote Streamable HTTP

    {
      "$schema": "https://opencode.ai/config.json",
      "mcp": {
        "servers": {
          "erpbridge": {
            "type": "remote",
            "url": "https://<erpbridge-host>/mcp/",
            "oauth": false,
            "headers": {
              "Authorization": "Bearer {env:ERPBRIDGE_MCP_TOKEN}"
            }
          }
        }
      }
    }

The {env:NAME} substitution keeps the token out of JSONC. OpenCode does not
document a server-native tool allowlist in this config shape; restrict
exposure in the ERPBridge registry and use approval controls for writes.
Run opencode mcp list after editing and restart the OpenCode session if
needed.

## OpenClaw

OpenClaw manages client-side MCP definitions with openclaw mcp. Use the
canonical streamable-http transport value, not the legacy SSE default.

### Remote Streamable HTTP

Set ERPBRIDGE_MCP_TOKEN in the OpenClaw gateway environment, then save a
definition whose header references that environment value:

    openclaw mcp set erpbridge '{"url":"https://<erpbridge-host>/mcp/","transport":"streamable-http","headers":{"Authorization":"Bearer ${ERPBRIDGE_MCP_TOKEN}"},"toolFilter":{"include":["list_employees","list_departments"]}}'
    openclaw mcp doctor erpbridge --probe

OpenClaw warns about literal sensitive headers. Keep the environment variable
in the gateway trusted environment or supported secret configuration, not in a
workspace file. On versions that do not expand environment variables in HTTP
header values, use that version's secret-reference mechanism. Do not replace
the placeholder with a committed token. OAuth is not appropriate for
ERPBridge's static bearer-token mode and takes precedence over a static
Authorization header.

Use openclaw mcp show erpbridge --json to inspect the saved definition without
exposing the token. Run openclaw mcp reload or restart the owning gateway
after changing the definition. doctor --probe performs live initialize and
tool discovery.

### Local stdio

    openclaw mcp add erpbridge-local --command erpbridge-server --arg --stdio --env ERP_PRIMARY_KEY=$ERP_PRIMARY_KEY
    openclaw mcp doctor erpbridge-local --probe

For stdio, ERP_PRIMARY_KEY is an upstream ERP credential passed to the server
process; it is not an inbound MCP bearer token.

## Hermes Agent

Hermes uses mcp_servers in its YAML config, commonly ~/.hermes/config.yaml.
Environment references can be stored in the active Hermes secret scope or
process environment.

### Remote Streamable HTTP

    mcp_servers:
      erpbridge:
        url: "https://<erpbridge-host>/mcp/"
        headers:
          Authorization: "Bearer ${ERPBRIDGE_MCP_TOKEN}"
        tools:
          include:
            - list_employees
            - list_departments
          resources: false
          prompts: false

Hermes sends the Authorization header on remote requests and supports
tools.include and tools.exclude filtering. Keep the token in ~/.hermes/.env
or another supported secret scope and reload MCP with /reload-mcp after
changing the config.

### Local stdio

    mcp_servers:
      erpbridge-local:
        command: "erpbridge-server"
        args: ["--stdio"]
        env:
          ERP_PRIMARY_KEY: "${ERP_PRIMARY_KEY}"
        tools:
          include: [list_employees, list_departments]

The stdio process receives the upstream ERP environment. HTTP bearer
authentication does not apply to this local process.

## Reload and troubleshoot

After changing a client configuration:

- Codex: run codex mcp list; restart the TUI or IDE extension if needed.
- OpenCode: run opencode mcp list; restart the OpenCode session.
- OpenClaw: run openclaw mcp doctor erpbridge --probe, then openclaw mcp
  reload or restart the gateway.
- Hermes: run /reload-mcp in the active session.

| Symptom | Check |
| --- | --- |
| 401 Unauthorized | The token is missing, expired, revoked, or unavailable to the agent process. Confirm Authorization: Bearer is sent and the token has mcp. |
| 403 Forbidden | The token lacks the mcp scope, or the operation is outside its role or tool policy. |
| HTTP client cannot connect | Use the exact https://<host>/mcp/ URL, verify GET /mcp/health, and check reverse-proxy forwarding of POST, GET, Authorization, Mcp-Session-Id, and MCP-Protocol-Version. |
| Tools list is empty or incomplete | Check that tools are active in the ERPBridge registry and review client-side allow or deny filters. |
| Stdio initialization fails with parse errors | Ensure the server is started with --stdio, the binary is executable, and no wrapper prints to stdout. ERPBridge writes startup diagnostics to stderr in stdio mode. |
| Upstream ERP calls fail | Check the server's ERP_BASE_URL and the environment variable named by each tool's credentialRef; these are server-side settings, not agent MCP settings. |

For protocol details and Postman, see [Connectivity & Transport](./connectivity.md).
For token lifecycle and scopes, see [API Tokens](./tokens.md).

## Official client references

- [Codex MCP configuration](https://developers.openai.com/codex/mcp/)
- [OpenCode MCP servers](https://opencode.ai/docs/mcp-servers)
- [OpenClaw MCP CLI](https://docs.openclaw.ai/cli/mcp)
- [Hermes MCP configuration](https://hermes-agent.nousresearch.com/docs/reference/mcp-config-reference/)
