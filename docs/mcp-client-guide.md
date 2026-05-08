# MCP Client Guide (Python & TypeScript)

This guide shows how to build MCP clients for ERPBridge using **Streamable HTTP** or **Stdio**, with complete examples in Python and TypeScript.

## 1. Prerequisites

1. **Run ERPBridge**
   - **Docker (recommended):**
     ```bash
     docker compose up -d --build
     ```
   - **Local HTTP server:**
     ```bash
     go run services/erpbridge-server/main.go
     ```
   - **Local Stdio server:**
     ```bash
     go run services/erpbridge-server/main.go --stdio
     ```

2. **Know your base URL**
   - Default Streamable HTTP URL: `http://localhost:8080/mcp/`
   - Change with `MCP_PORT` or `BASE_URL` environment variables.

## 2. Choose a Transport

| Transport | Best For | Notes |
| --- | --- | --- |
| **Streamable HTTP** | Web apps, Postman, services | JSON-RPC over HTTP + optional SSE notifications. |
| **Stdio** | Local integrations | Spawn the server and communicate over stdin/stdout. |

## 3. Streamable HTTP Protocol (Overview)

All MCP requests use JSON-RPC 2.0 and are sent to `POST /mcp/`.

1. **Initialize**
   ```json
   {
     "jsonrpc": "2.0",
     "id": 1,
     "method": "initialize",
     "params": {
       "protocolVersion": "2024-11-05",
       "capabilities": {},
       "clientInfo": { "name": "my-client", "version": "0.1.0" }
     }
   }
   ```
   - Save the `Mcp-Session-Id` response header for later calls.

2. **Open notifications (optional)**
   - `GET /mcp/` with `Accept: text/event-stream` and `Mcp-Session-Id`.
   - Notifications you may see: `notifications/progress`, `notifications/message`, `notifications/alert`, `notifications/tools/list_changed`.

3. **List tools**
   ```json
   { "jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {} }
   ```

4. **Call a tool**
   ```json
   {
     "jsonrpc": "2.0",
     "id": 3,
     "method": "tools/call",
     "params": {
       "name": "finance.list_invoices_api_v1_finance_invoices_get",
       "arguments": {}
     }
   }
   ```

## 4. Python Client (Streamable HTTP)

```python
import json
import requests

BASE_URL = "http://localhost:8080/mcp/"

init_payload = {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {"name": "python-client", "version": "0.1.0"},
    },
}

session = requests.Session()
init_resp = session.post(BASE_URL, json=init_payload)
init_resp.raise_for_status()
session_id = init_resp.headers.get("Mcp-Session-Id")

headers = {"Mcp-Session-Id": session_id}

tools_resp = session.post(
    BASE_URL,
    headers=headers,
    json={"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}},
)
tools = tools_resp.json()

call_resp = session.post(
    BASE_URL,
    headers=headers,
    json={
        "jsonrpc": "2.0",
        "id": 3,
        "method": "tools/call",
        "params": {
            "name": "finance.list_invoices_api_v1_finance_invoices_get",
            "arguments": {},
        },
    },
)
result = call_resp.json()
print(json.dumps(result, indent=2))
```

**Optional: Notifications (SSE)**
```python
with session.get(
    BASE_URL,
    headers={"Accept": "text/event-stream", "Mcp-Session-Id": session_id},
    stream=True,
) as resp:
    for line in resp.iter_lines():
        if line:
            print(line.decode("utf-8"))
```

## 5. Python Client (Stdio)

```python
import json
import subprocess

proc = subprocess.Popen(
    ["go", "run", "services/erpbridge-server/main.go", "--stdio"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    text=True,
)

def send(msg):
    proc.stdin.write(json.dumps(msg) + "\n")
    proc.stdin.flush()

def read():
    return json.loads(proc.stdout.readline())

send({
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {"name": "python-stdio", "version": "0.1.0"},
    },
})

init_response = read()
print(init_response)
```

## 6. TypeScript Client (Streamable HTTP)

```ts
const baseUrl = "http://localhost:8080/mcp/";

const initPayload = {
  jsonrpc: "2.0",
  id: 1,
  method: "initialize",
  params: {
    protocolVersion: "2024-11-05",
    capabilities: {},
    clientInfo: { name: "ts-client", version: "0.1.0" },
  },
};

const initResp = await fetch(baseUrl, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(initPayload),
});

const sessionId = initResp.headers.get("Mcp-Session-Id");
const headers = {
  "Content-Type": "application/json",
  "Mcp-Session-Id": sessionId ?? "",
};

const toolsResp = await fetch(baseUrl, {
  method: "POST",
  headers,
  body: JSON.stringify({ jsonrpc: "2.0", id: 2, method: "tools/list", params: {} }),
});
const tools = await toolsResp.json();

const callResp = await fetch(baseUrl, {
  method: "POST",
  headers,
  body: JSON.stringify({
    jsonrpc: "2.0",
    id: 3,
    method: "tools/call",
    params: {
      name: "finance.list_invoices_api_v1_finance_invoices_get",
      arguments: {},
    },
  }),
});
const result = await callResp.json();
console.log(result);
```

**Optional: Notifications (SSE)**
```ts
const streamResp = await fetch(baseUrl, {
  headers: { Accept: "text/event-stream", "Mcp-Session-Id": sessionId ?? "" },
});
const reader = streamResp.body?.getReader();
```

## 7. TypeScript Client (Stdio)

```ts
import { spawn } from "node:child_process";

const proc = spawn("go", ["run", "services/erpbridge-server/main.go", "--stdio"], {
  stdio: ["pipe", "pipe", "inherit"],
});

proc.stdin.write(
  JSON.stringify({
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2024-11-05",
      capabilities: {},
      clientInfo: { name: "ts-stdio", version: "0.1.0" },
    },
  }) + "\n",
);

proc.stdout.on("data", (data) => {
  const lines = data.toString("utf-8").trim().split("\n");
  for (const line of lines) {
    console.log(JSON.parse(line));
  }
});
```

## 8. Troubleshooting

- **Missing `Mcp-Session-Id`:** Ensure the first request is `initialize` and that you’re using `POST /mcp/`.
- **404 or connection errors:** Confirm `MCP_PORT`/`BASE_URL` and that the server is running.
- **Tools not found:** Call `tools/list` and use the exact `name` field for `tools/call`.
- **No notifications:** The SSE stream is optional; you’ll only receive events for logs, progress, or tool list changes.
