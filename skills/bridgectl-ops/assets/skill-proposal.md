# Skill improvement proposal

## ID

`prop-<YYYY>-<short-id>`

## Target

- `skills/bridgectl-ops/<source-file>`

## Base source revision

`<git-revision>`

## Rollback plan

<Describe the version-controlled change to revert if the accepted patch causes
an error.>

## Problem

<Describe the repeated skill weakness or verified source conflict.>

## Supporting knowledge

- `<knowledge-id>`

## Supporting executions

- `<execution-id>`

## Current behavior

<Describe the current authoritative guidance.>

## Proposed change

<Describe one coherent change to repository source only.>

## Expected benefit

<Describe the safer or clearer behavior.>

## Safety impact

<Explain why preflight, authentication, authorization, redaction, validation,
and confirmation gates remain intact.>

## Version scope

- `bridgectl`: <constraint-or-unknown>
- `erpbridge_server`: <constraint-or-unknown>

## Validation plan

- <Focused skill/package validation>
- <Relevant CLI, server, security, and regression checks>
- <Installed-skill comparison after rebuilding, if applicable>

## Hard-gate result

Set every item to `false` before acceptance:

```yaml
secrets_exposed: false
authorization_headers_exposed: false
pii_exposed: false
erp_bodies_persisted: false
plugin_sensitive_payloads_persisted: false
mutation_without_confirmation: false
hard_delete_without_explicit_confirmation: false
all_cache_flush_without_explicit_confirmation: false
source_env_file: false
admin_identity_used_to_mask_role_failure: false
unvalidated_manifest_applied: false
installed_skill_modified_directly: false
memory_overrode_current_skill: false
```

## Result

`pending`

Record the decision in
`.agents/skill-memory/bridgectl-ops/evolution/skill-impact.jsonl`, including
rejected proposals and the reason for rejection.
