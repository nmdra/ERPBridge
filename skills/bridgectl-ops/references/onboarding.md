# Onboard an ERP API as an MCP tool

| Task | Section |
| --- | --- |
| Establish deployment and action evidence | [Preflight](#preflight) |
| Register and publish | [Workflow](#workflow) |
| Resolve a stable error | [Recovery branches](#recovery-branches) |
| Verify and hand off | [Verification checklist](#verification-checklist) |

## Outcome

An ERP endpoint is registered locally, approved connectivity testing is
completed through the server, its tool schema is reviewed and validated, and
the approved manifest is visible in the selected ERPBridge context. Stop
before mutation when a required precondition is unknown.

## Preflight

Run [core preflight and the API onboarding profile](preflight.md) before this
workflow. Add the remote or local Compose profile for the selected deployment,
and the plugin-binding profile only when needed. Set `CTX` to the intended
configured context. A probe is an effectful action requiring exact approval,
not a read-only prerequisite for unrelated operations.

## Workflow

1. Confirm the endpoint owner, URL, method, module, purpose, expected inputs,
   effects, and credential source. Inspect installed CLI help and matching
   documentation for that release. Register only after preflight and
   exact-action confirmation. For a mounted source, preserve it in the local
   API registry:

   ```text
   bridgectl api register --context "$CTX" --name <api-name> \
     --url <erp-url> --method GET --module <module> \
     --description "<purpose>" --credential-ref <ERP_CREDENTIAL_REF> \
     --credential-source file
   ```

   Use `--force` only after confirming intentional replacement of the named API.
2. Obtain approval for the exact server-side test, including possible downstream
   effects, then run `bridgectl api test <name> --context "$CTX"`. Management
   MCP also needs its `probe` grant and confirmation fields; see [MCP](mcp.md).
   Stop on failure and use the recovery branches. Do not publish an unverified
   endpoint or retry the test without resolving the cause and confirming the
   next action. Use `--local` only for an explicitly approved host-side
   diagnostic; keep its response body out of evidence.
3. Generate a draft with
   `bridgectl tool generate --api <name> --context "$CTX"`; add
   `--openapi <path-or-url>` for multiple operations. Remote OpenAPI fetching
   and file output use CLI, not management MCP. For one file per tool, add
   `--output-dir <draft-dir>` and `-o json` or `-o yaml`. Filenames are exact
   tool names; the directory is created as needed and progress goes to stderr.
   Without `--output-dir`, the normal stdout contract is unchanged. Review and
   validate each draft before moving approved content to `manifests/<module>/`.
   `make generate-tools` applies one temporary YAML stream once. Use it only
   after verifying how it selects the intended context and confirming that
   exact apply; otherwise use explicit-context CLI commands.
4. Complete intent metadata, input/output schemas, execution path,
   `credentialRef`, optional `credentialSource`, cache policy,
   `security.dataClass`, and optional `allowedRoles`. Review method-generated
   annotations as drafts: `title`, `readOnlyHint`, `destructiveHint`,
   `idempotentHint`, and `openWorldHint` are hints, not authorization.
   Discovery may expose intent and roles through `io.erpbridge/whenToUse`,
   `io.erpbridge/whenNotToUse`, `io.erpbridge/examples`, and
   `io.erpbridge/allowedRoles` `_meta` keys. Clients may ignore them; server
   identity, authorization, and schemas remain authoritative.

   Supported data classes are `public`, `internal`, `pii`, and `restricted`.
   The last two require configured opaque role slugs. Use approved functions
   or opaque assignments, never person names, emails, employee numbers, or
   business records. Use synthetic examples. Disable cache for writes and list
   affected reads in `flushOn` when needed. Use the
   [tool field guide](../assets/mcp-tool.yaml), then validate the actual source.
5. Run `bridgectl tool validate -f <manifest> --context "$CTX"`. Correct all
   failures before the exact apply confirmation. Apply only the reviewed
   source with `bridgectl tool apply -f <manifest> --context "$CTX"`.
   For split output, validate all reviewed files before applying
   `bridgectl tool apply -f <reviewed-directory> --context "$CTX"`.
6. Independently read back with
   `bridgectl tool get <name> --context "$CTX"` and
   `bridgectl tool describe <name> --context "$CTX"`. Verify runtime discovery
   at `/mcp/` and an explicitly approved safe call when required for publication.
   Check the returned version against the reviewed target; do not assume a
   name-only get proves the intended version.
   For guarded tools, check a permitted role and a denied role without elevation.
7. For post-response processing, read [Plugins](plugins.md). Apply and read back
   the exact plugin version before the exact binding and tool version.

## Recovery branches

Resolve the cause before another mutation or probe. Use stable CLI JSON codes
or bracketed table errors. Invalidate affected [session evidence](preflight.md#session-evidence).

1. `CONTEXT_NOT_FOUND`: list safe context names, select an existing `CTX`, and
   rerun core preflight. Do not silently change the saved current context.
2. `LEGACY_REGISTRY`: stop API writes. After confirmation, run
   `bridgectl api scrub-credentials --context "$CTX" --yes` to scrub all known
   registry files, then separately confirm and run
   `bridgectl api migrate-registry --context "$CTX" --yes` for this destination.
   Scrub affects all registry targets despite the explicit context. Review
   collisions before adding `--force`; retain no plaintext backup.
3. `REGISTRY_CONFLICT`: compare the selected context's existing API URL, method,
   module, and credential reference. Use `api register --force` only after
   confirming intentional replacement.
4. `CONTROL_PLANE_URL_INVALID`: use a host root or exact `/mcp`/`/mcp/` suffix.
   Other paths are invalid. Never append REST routes to `/mcp/` manually.
5. `VALIDATION_FAILED`: repair the manifest, data class, roles, schema, or cache
   policy and validate locally again. Do not apply invalid YAML.
6. `AUTHENTICATION_FAILED`: verify the bridge token source, expiry, and scope
   without displaying its value. ERP credentials do not belong in this field.
7. `AUTHORIZATION_DENIED`: verify required admin scope or configured tool role.
   Preserve intended-role denial evidence; do not mask it with admin access.
8. `UPSTREAM_UNREACHABLE`: inspect the selected API route and bounded server
   evidence. For a local deployment, use the relevant local Compose health
   checks in preflight. Remote work does not require localhost checks.
   `--local` remains a separately approved diagnostic, not a workaround.
9. `INSECURE_TRANSPORT`: use HTTPS for credentialed endpoints. Local fixtures
   use only the documented exact development host allowlist; never broaden it
   to make a test pass.
10. `HEALTH_CHECK_FAILED`: inspect the affected deployment's bounded health and
    logs. Use Compose checks only for the local deployment. Recreate only after
    approved configuration changes, then verify health.
11. `RECONCILIATION_FAILED`: read back the exact resource and referenced versions.
    Repair the source and wait for reconciliation rather than repeatedly applying.
12. `RESOURCE_NOT_FOUND`: verify context, name, and version; list before mutation.
    `METHOD_NOT_ALLOWED`: inspect installed help and the route contract instead
    of guessing REST paths.
13. `API_PROBE_FAILED`: keep the bounded probe summary. Check credential reference
    presence on the server and endpoint policy, then obtain exact approval
    before a new test. Upstream bodies and headers stay out of evidence.

## Verification checklist

Use the shared [action and verification gates](preflight.md#action-and-verification-gates).
Mark only applicable onboarding checks, and report any untested seam:

- API registry readback identifies the selected context and intended registration,
  with no ignored legacy registry or unconfirmed overwrite.
- The approved server-side test has a bounded status/timing summary. Label any
  separately approved `--local` run as a host-side diagnostic.
- The only applied source is reviewed under `manifests/<module>/`; generated
  drafts and redacted inspection views are not manifest authority.
- Local validation passed. Independent readback shows the exact tool version
  and expected state. Publication discovery and approved runtime checks passed,
  including intended allow/deny roles where needed.

## Completion evidence

Report the selected context, versions, applicable preflight evidence, probe
status, validation, registry and resource readback, runtime verification, and
remaining risk. Include only safe summaries, not request/response bodies,
credentials, authorization headers, personal data, or opaque invocation tokens.
