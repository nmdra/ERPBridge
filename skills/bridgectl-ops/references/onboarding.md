# Onboard an ERP API as an MCP tool

## Outcome

An ERP endpoint is registered locally, connectivity is demonstrated through the
server, its tool schema is reviewed and validated, and the approved manifest is
visible through the selected ERPBridge context. The path is deterministic and
stops before mutation when a precondition is unknown.

## Preflight

Run this read-only gate before registering an API, applying a tool, or changing
stack state. Set `CTX` to the intended configured context; use the same explicit
`--context "$CTX"` on every bridgectl command.

1. **Context:** record the CLI version and inspect context names and the selected
   context without printing tokens:
   `bridgectl version` and `bridgectl context list -o json`. A missing or
   malformed selected context is a hard stop (`CONTEXT_NOT_FOUND`).
2. **Stack:** inspect `docker compose ps`, then check
   `curl -fsS --max-time 5 http://localhost:8080/mcp/health >/dev/null` and
   `curl -fsS --max-time 5 http://localhost:8081/health >/dev/null`. Start or
   repair the stack before any API or tool mutation. `make dev-up` is the safe
   local bootstrap. After a quoted `.env` value or Compose credential source
   changes, use `docker compose --env-file .env up --build --force-recreate -d`.
   `--force-recreate` refreshes containers; it does not replace health checks.
3. **Environment:** check only whether the MockERP credential source is set
   (`MOCK_ERP_CREDENTIALS_JSON` or `MOCK_ERP_CREDENTIALS_FILE`) and, for a file
   source, whether the path is readable. Use Compose's `--env-file .env` and
   `docker compose --env-file .env config --quiet` to test interpolation. Never
   source `.env`, print its contents, or echo credential values.
4. **Control-plane root:** inspect the selected context's `mcp-server`. The
   CLI accepts a host root or removes only the exact `/mcp` or `/mcp/` suffix.
   Any other path is a `CONTROL_PLANE_URL_INVALID` stop. `/mcp/` is the MCP
   transport, not a REST/control-plane path.
5. **Probe mode:** confirm the installed command exposes `--local` with
   `bridgectl api test --help`. The normal `bridgectl api test <name>` is the
   authenticated server-side probe and returns only status, content type,
   latency, and success. Reserve `--local` for an explicit offline diagnostic;
   it resolves the ERP credential in the CLI process.
6. **Registry:** run `bridgectl api list --context "$CTX" -o json`. New entries
   belong to `~/.bridgectl/registries/<context>.json`; listings are context
   scoped and sorted. A legacy `~/.bridgectl/registry.json` is a
   `LEGACY_REGISTRY` stop, not an invitation to overwrite or ignore it. Use the
   explicit scrub/migration workflow only after confirmation and with one
   selected destination context.
7. **Manifest ownership:** choose one reviewed source at
   `manifests/<module>/`. Generated output is a temporary draft; it is not a
   second `schemas/` tree or a set of sibling JSON files. Inspect the target
   file and `git status` before applying, and keep generated and reviewed
   artifacts distinct.
8. **Local-demo boundary:** `MCP_ENABLE_TEST_TOOLS=true` is for the development
   stack only; production discovery must exclude `system.*_test`. Check that
   Use `docker compose port redis 8001` to confirm RedisInsight is
   loopback-only or opt it out. Never use demo tools or RedisInsight as
   production evidence.

Do not proceed when any check is ambiguous. Capture only safe booleans, names,
status codes, and timings. The exact register/apply target and expected effect
still require the change confirmation in `SKILL.md`.

## Workflow

1. Confirm the endpoint owner, URL, method, module, purpose, expected inputs,
   and the environment/secret-manager reference for its credential. Inspect the
   installed CLI help and matching `docs/cli/` page. Register only after the
   preflight and exact-action confirmation. Use `--force` on `api register`
   only when replacing the named API is intentional.
2. Run `bridgectl api test <name> --context "$CTX"`. Stop on failure and use
   the recovery branches below; do not publish an unverified endpoint. Use
   `--local` only when the operator explicitly needs the legacy host-side
   path, and do not copy its response body into evidence.
3. Generate a draft with `bridgectl tool generate --api <name>`; add
   `--openapi <path-or-url>` for multiple operations. Redirect the explicit
   YAML output to a temporary file, validate it, review it, and remove it.
   `make generate-tools` is the convenience path that applies one temporary
   stream once; use it only when that apply is intended.
4. Save reviewed manifests only under `manifests/<module>/`. Complete intent
   metadata, input/output schemas, execution path, `credentialRef`, cache
   policy, `security.dataClass`, and optional `allowedRoles`. `public`,
   `internal`, `pii`, and `restricted` are the supported data classes;
   `pii` and `restricted` require configured opaque role slugs. Use roles that
   identify an approved function or opaque assignment, never a person name,
   email, employee number, or business record. Use safe synthetic examples.
   Disable cache for writes and list affected reads in `flushOn` when needed.
   Use `assets/mcp-tool.yaml` as a field guide, then validate the actual file.
5. Run `bridgectl tool validate -f <manifest> --context "$CTX"`. Correct all
   failures before the apply confirmation. Apply only the reviewed source:
   `bridgectl tool apply -f <manifest> --context "$CTX"`.
6. Verify with `tool get`, `tool describe`, MCP discovery, and a safe call at
   `/mcp/`. For guarded tools, verify one permitted role and one denied role;
   never elevate an identity to hide a role failure.
7. If post-response processing is required, read [Plugins](plugins.md). Keep
   plugin deployment and protocol details there: apply the exact plugin
   version, then the exact binding and tool version, and verify the binding.

## Recovery branches

Do not retry a mutating command until its cause is resolved. Use the stable
code in CLI JSON output or the bracketed table error to select one branch:

1. `CONTEXT_NOT_FOUND`: list contexts, select an existing name with
   `--context`, and re-run the read-only preflight. Do not silently create or
   switch the saved current context.
2. `LEGACY_REGISTRY`: stop API writes. After confirmation, run
   `bridgectl api scrub-credentials --yes` to scrub all known registry files,
   then run `bridgectl api migrate-registry --context "$CTX" --yes` to move
   the cleaned global registry to this destination. Add `--force` only after
   reviewing collisions; retain no plaintext backup.
3. `REGISTRY_CONFLICT`: read the existing API in the selected context and
   compare its URL, method, module, and credential reference. Use
   `api register --force` only after confirming intentional replacement.
4. `CONTROL_PLANE_URL_INVALID`: set `mcp-server` to the host root or an exact
   `/mcp`/`/mcp/` transport suffix. Remove other paths; never append REST
   routes to `/mcp/` manually.
5. `VALIDATION_FAILED`: repair the manifest, data class, roles, schema, or
   cache policy and run local `tool validate` again. Do not apply invalid YAML.
6. `AUTHENTICATION_FAILED`: verify the selected bridge token source and
   expiry/scope without displaying its value. Use an authorized credential;
   do not put ERP credentials in the bridge token field.
7. `AUTHORIZATION_DENIED`: verify the required admin scope or configured tool
   role. Test with the intended role and keep the denial; do not use admin
   access to mask a guarded-tool failure.
8. `UPSTREAM_UNREACHABLE`: check both health endpoints, the selected API URL,
   and bounded server evidence. Correct the service route or wait for recovery;
   `--local` is only a separately stated diagnostic.
9. `INSECURE_TRANSPORT`: use HTTPS for credentialed endpoints. For the local
   fixture, use only the documented exact development host allowlist; never
   broaden it to make a test pass.
10. `HEALTH_CHECK_FAILED`: inspect `docker compose ps` and bounded service
    health/log evidence, repair the failed dependency, and re-run preflight.
    `--force-recreate` is appropriate after configuration changes, not as a
    substitute for diagnosis.
11. `RECONCILIATION_FAILED`: read back the exact tool, plugin, or binding
    identity and validate referenced versions. Correct the source manifest and
    wait for reconciliation rather than repeatedly applying it.
12. `RESOURCE_NOT_FOUND`: verify the exact context, API/tool name, and version;
    list before changing state. `METHOD_NOT_ALLOWED`: re-read the installed
    CLI help and route contract instead of guessing a REST path.
13. `API_PROBE_FAILED`: preserve the bounded probe summary, check credential
    reference presence on the server and endpoint policy, then retry the
    server-side probe. Upstream bodies and headers stay out of evidence.

## Verification checklist

Mark each item with a safe observable result before handoff:

- [ ] The selected context name, CLI version, and control-plane root are
  recorded; every command used the same explicit context.
- [ ] Compose interpolation passed with `--env-file` (or no env file), both
  health endpoints passed, and any changed quoted value was followed by
  `--force-recreate`.
- [ ] Credential sources and references were present by name/boolean only;
  no credential, token, `.env` content, or authorization header was printed.
- [ ] API test used the server-side default and returned only its bounded
  summary; `--local`, if used, is labelled as an offline diagnostic.
- [ ] API readback is in the selected context registry, with no legacy global
  registry being ignored and no unconfirmed overwrite.
- [ ] The reviewed manifest is the only applied source under
  `manifests/<module>/`; no generated `schemas/` or per-tool JSON artifact is
  treated as authoritative.
- [ ] Local validation passed; apply readback shows the exact name/version and
  expected state; MCP `/mcp/` discovery and a safe call succeeded.
- [ ] For sensitive tools, role admission and both allow/deny calls were
  checked with non-identifying role slugs. Demo tools are disabled outside
  local development, and RedisInsight is loopback-only or absent.
- [ ] Logs, reports, and final output contain status/timing evidence only, with
  no ERP bodies, personal data, secrets, or internal stack details.

## Completion evidence

Record the selected context, API probe status, manifest version, validation
result, registry readback, MCP verification, and unresolved risk. Do not
include request bodies or responses that contain secrets, personal data, or
opaque invocation tokens.
