# Maintain ERPBridge with bridgectl

## Inspect before changing

Run the read-only [onboarding preflight](onboarding.md#preflight) first. Use
`bridgectl api list`, `bridgectl tool get`, and
`bridgectl tool describe <name>` with the same explicit `--context` to identify
the current resource and version. Read command help or generated CLI docs for
the installed release before using flags. Context-scoped API registries live
under `~/.bridgectl/registries/`; a legacy global registry is a stop for the
explicit scrub/migration workflow, not data to ignore. Changing the saved
context is a separate confirmed action.

## Tool and API lifecycle

- Test a registered API before changing a dependent tool. The default
  `bridgectl api test` is the authenticated server-side, body-free probe;
  `--local` is only an explicit host-side offline diagnostic.
- Update a manifest as a new reviewed version, validate it locally, then apply
  it only after confirmation. Read the registry back after apply. Keep the
  reviewed source under `manifests/<module>/`; generated YAML is a temporary
  draft and does not create a parallel `schemas/` or per-tool JSON authority.
- A quoted Compose environment change needs `--env-file .env` and
  `--force-recreate`; never source `.env`. Confirm health after recreation.
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

`bridgectl token create`, `list`, and `revoke` require the admin credential.
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

## Logs and cache

Start with `bridgectl log stats` and bounded `bridgectl log tail` filters to
identify a time window, component, tool, request ID, or error class. Treat log
output as sensitive evidence and redact it before sharing.

Run `bridgectl cache stats` before any invalidation. Confirm the narrowest
target—tool, module, or all entries—before `bridgectl cache flush`. For an
all-cache flush, repeat the environment and expected blast radius in the
confirmation, then verify the post-flush statistic or the affected tool call.
