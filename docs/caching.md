# Exact Match Caching

ERPBridge has a caching layer that reduces latency and load on legacy ERP systems. It uses Redis when configured, or a bounded in-memory LRU when Redis is not configured.

## 🚀 Overview

The caching mechanism runs as middleware in the MCP tool execution pipeline. The server selects Redis when `REDIS_URL` is set and the in-memory backend otherwise. A configured but unreachable Redis server is not replaced by memory.

### Layer 1: Exact Match (SHA256)

The system gives O(1) lookups for identical requests.

- **Key Generation**: A deterministic hash of the tool name, the user role scope, and the JSON-sorted arguments.
- **TTL**: Configurable per tool. Defaults to `0` (no expiry) when `ttlSeconds` is omitted.
- **Benefit**: Fast response times for repetitive queries.

## ⚙️ Configuration

Caching is opt-in. It is configured per tool in the `spec.cache` section of the tool schema.

### Example Schema Configuration

```json
{
  "apiVersion": "erpbridge.io/v1",
  "kind": "MCPTool",
  "metadata": {
    "name": "list_employees",
    "version": "1.0.0",
    "module": "hr"
  },
  "spec": {
    "cache": {
      "enabled": true,
      "ttlSeconds": 3600,
      "isReadOnly": true,
      "flushOn": []
    }
  }
}
```

| Field | Description |
| :--- | :--- |
| `enabled` | Enables or disables the cache middleware for this tool. |
| `ttlSeconds` | How long the entry stays in the selected cache backend. A value of `0` means no expiry. |
| `isReadOnly` | If `true`, the cache uses the shared `role: shared` scope. If `false`, entries use the verified caller role. |
| `flushOn` | An array of tool names. When the current tool runs, it flushes the cache of the listed tools. |

## 🧹 Cache Invalidation (Auto-Flush)

ERPBridge supports automatic cache invalidation. Use it on `POST`, `PUT`, or `PATCH` tools.

**Example: Invalidating "Get Invoices" when a new one is created.**

```json
"spec": {
  "cache": {
    "enabled": false,
    "flushOn": ["get_invoices"]
  }
}
```

## 🛠 Management with `bridgectl`

The developer CLI provides tools to monitor and manage the cache.

### Check Cache Statistics

Shows counts of cached keys and memory usage.

```bash
bridgectl cache stats
```

### Flush Cache

Clears the cache for a specific tool or an entire module.

```bash
# Flush specific tool (positional argument)
bridgectl cache flush list_employees

# Flush entire module
bridgectl cache flush --module erp

# Flush everything
bridgectl cache flush --all
```

## 🔎 When Redis Is Not Configured

If `REDIS_URL` is not set, the server uses the bounded in-memory cache. Set `CACHE_MEMORY_MAX_ENTRIES=0` to disable memory-cache storage while keeping tool calls available. The default capacity is 10,000 entries.

The cache endpoints work with both backends:

- `GET /api/cache/stats`
- `GET /api/cache/flush`

## Credential rotation and cache safety

Environment-backed tools keep the normal cache behavior. A tool with
`security.credentialSource: file` bypasses cache reads and writes. A tool with
an active authenticated file-backed plugin binding also bypasses the cache.
This prevents a response produced with an earlier file value from being served
after rotation. Cache keys never contain credential values, file contents, or
file metadata.

File resolution happens immediately before the outbound operation. A complete
file replacement observed by ERPBridge is used by the next request; missing,
empty, invalid, control-character, non-regular, or oversized files fail before
transport. Provider-managed CSI updates are eventual and are outside cache
coordination. Multiple replicas can observe different versions temporarily.

## 🏗 System Architecture

1. **ERPBridge Server**: Orchestrates the middleware and selects the configured backend.
2. **Redis or MemoryBackend**: Stores the hashes and responses for high-speed retrieval.

For deployment details, see the [Docker Guide](./docker.md).
