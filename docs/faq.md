# Frequently Asked Questions

## Configuration

### How do I configure the server?

Set the environment variables listed in the [Environment Variables Reference](./environment-variables.md). Export them in your shell, or set them in `docker-compose.yml`.

### How do I configure the CLI?

The CLI reads `~/.bridgectl/config.yaml`. The file holds named contexts. Each context has `server`, `mcp-server`, `erp-base`, and an optional `api-token` value.

For HTTP authentication, the CLI uses the `--token` flag first, then
`BRIDGE_API_TOKEN`, and then the active context's `api-token`.

To switch contexts:

```bash
bridgectl context list
bridgectl context set <name>
```

To override the address for one command:

```bash
BRIDGE_MCP_SERVER=http://localhost:8080 bridgectl tool get
```

### How do I verify onboarding without changing my local registry?

Run `./scripts/test-onboarding.sh`. It uses a temporary CLI home, an isolated
Compose project, dynamic loopback ports, synthetic fixture data, and ephemeral
credentials. The script checks the server-side API probe, generation,
validation, apply, MCP discovery/call, duplicate protection, and invalid-root
recovery. It removes the temporary stack when it exits.

### Why do `bridgectl cache` and `bridgectl log` fail with "connection refused"?

The default context points `server` at `http://localhost:8082`. Nothing listens there. Set `BRIDGE_SERVER=http://localhost:8080`, or edit the `server` value in `~/.bridgectl/config.yaml`.

### How do I enable debug logging?

Set `LOG_LEVEL=debug` on the server. You can also set per-component levels, for example `LOG_LEVEL_MCP=debug`.

### How do I run the server in JSON log mode?

Set `APP_ENV=production`.

### Do I need Redis?

No. The cache uses a bounded in-memory LRU when `REDIS_URL` is empty. Set `CACHE_MEMORY_MAX_ENTRIES=0` to disable memory-cache storage. If `REDIS_URL` is set, Redis remains the selected backend even when it is unreachable; the server reports the backend error instead of silently using memory.

## Tools & Schemas

### Where do tool schemas come from?

`make generate-tools` creates one bounded YAML draft stream, applies it once,
and removes its temporary files. It does not create per-tool JSON files or a
second generated directory.

Reviewed manifests are the single source of truth. Keep them under
`manifests/<module>/` and apply the reviewed file:

```bash
make generate-tools
bridgectl tool apply -f manifests/erp/tools.yaml
```

For a review-only draft, use `bridgectl tool generate --api <name> -o yaml`
and save its stdout explicitly to a temporary file. Remove the file after use.

### How do I update a tool?

Apply the new version with `bridgectl tool apply -f <file>`. The registry keeps multiple versions. MCP clients receive the latest stable version.

### How do I hide a tool without deleting it?

Soft-delete it:

```bash
bridgectl tool delete <name> <version>
```

The tool stays in the database. It no longer appears in `tools/list`.

### How do I restore a soft-deleted tool?

Apply the reviewed manifest again:

```bash
bridgectl tool apply -f manifests/erp/tools.yaml
```

### How do I permanently remove a tool?

```bash
bridgectl tool delete <name> <version> --hard
```

CAUTION: A hard delete removes the tool from the database. You cannot restore it.

### How quickly does the server pick up tool changes?

Within 10 seconds. The reconciliation controller ticks every 10 seconds. A tool applied over the API is visible immediately.

## Authentication

### How does the server authenticate ERP calls?

Tool schemas reference a credential by name (`credentialRef`). The server resolves the reference from environment variables at call time. Schemas never contain raw secrets.

MockERP accepts the configured ERPNext token header, session cookie, or Basic
Auth identity. Credential values are supplied through environment variables or
an external secret file; they are not documented or stored in schemas.

See the [MockERP Integration Contract](./mock-erp.md) for details.

## Upgrades

### How do I upgrade ERPBridge?

1. Pull the latest changes: `git pull`
2. Rebuild the binaries: `make build`
3. Restart the containers: `docker compose up -d --build`

The SQLite registry keeps its data when the migration succeeds. If the server
cannot initialize the registry, inspect the startup error and correct the
database path or permissions before you restart the server.

### Where do I find release notes?

See [CHANGELOG.md](../CHANGELOG.md).

## Troubleshooting

### Tool calls return `internal server error`

Check the server logs:

```bash
docker compose logs erpbridge-server
```

Common causes:

- The ERP service is unreachable. Make sure `ERP_BASE_URL` uses `http://mock-erp:8081` inside Docker.
- The tool endpoint path contains a secret pattern and was rejected.
- The selected Redis or in-memory cache backend is unavailable. Inspect the server logs and run `bridgectl cache stats`.

### Where do I get help?

- Report bugs and request features: [GitHub Issues](https://github.com/nmdra/ERPBridge/issues)
- See [CONTRIBUTING.md](../CONTRIBUTING.md)
- See the [Troubleshooting section of the Onboarding Guide](./onboarding.md#troubleshooting)
