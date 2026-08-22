# Maintain ERPBridge with bridgectl

## Inspect before changing

Use `bridgectl api list`, `bridgectl tool get`, and
`bridgectl tool describe <name>` to identify the current resource and version.
Read command help or generated CLI docs for the installed release before using
flags. Use `--context` for a one-off target; changing the saved context is a
separate confirmed action.

## Tool and API lifecycle

- Test a registered API before changing a dependent tool.
- Update a manifest as a new reviewed version, validate it locally, then apply
  it only after confirmation. Read the registry back after apply.
- Treat deletion as user-visible impact. Confirm the exact `name@version`
  before deletion. A hard delete permanently removes database state and needs
  an explicit hard-delete confirmation even when `--yes` is available.
- Use deactivation/soft deletion where retention for audit or rollback is
  needed. Confirm that MCP discovery no longer exposes the retired tool.

## Tokens and guarded tools

`bridgectl token create`, `list`, and `revoke` require the admin credential.
Create the smallest set of scopes (`mcp`, `metrics`, or `logs`) and roles that
the caller needs. A created token is disclosed once; direct the recipient to a
secret manager and omit the value from output thereafter.

A tool with `spec.security.allowedRoles` needs a selected role that appears in
both the caller identity and the tool allow-list. Verify an authorized call and
a denial. Never use an administrative identity to mask a caller-role failure.

## Logs and cache

Start with `bridgectl log stats` and bounded `bridgectl log tail` filters to
identify a time window, component, tool, request ID, or error class. Treat log
output as sensitive evidence and redact it before sharing.

Run `bridgectl cache stats` before any invalidation. Confirm the narrowest
target—tool, module, or all entries—before `bridgectl cache flush`. For an
all-cache flush, repeat the environment and expected blast radius in the
confirmation, then verify the post-flush statistic or the affected tool call.
