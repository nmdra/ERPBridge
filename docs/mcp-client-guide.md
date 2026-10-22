# MCP Client Guide (Python & TypeScript)

> **Who is this guide for?** This guide helps developers who are new to MCP (Model Context Protocol). Use it to build Python or TypeScript clients for ToolPlane through **Streamable HTTP** or **Stdio** transport. The examples show basic message exchange, not a complete production client.

---

## Table of Contents

1. [What is MCP?](#1-what-is-mcp)
2. [Prerequisites](#2-prerequisites)
3. [Choosing a Transport](#3-choosing-a-transport)
4. [How Streamable HTTP Works](#4-how-streamable-http-works)
5. [Python Client — Streamable HTTP](#5-python-client--streamable-http)
6. [Python Client — Stdio](#6-python-client--stdio)
7. [TypeScript Client — Streamable HTTP](#7-typescript-client--streamable-http)
8. [TypeScript Client — Stdio](#8-typescript-client--stdio)
9. [Working with Resources](#9-working-with-resources)
10. [Working with Prompts](#10-working-with-prompts)
11. [Completion & Suggestions](#11-completion--suggestions)
12. [Hot Reloading](#12-hot-reloading)
13. [Error Handling](#13-error-handling)
14. [Troubleshooting](#14-troubleshooting)
15. [Logging & Redaction](#15-logging--redaction)
16. [Quick Reference Cheat Sheet](#16-quick-reference-cheat-sheet)

---

## 1. What is MCP?

**MCP (Model Context Protocol)** is a standard protocol for AI and automation workflows. Clients use it to call tools, read resources, and use prompts exposed by a server.

In this guide, **ToolPlane** exposes business tools, such as invoice listing, over MCP. The client must:

1. **Connect** to the server and start a session.
2. **Discover** which tools, resources, and prompts are available.
3. **Interact** with them and receive results.

All communication uses the **JSON-RPC 2.0** message format.

---

## 2. Prerequisites

<a id="21-start-the-erpbridge-server"></a>

### 2.1 Start the ToolPlane Server

Before you write client code, start a ToolPlane server.

**Option A — Docker (recommended for beginners):**
```bash
docker compose up -d --build
```

**Option B — Run the HTTP server locally:**
```bash
go run services/erpbridge-server/main.go
```

**Option C — Run the Stdio server locally:**
```bash
go run services/erpbridge-server/main.go --stdio
```

> **Tip:** If you do not know which option to choose, use Docker. Compose manages the configured service dependencies.

### 2.2 Verify the Server is Running

In HTTP mode, the server listens at:

```
http://localhost:8080/mcp/
```

To change the listening port, set the `MCP_PORT` environment variable.
`BASE_URL` sets the server base URL used by the HTTP transport; it does not
change the listening address.

**Quick check — make sure that the server responds:**
```bash
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/mcp/ \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}'
```

With authentication disabled, a successful request returns `200`.
If `API_AUTH_TOKEN` is configured, supply the bearer credential with each
protected HTTP request. The examples below do not include authentication.

### 2.3 Install Language Dependencies

**Python:**
```bash
pip install requests
```

**TypeScript / Node.js:**
```bash
npm install node-fetch   # only needed for Node.js < 18; built-in fetch works in Node 18+
```

---

## 3. Choosing a Transport

MCP supports two ways to communicate with the server:

| Transport | How it works | Best for |
|---|---|---|
| **Streamable HTTP** | Send JSON-RPC messages over regular HTTP POST requests. Optionally receive server-sent events (SSE) for real-time notifications. | Web apps, services, API testing tools like Postman |
| **Stdio** | Spawn the server as a child process and communicate over its standard input/output (stdin/stdout). | Local scripts, CLI tools, desktop integrations |

**Not sure which to pick?** Start with **Streamable HTTP**. You can inspect requests in a browser or Postman.

---

## 4. How Streamable HTTP Works

Send **JSON-RPC 2.0** messages as `POST` requests to `/mcp/`. The typical flow is:

```
Client                          ERPBridge Server
  |                                    |
  |--- POST /mcp/ (initialize) ------->|
  |<-- 200 OK + Mcp-Session-Id --------|   ← Save this header!
  |                                    |
  |--- POST /mcp/ (tools/list) ------->|   ← Include session ID
  |<-- List of available tools --------|
  |                                    |
  |--- POST /mcp/ (resources/list) ---->|
  |<-- List of available resources ----|
  |                                    |
  |--- POST /mcp/ (tools/call) ------->|   ← Call a specific tool
  |<-- Tool result ------------------- |
  |                                    |
  |--- GET /mcp/ (SSE, optional) ----->|   ← Subscribe to notifications
  |<-- Server-sent events (ongoing) ---|
```

### 4.1 Step 1 — Initialize the Session

Send an `initialize` request first. Include the returned `Mcp-Session-Id`
header in all subsequent requests. For a complete MCP lifecycle, send
`notifications/initialized` after initialization. The minimal examples omit
that notification.

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "logging": {},
      "prompts": {},
      "resources": {},
      "tools": {}
    },
    "clientInfo": { "name": "my-client", "version": "0.1.0" }
  }
}
```

> **Why is the session ID important?** The server uses it to associate requests with a session. Subsequent calls fail without it.

---

## 5. Python Client — Streamable HTTP

### 5.1 Minimal Working Example

This example connects to ToolPlane, lists available tools, and calls one.

```python
import json
import requests

BASE_URL = "http://localhost:8080/mcp/"

# --- Step 1: Initialize the session ---
init_payload = {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {"tools": {}, "resources": {}, "prompts": {}},
        "clientInfo": {"name": "python-client", "version": "0.1.0"},
    },
}

session = requests.Session()
init_resp = session.post(BASE_URL, json=init_payload)
init_resp.raise_for_status()

# Save the session ID — required for all future requests
session_id = init_resp.headers.get("Mcp-Session-Id")
if not session_id:
    raise RuntimeError("Server did not return Mcp-Session-Id. Did initialize succeed?")

headers = {"Mcp-Session-Id": session_id}
print(f"Session started: {session_id}")

# --- Step 2: List available tools ---
tools_resp = session.post(
    BASE_URL,
    headers=headers,
    json={"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}},
)
tools_resp.raise_for_status()
tools = tools_resp.json()

print("Available tools:")
for tool in tools.get("result", {}).get("tools", []):
    print(f"  - {tool['name']}: {tool.get('description', 'No description')}")

# --- Step 3: Call a tool ---
call_resp = session.post(
    BASE_URL,
    headers=headers,
    json={
        "jsonrpc": "2.0",
        "id": 3,
        "method": "tools/call",
        "params": {
            "name": "list_purchase_invoices",
            "arguments": {},
        },
    },
)
call_resp.raise_for_status()
result = call_resp.json()

print("\nTool result:")
print(json.dumps(result, indent=2))
```

---

## 6. Python Client — Stdio

To communicate through standard input/output, run ToolPlane as a subprocess.

### 6.1 Minimal Working Example

```python
import json
import subprocess

# --- Start the server as a child process ---
proc = subprocess.Popen(
    ["go", "run", "services/erpbridge-server/main.go", "--stdio"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,  # Capture stderr so it doesn't pollute your output
    text=True,
)

def send(message: dict):
    """Send a JSON-RPC message to the server via stdin."""
    proc.stdin.write(json.dumps(message) + "\n")
    proc.stdin.flush()

def receive() -> dict:
    """Read one JSON-RPC response from the server via stdout."""
    line = proc.stdout.readline()
    if not line:
        raise RuntimeError("Server process closed unexpectedly.")
    return json.loads(line)

# --- Step 1: Initialize ---
send({
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {"tools": {}, "resources": {}, "prompts": {}},
        "clientInfo": {"name": "python-stdio", "version": "0.1.0"},
    },
})
init_response = receive()
print("Initialized:", json.dumps(init_response, indent=2))

# --- Step 2: List tools ---
send({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
tools_response = receive()
print("Tools:", json.dumps(tools_response, indent=2))

# --- Step 3: Call a tool ---
send({
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
        "name": "list_purchase_invoices",
        "arguments": {},
    },
})
result = receive()
print("Result:", json.dumps(result, indent=2))

# --- Always clean up ---
proc.stdin.close()
proc.wait()
```

---

## 7. TypeScript Client — Streamable HTTP

### 7.1 Minimal Working Example

```typescript
const BASE_URL = "http://localhost:8080/mcp/";

async function main() {
  // --- Step 1: Initialize the session ---
  const initResp = await fetch(BASE_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: 1,
      method: "initialize",
      params: {
        protocolVersion: "2024-11-05",
        capabilities: { tools: {}, resources: {}, prompts: {} },
        clientInfo: { name: "ts-client", version: "0.1.0" },
      },
    }),
  });

  if (!initResp.ok) {
    throw new Error(`Initialize failed: ${initResp.status} ${initResp.statusText}`);
  }

  // Save the session ID — required for all future requests
  const sessionId = initResp.headers.get("Mcp-Session-Id");
  if (!sessionId) {
    throw new Error("Server did not return Mcp-Session-Id. Did initialize succeed?");
  }

  const headers = {
    "Content-Type": "application/json",
    "Mcp-Session-Id": sessionId,
  };

  console.log(`Session started: ${sessionId}`);

  // --- Step 2: List available tools ---
  const toolsResp = await fetch(BASE_URL, {
    method: "POST",
    headers,
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: 2,
      method: "tools/list",
      params: {},
    }),
  });

  const tools = await toolsResp.json();
  console.log("Available tools:", JSON.stringify(tools, null, 2));

  // --- Step 3: Call a tool ---
  const callResp = await fetch(BASE_URL, {
    method: "POST",
    headers,
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: 3,
      method: "tools/call",
      params: {
        name: "list_purchase_invoices",
        arguments: {},
      },
    }),
  });

  const result = await callResp.json();
  console.log("Tool result:", JSON.stringify(result, null, 2));
}

main().catch(console.error);
```

---

## 8. TypeScript Client — Stdio

```typescript
import { spawn } from "node:child_process";
import * as readline from "node:readline";

// --- Start the server as a child process ---
const proc = spawn(
  "go",
  ["run", "services/erpbridge-server/main.go", "--stdio"],
  {
    stdio: ["pipe", "pipe", "inherit"], // inherit stderr so errors are visible
  }
);

// Use readline to read one line at a time from stdout
const rl = readline.createInterface({ input: proc.stdout });
const pendingReads: Array<(line: string) => void> = [];

rl.on("line", (line) => {
  const resolve = pendingReads.shift();
  if (resolve) resolve(line);
});

function send(message: object): void {
  proc.stdin.write(JSON.stringify(message) + "\n");
}

function receive(): Promise<object> {
  return new Promise((resolve) => {
    pendingReads.push((line) => resolve(JSON.parse(line)));
  });
}

async function main() {
  // --- Step 1: Initialize ---
  send({
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2024-11-05",
      capabilities: { tools: {}, resources: {}, prompts: {} },
      clientInfo: { name: "ts-stdio", version: "0.1.0" },
    },
  });

  const initResponse = await receive();
  console.log("Initialized:", JSON.stringify(initResponse, null, 2));

  // --- Step 2: List tools ---
  send({ jsonrpc: "2.0", id: 2, method: "tools/list", params: {} });
  const toolsResponse = await receive();
  console.log("Tools:", JSON.stringify(toolsResponse, null, 2));

  // --- Step 3: Call a tool ---
  send({
    jsonrpc: "2.0",
    id: 3,
    method: "tools/call",
    params: {
      name: "list_purchase_invoices",
      arguments: {},
    },
  });

  const result = await receive();
  console.log("Result:", JSON.stringify(result, null, 2));

  // --- Always clean up ---
  proc.stdin.end();
}

main().catch(console.error);
```

---

## 9. Working with Resources

Resources are read-only data sources, such as database records or documentation. An AI client can retrieve them.

### 9.1 List Resources
```json
{ "jsonrpc": "2.0", "id": 5, "method": "resources/list", "params": {} }
```

### 9.2 Read a Resource
```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "resources/read",
  "params": {
    "uri": "mcp://finance/invoices/INV-2024-001"
  }
}
```

---

## 10. Working with Prompts

Prompts are predefined instruction templates that help AI agents perform specific tasks.

### 10.1 List Prompts
```json
{ "jsonrpc": "2.0", "id": 7, "method": "prompts/list", "params": {} }
```

### 10.2 Get a Prompt
```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "method": "prompts/get",
  "params": {
    "name": "analyze-spending",
    "arguments": {
      "department": "Engineering"
    }
  }
}
```

---

## 11. Completion & Suggestions

ToolPlane registers completion providers for resource and prompt arguments.
The current providers return fixed sample suggestions. They do not query ERP
data or provide tool-argument completion.

### 11.1 Get Suggestions
```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "completion/complete",
  "params": {
    "ref": {
      "type": "ref/resource",
      "uri": "mcp://finance/invoices/"
    },
    "argument": {
      "name": "invoice_id",
      "value": "INV-"
    }
  }
}
```

---

<a id="12-hot-reloading"></a>

## 12. Hot Reloading & Lifecycle

ToolPlane supports **Hot Reloading** and dynamic tool lifecycle management.
If you apply or delete tool resources through `bridgectl`, the server updates
its registry and notifies active sessions. Changed executable content requires
a new tool version.

### 12.1 Standard Sync Notification
When tools are added or removed, the server sends a standard notification:
*   **Method**: `notifications/tools/list_changed`
*   **Action**: Re-call `tools/list` to get the updated set.

<a id="122-erpbridge-custom-notifications"></a>

### 12.2 ToolPlane Custom Notifications
ToolPlane also sends specific lifecycle events:

#### `notifications/tool_deleted`
Sent when a tool is deactivated (soft-deleted) from the registry.
```json
{
  "method": "notifications/tool_deleted",
  "params": {
    "name": "finance.get_invoice",
    "version": "1.0.0",
    "reason": "deregistered from registry"
  }
}
```

#### `notifications/message`
Used for system-wide alerts or administrative broadcasts.
```json
{
  "method": "notifications/message",
  "params": {
    "message": "Server will undergo maintenance in 5 minutes",
    "type": "system"
  }
}
```

---

## 13. Error Handling

JSON-RPC protocol errors appear in the response body. Make sure that the
response has no `error` field. Also handle HTTP transport errors, such as
authentication failures. Tool execution failures use `result.isError=true`
instead of a JSON-RPC error.

### 13.1 What a JSON-RPC Error Looks Like

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "error": {
    "code": -32601,
    "message": "Method not found",
    "data": "No tool named 'finance.bad_tool_name'"
  }
}
```

### 13.2 Common JSON-RPC Error Codes

| Code | Meaning | Fix |
|---|---|---|
| `-32700` | Parse error | Your request body contains invalid JSON |
| `-32600` | Invalid request | Missing `jsonrpc`, `id`, or `method` fields |
| `-32601` | Method not found | Tool name doesn't exist — check with `tools/list` |
| `-32602` | Invalid params | Wrong or missing arguments for the tool |
| `-32603` | Internal error | Server-side error — check server logs |

### 13.3 Preserve the MCP Result Envelope

An MCP `tools/call` response uses a JSON-RPC result envelope. Read the `content`
array. Preserve `structuredContent` and `isError` when present. For a declared
output schema and an object result, ToolPlane includes structured content.
Text content contains the JSON result, or the extracted text when the object
has a string `text` field. Do not treat `content[0].text` as the complete
protocol response.

The direct REST endpoint, `/api/tools/invoke`, is a registry-only compatibility endpoint and returns the legacy `ToolResult` shape. MCP built-in tools whose names start with `system.` are available through MCP, not through that REST endpoint.

---

## 14. Troubleshooting

### ❌ `Mcp-Session-Id` is missing from the response

**Cause:** Session initialization did not succeed.

**Fix:**
- Make sure that the first request uses `"method": "initialize"`.
- Make sure that requests use `POST /mcp/`, not `GET` or `/mcp` without the trailing slash.

---

### ❌ `404 Not Found` or connection refused

**Cause:** The server is not running, or the URL is incorrect.

**Fix:**
- Make sure that the server started successfully. Read Docker logs or terminal output.
- Make sure that the port is correct: `curl http://localhost:8080/mcp/`

---

## 15. Logging & Redaction

ToolPlane implements standard MCP logging via `notifications/message`.

### 15.1 Setting the Log Level
```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "method": "logging/setLevel",
  "params": { "level": "debug" }
}
```

Available levels: `debug`, `info`, `notice`, `warning`, `error`, `critical`, `alert`, `emergency`.

### 15.2 Automatic Redaction
For security, ToolPlane automatically redacts sensitive data (API keys, passwords, PII) from all logs sent to clients. Redacted fields appear as `[REDACTED]`.

---

## 16. Quick Reference Cheat Sheet

### JSON-RPC Message Template

```json
{
  "jsonrpc": "2.0",
  "id": <any unique integer>,
  "method": "<method name>",
  "params": {}
}
```

### Request Sequence (HTTP)

```
1. POST /mcp/  →  initialize          (no session ID needed)
2. POST /mcp/  →  tools/list          (include Mcp-Session-Id)
3. POST /mcp/  →  resources/list      (include Mcp-Session-Id)
4. POST /mcp/  →  prompts/list        (include Mcp-Session-Id)
5. POST /mcp/  →  tools/call          (include Mcp-Session-Id)
6. GET  /mcp/  →  SSE stream          (optional, include Mcp-Session-Id)
```

### Transport at a Glance

| | Streamable HTTP | Stdio |
|---|---|---|
| Session ID | Required (from `initialize` response header) | Not used |
| Notifications | Via SSE (GET stream) | Supported (via stdout) |
| Debugging | Easy (curl, Postman, browser devtools) | Harder |
| Best for | Services, web apps | Local scripts, CLIs |

### Key Headers

| Header | When to use |
|---|---|
| `Content-Type: application/json` | All POST requests |
| `Mcp-Session-Id: <id>` | All requests after `initialize` |
| `Accept: text/event-stream` | SSE / notifications GET request |
