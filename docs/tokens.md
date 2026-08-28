# API Token Guide

ERPBridge supports opaque bearer API tokens for MCP clients, metrics
scrapers, and log consumers. Token management is available only to the admin
credential configured with `API_AUTH_TOKEN`.

## Enable HTTP authentication

Set a non-empty admin credential before starting the HTTP server:

```bash
export API_AUTH_TOKEN='<set-in-environment>'
export API_AUTH_ADMIN_ROLES='finance_reader,finance_admin'
erpbridge-server
```

Send the credential as a bearer token:

```bash
curl -H 'Authorization: Bearer ${API_AUTH_TOKEN}' \
  http://localhost:8080/api/auth/tokens
```

The admin identity has implicit access to every authenticated route. The
optional `API_AUTH_ADMIN_ROLES` value gives that identity verified roles.

## Create a token

```bash
curl -X POST http://localhost:8080/api/auth/tokens \
  -H 'Authorization: Bearer ${API_AUTH_TOKEN}' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "finance-agent",
    "scopes": ["mcp"],
    "roles": ["finance_reader"],
    "expiresAt": "2030-01-01T00:00:00Z"
  }'
```

The response contains the raw token once. It starts with `erpbt_`. Store it
in a secret manager. The database stores only a SHA-256 hash, and list or
lookup operations never return the raw token or its hash.

Supported scopes are `mcp`, `metrics`, and `logs`. Roles are optional and must
match `[a-z][a-z0-9_-]{0,63}`. A token can contain at most 32 unique roles.

## List and revoke tokens

```bash
curl -H 'Authorization: Bearer ${API_AUTH_TOKEN}' \
  http://localhost:8080/api/auth/tokens

curl -X DELETE \
  -H 'Authorization: Bearer ${API_AUTH_TOKEN}' \
  http://localhost:8080/api/auth/tokens/<id>
```

Revocation is immediate. Expired and revoked tokens receive `401`; valid
tokens without the required scope receive `403`.

## Use a token

```bash
curl -H 'Authorization: Bearer <token-from-environment>' \
  -H 'Content-Type: application/json' \
  http://localhost:8080/mcp/
```

The `mcp` scope is required for `/mcp/`, `metrics` for `/metrics`, and `logs`
for `/api/logs/recent` and `/api/logs/stream`. Registry, direct invoke, cache,
and token lifecycle routes require the admin credential.

Use `bridgectl --token`, `BRIDGE_API_TOKEN`, or the active context `api-token`
to authenticate CLI requests. The precedence is flag, environment, then
context.

## Role-protected tools

A tool with `spec.security.allowedRoles` is guarded. MCP clients pass the
selected role as `arguments.role`; direct callers use `X-ERPBridge-Role`.
The selected role must be present in both the token identity and the tool
allow-list. The server removes the MCP selector before the ERP call.

During MCP `tools/list`, a guarded tool also publishes its non-empty role
allow-list as `io.erpbridge/allowedRoles` in namespaced `_meta`. This is
informational discovery metadata for the host or model. It does not grant
permission, and custom `_meta` may not be forwarded into the model context by
every MCP client. The server's authenticated identity and authorization check
remain authoritative.

Classify sensitive tools with `spec.security.dataClass: pii` or `restricted`.
These classes require one or more opaque, verified role names. Use names such
as `role_alpha`, not employee names, email addresses, or other personal data.
Authorization runs before cache lookup and ERP work. A missing or unauthorized
role cannot read a cached result. `dataClass` is optional for older manifests;
when present it must be `public`, `internal`, `pii`, or `restricted`. ERPBridge
does not infer sensitivity from arbitrary ERP response payloads.

The bundled `system.progress_test` and `system.sensitive_log_test` tools are
development demonstrations. They are absent from discovery unless
`MCP_ENABLE_TEST_TOOLS=true`; only the development Compose stack sets this
flag. Do not enable it for a production server.

Open tools do not reserve `role`, so existing business arguments remain
unchanged. Guarded tools deny calls without a verified identity, including
calls over stdio. Denied calls run before cache lookup and downstream ERP
execution.
