# Select and verify the operating context

For deployment work, run core preflight, then only the profiles needed for
its deployment and action. Local-only validation and skill installation need
only their local inputs, version, and target checks, not deployment access. Preflight establishes evidence, not permission to
mutate or invoke an ERP endpoint. Use [management MCP](mcp.md) when that is the
selected route; its context is pinned at launch. CLI deployment examples use
`--context "$CTX"`, where `CTX` is the intended configured context.

| Task | Section |
| --- | --- |
| Deployment workflow | [Core](#core) and [evidence](#session-evidence) |
| Remote reads or administration | [Remote](#remote) |
| Local stack setup or repair | [Local Compose](#local-compose) |
| Register, test, or publish an API | [API onboarding](#api-onboarding) |
| Plugin or binding lifecycle | [Plugin binding](#plugin-binding) |
| Complete a change | [Action and verification gates](#action-and-verification-gates) |

## Core

1. **Context and version:** record the installed CLI version and safe context
   names using `bridgectl version` and `bridgectl context list -o json`, or
   equivalent MCP initialization and context-list evidence when using MCP only.
   Select `CTX` explicitly. A missing or malformed context is a
   `CONTEXT_NOT_FOUND` stop. Do not silently create a context or change the
   saved default. For MCP, confirm the pinned selection instead.
2. **Endpoint:** inspect the selected context's `mcp-server` without disclosing
   sensitive URL parts. The CLI accepts a host root or removes only the exact
   `/mcp` or `/mcp/` suffix. Any other path is a
   `CONTROL_PLANE_URL_INVALID` stop. `/mcp/` is the runtime transport, not a
   REST/control-plane prefix.
3. **Credential source:** identify the bridge token source without displaying
   its value. CLI precedence is `--token`, `BRIDGE_API_TOKEN`, then the context
   token. Prefer the configured secret source over a literal command argument.
   Identify downstream credential references only when the action needs them.
   Environment credentials need the relevant process refresh after rotation.
   File-backed resources need `ERPBRIDGE_CREDENTIALS_DIR` and a readable regular
   reference-named file on the resolving host. Inspect presence, permissions,
   and bounded size only; never read the value into evidence. File resolution
   fails closed and does not fall back to the environment.
4. **Authorization:** identify the required scope, admin access, or tool role.
   A local MCP write grant does not grant remote permission or replace exact
   approval. Check the discovered catalogue and [MCP grant rules](mcp.md).
   Restart management MCP after endpoint, credential, or grant changes.
5. **Target:** name the exact API, tool/version, plugin/version, binding, token
   identifier, or cache selector and expected effect. For repository work,
   record the revision and preserve unrelated working-tree changes. Inspect
   installed command help before using flags.

Stop on unknown required evidence. Capture only safe names, booleans, status
codes, timings, and redacted summaries. Context listing and local validation
alone prove neither remote authentication nor ERP connectivity.

## Remote

For a remote deployment, use the narrowest authorized control-plane read, such
as `bridgectl cache stats --context "$CTX"` for cache work or an exact resource
get for lifecycle work. Check the result, context, and stable error code. For
MCP, inspect both the tool error flag and the JSON `success`/`error.code`.
This establishes only the observed control-plane access, not ERP reachability.

Remote reads and narrow administration do not require local Docker, localhost
health, MockERP, RedisInsight, or API probes. Obtain remote deployment evidence
from its authorized operator when a task needs it. Do not substitute local
fixture health for remote evidence.

## Local Compose

Use this profile only when operating the local Compose deployment or diagnosing
its dependencies. Confirm the checkout, Compose project, ports, and services
before using local addresses.

- Inspect `docker compose ps`. For the standard local demo, check
  `curl -fsS --max-time 5 http://localhost:8080/mcp/health >/dev/null` and,
  only when MockERP is relevant,
  `curl -fsS --max-time 5 http://localhost:8081/health >/dev/null`.
- Check whether `MOCK_ERP_CREDENTIALS_JSON` or `MOCK_ERP_CREDENTIALS_FILE` is
  configured when using MockERP. For a file source, check readability without
  printing content. Check ERP/plugin mounted references as specified in Core.
- Use `docker compose --env-file .env config --quiet` for interpolation.
  Never source `.env` or print its contents. Bootstrap with `make dev-up` only
  after exact approval. After a quoted environment value or Compose credential
  source changes, an approved
  `docker compose --env-file .env up --build --force-recreate -d` refreshes
  containers. Recheck health afterward; recreation is not health evidence.
- `MCP_ENABLE_TEST_TOOLS=true` is development-only. Production discovery must
  exclude `system.*_test`. Use `docker compose port redis 8001` to confirm
  RedisInsight is loopback-only, or opt it out. Neither demo tools nor
  RedisInsight supplies production evidence.

## API onboarding

Use for API registry writes, endpoint tests, generation, and dependent tool
publication. Continue with [Onboarding](onboarding.md) after these checks.

- Inspect `bridgectl api list --context "$CTX" -o json`. Entries belong to
  `~/.bridgectl/registries/<context>.json`; listings are scoped and sorted.
  `LEGACY_REGISTRY` stops API writes. Use the confirmed scrub/migration path
  in onboarding, with one selected destination and no plaintext backup.
- Confirm the endpoint owner, method, expected effects, and downstream
  environment or mounted-file credential source on the resolving server.
- Inspect `bridgectl api test --help`. The default server-side test returns
  only status, content type, latency, and success, but can cause downstream
  effects. Obtain explicit approval for that exact API test. Management MCP
  also requires the `probe` launch grant and exact confirmation fields.
  Never use a probe as a universal read-only health check. `--local` is a
  separately approved host-side diagnostic that resolves credentials in the
  CLI process; keep response bodies out of evidence.
- Select one reviewed source under `manifests/<module>/`. Inspect its target
  and working-tree status. Generated files are temporary drafts, not a second
  `schemas/` tree or sibling JSON authority. Review and validate the exact
  source before apply. Redacted MCP inspection views are not applicable
  manifests; use generation or the reviewed source instead.

## Plugin binding

Use for plugin or binding changes. Read [Plugins](plugins.md) for the resource
and transport contract rather than duplicating it here. Confirm the separately
operated endpoint, exact plugin and tool versions, binding name, timeout,
phase, failure policy, and affected-tool cache impact. Check admin admission,
credential-source presence, and exact endpoint allowlist requirements for the
selected phase. A `raw_response` binding requires authenticated admin access
and the endpoint allowlist even without plugin authentication. Validate the
exact manifests before any apply.

## Session evidence

Keep a short redacted table in the current workflow, not a credential log:

| Evidence | Context/target | Observation | Validity |
| --- | --- | --- | --- |
| Core | Selected context and endpoint label | Version, source names, scope | Current selection |
| Selected profile | Exact resource/version or deployment | Status/code, validation or readback | Current state |
| Action | Exact confirmed target | Approval and observed outcome | This action only |

Reuse evidence only within the same session while its inputs remain unchanged.
Invalidate relevant rows after mutation, context/endpoint/credential/revision
change, or a new failure. Recheck affected rows before continuing. Approval is
per action, not reusable blanket authority. Persist only redacted terminal
knowledge through the existing completion checkpoint, when applicable.

## Action and verification gates

Before every mutation or effectful test, explain and obtain confirmation for
the exact context, target, and expected effect. Hard delete and all-cache flush
need their explicit impact repeated. Validate applicable manifests first.
Afterward, independently read back the exact resource and expected state, or
verify the narrow cache statistic. A completed command is not reconciliation
proof. For an unknown write outcome, read back before considering a retry.

Use runtime discovery and an explicitly approved safe call only when runtime
behavior is part of acceptance. Verify intended allow/deny roles for guarded
tools without elevation. Report what each check proves and any untested seam;
never turn an unapproved probe into a completion requirement for a narrow read.
Keep tokens, authorization headers, payloads, ERP data, and credential values
out of logs, reports, and handoff evidence.
