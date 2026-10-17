# Maintain ERPBridge with bridgectl

| Task | Section |
| --- | --- |
| Select context and inspect | [Inspect before changing](#inspect-before-changing) |
| Update or retire a resource | [Tool and API lifecycle](#tool-and-api-lifecycle) |
| Plugin lifecycle | [External plugins](#external-plugins) |
| Identity and roles | [Tokens and guarded tools](#tokens-and-guarded-tools) |
| Read logs or invalidate cache | [Logs and cache](#logs-and-cache) |

## Inspect before changing

Run [core preflight and only the relevant profiles](preflight.md) first.
For remote reads or narrow administration, local Docker, localhost health,
MockERP, RedisInsight, and API probes are not prerequisites. Use the narrowest
read for the task, such as `bridgectl api list --context "$CTX"`,
`bridgectl tool get --context "$CTX"`, or
`bridgectl tool describe <name> --context "$CTX"`, to identify the target.
Read command help or generated CLI docs for the installed release before
using flags. Context-scoped API registries live
under `~/.bridgectl/registries/`; a legacy global registry is a stop for the
explicit scrub/migration workflow, not data to ignore. Changing the saved
context is a separate confirmed action.

## Tool and API lifecycle

- When a dependent tool change needs ERP connectivity evidence, obtain exact
  approval before `bridgectl api test <name> --context "$CTX"`. Its bounded,
  server-side result does not make it read-only: the request can cause downstream
  effects. Management MCP also needs the `probe` grant; see [MCP](mcp.md).
  `--local` is only a separately approved host-side diagnostic.
- Update a manifest as a new reviewed version, validate it locally, then apply
  it only after confirmation. Read the registry back after apply. Keep the
  reviewed source under `manifests/<module>/`; generated YAML is a temporary
  draft and does not create a parallel `schemas/` or per-tool JSON authority.
- For local Compose only, a quoted environment change needs `--env-file .env` and
  `--force-recreate`; never source `.env`. Confirm health after recreation.
  Environment-backed credential changes need the refreshed process. A
  `credentialSource: file` resource reads its reference-named file from
  `ERPBRIDGE_CREDENTIALS_DIR` immediately before the operation, fails closed on
  invalid content, and bypasses the response cache; provider-mounted updates
  are eventually visible.
- Treat deletion as user-visible impact. Confirm the exact `name@version`
  before deletion. A hard delete permanently removes database state and needs
  an explicit hard-delete confirmation even when `--yes` is available.
- Use deactivation/soft deletion where retention for audit or rollback is
  needed. Confirm that MCP discovery no longer exposes the retired tool.

## External plugins

Read [Plugins](plugins.md) before managing an external plugin or binding. Apply
and read back the exact plugin version before applying a binding that references
that plugin and an exact tool version. A normal delete is a soft delete; a hard
delete needs explicit confirmation and an active binding blocks hard deletion
of its exact plugin version. Applying, updating, or deleting a plugin or
binding flushes affected tool cache entries, so verify the narrowest affected
cache target after the lifecycle change.

## Tokens and guarded tools

`bridgectl token create --name <name> --context "$CTX"`,
`bridgectl token list --context "$CTX"`, and
`bridgectl token revoke <id> --context "$CTX"` require the admin credential.
Confirm each creation or revocation for its exact target and scope, then
independently read token metadata back. Token creation stays in CLI, not MCP.
Create the smallest set of scopes (`mcp`, `metrics`, or `logs`) and roles that
the caller needs. A created token is disclosed once; direct the recipient to a
secret manager and omit the value from output thereafter.

A tool with `spec.security.allowedRoles` needs a selected role that appears in
both the caller identity and the tool allow-list. `pii` and `restricted`
`dataClass` values require at least one configured role. Use existing opaque,
non-identifying role slugs; do not use a person's name, email, employee number,
or ERP record as a role or example. Verify an authorized call and a denial.
Never use an administrative identity to mask a caller-role failure.

The development-only `system.*_test` tools require `MCP_ENABLE_TEST_TOOLS=true`
and must be absent from production discovery. RedisInsight is a local
inspection aid: require a loopback binding or an explicit opt-in, and never
use it as production evidence.

MCP annotations and namespaced `_meta` values help clients select and explain
published tools, but they are optional and informational. Check the authenticated
identity and server-side role admission instead of treating discovery metadata
as permission.

## Logs and cache

Start with `bridgectl log stats --context "$CTX"` and bounded
`bridgectl log tail --context "$CTX"` filters to
identify a time window, component, tool, request ID, or error class. Treat log
output as sensitive evidence and redact it before sharing.

Run `bridgectl cache stats --context "$CTX"` before any invalidation. Confirm
the exact narrowest target (tool, module, or all entries) before
`bridgectl cache flush --context "$CTX"` with its approved selector. For an
all-cache flush, repeat the environment and expected blast radius in the
confirmation, then independently verify the post-flush statistic. Use an
explicitly approved safe tool call only if runtime behavior needs verification.
Invalidate affected [session evidence](preflight.md#session-evidence) after the
change. Follow the shared [verification gates](preflight.md#action-and-verification-gates)
for every lifecycle action; command completion alone is not proof of state.
File-backed tools and active authenticated file-backed plugin bindings bypass
response caching, so a file replacement does not need a cache flush merely to
be observed. Environment-backed rotation keeps the normal cache behavior and
needs the deployment refresh and narrow cache invalidation required by the
changed credential.
