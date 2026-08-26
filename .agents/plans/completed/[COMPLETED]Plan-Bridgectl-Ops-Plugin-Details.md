# Plan: Extend the `bridgectl-ops` Skill with External Plugin Operations

## Goal

Make `bridgectl-ops` reliably guide agents through external plugin and plugin-binding authoring, authentication, lifecycle management, verification, and diagnosis. Keep the skill focused on `bridgectl`, the ERPBridge server, and separately operated plugin processes; do not add ERPBridge Console or UI guidance.

## Current State

- The model-invoked skill currently advertises API onboarding, MCP tool lifecycle work, token/role administration, cache/log operations, SDK diagnosis, and bug reporting, but not plugin operations (`skills/bridgectl-ops/SKILL.md:1-11`).
- Its workflow router points to onboarding, operations, diagnostics, and an MCP tool template, with no plugin-specific reference (`skills/bridgectl-ops/SKILL.md:31-40`).
- Its change gate names API/tool/token/cache mutations but does not cover plugin or binding apply/delete operations (`skills/bridgectl-ops/SKILL.md:42-52`).
- The authoritative plugin contract defines exact-version `Plugin` resources, `api`/`docker` types, HTTP endpoints, bounded timeouts, `bearer`/`api-key` auth, `PLUGIN_*` credential references, and allowlisted credentialed endpoints (`docs/plugin-schema.md:7-50`).
- `PluginBinding` connects exact plugin and tool versions, runs only in `after_response`, orders by priority, supports `continue`/`fail`, and has soft-delete and active-reference constraints (`docs/plugin-schema.md:52-84`).
- The plugin wire contract is a bounded synchronous `POST /v1/process`; it excludes original arguments, inbound headers, caller identity, caller tokens, and ERP credentials, and disables redirects and retries (`docs/plugin-schema.md:86-117`).
- The CLI already supports strict JSON/YAML resource decoding, sequences, multi-document YAML, and directory application (`internal/cli/plugin.go:46-126`), plus plugin `apply|get|validate|delete` and exact `name@version` lookup (`internal/cli/plugin.go:140-300`). Bindings have parallel `apply|get|validate|delete` commands and name-only identity (`internal/cli/plugin_binding.go:19-179`).
- Credentialed plugin admission requires configured `API_AUTH_TOKEN`, an authenticated admin caller, and an exact normalized `PLUGIN_ENDPOINT_ALLOWLIST` match (`internal/mcp/plugin_api.go:193-214`).
- Runtime reconciliation activates only matching active plugin/tool references and orders bindings by priority (`internal/mcp/plugin_registry.go:9-135`). Plugin processing occurs after successful tool/output validation, revalidates transformed output, and applies the binding failure policy without exposing plugin details (`internal/mcp/plugin_pipeline.go:22-124`).
- Plugin or binding lifecycle changes flush affected tool cache entries (`internal/mcp/plugin_api.go:371-394`), and control-plane routes are `/apis/erpbridge.io/v1/plugins` and `/apis/erpbridge.io/v1/pluginbindings` (`internal/mcp/server.go:690-701`).
- Existing CLI, runtime, and black-box plugin tests provide verification seams (`internal/cli/plugin_test.go`, `internal/cli/plugin_binding_test.go`, `internal/mcp/plugin_test.go`, `internal/mcp/plugin_client_test.go`, `internal/mcp/plugin_api_test.go`, `internal/mcp/server_plugin_test.go`, `internal/integration/plugin_system_test.go`).

## External Skill-Creation Guidance

This plan also follows the supplied Agent Skills guidance:

- The quickstart defines a skill as a directory containing `SKILL.md`, with `name` and `description` driving discovery and activation through progressive disclosure: [quickstart](https://agentskills.io/skill-creation/quickstart).
- Best practices require domain-specific evidence, coherent scope, moderate detail, a concise core under roughly 500 lines/5,000 tokens, explicit pointers to references, validation loops, and scripts only where deterministic repeated work justifies bundling: [best practices](https://agentskills.io/skill-creation/best-practices).
- Description optimization requires imperative, user-intent-focused trigger wording, a concise description under 1,024 characters, realistic positive and near-miss negative queries, repeated runs, and a fixed train/validation split: [optimizing descriptions](https://agentskills.io/skill-creation/optimizing-descriptions).
- Script guidance favors existing one-off commands when sufficient; any future bundled script must be non-interactive, expose `--help`, separate structured stdout from diagnostics, use safe/idempotent defaults, bound output, and return meaningful exit codes: [using scripts](https://agentskills.io/skill-creation/using-scripts).

## Decisions

1. **Use a dedicated `references/plugins.md`.** Plugin manifests, lifecycle, wire behavior, security constraints, and verification form a distinct operational branch. Keep `SKILL.md` as the router and guardrail surface, and avoid duplicating the full contract in `operations.md` or `diagnostics.md`.
2. **Keep the deployment boundary explicit.** The skill will state that ERPBridge stores plugin definitions and invokes an already-running endpoint; it does not install, start, upgrade, or manage plugin code or containers.
3. **Use exact identities.** Plugin commands use `name@version`; bindings use their binding name and reference exact plugin/tool versions. This prevents ambiguous lifecycle or rollback instructions.
4. **Keep credential handling environment-backed.** Manifests contain only `PLUGIN_*` credential references. The skill will require protected admin admission and exact endpoint allowlisting for credentialed plugins, and will never ask agents to print or paste credential values.
5. **Preserve progressive disclosure.** Add short router links and lifecycle gates to existing references, while placing detailed plugin contract and protocol guidance behind `references/plugins.md`.
6. **Do not change public product documentation or add Console details.** This is an agent-skill update; `docs/plugin-schema.md` and generated CLI pages remain the behavior sources of truth.
7. **Keep the always-loaded skill core lean.** Put the plugin contract, protocol details, and troubleshooting branches behind `references/plugins.md`; keep `SKILL.md` limited to routing, guardrails, and completion criteria so it remains within the Agent Skills progressive-disclosure budget.
8. **Evaluate the description instead of guessing.** Add a fixed 20-query trigger set with balanced positive and near-miss negative cases, split it into approximately 60% training and 40% held-out validation data, run each query three times, and select the description by held-out performance rather than training performance.
9. **Do not bundle an operations script without repeated deterministic work.** Existing `bridgectl` and `npx` commands are sufficient one-offs. If later evaluation work reveals repeated helper logic, add a small non-interactive script with `--help`, structured output, bounded output size, safe defaults, and meaningful exit codes.
10. **Keep evaluation artifacts local-only.** The entire `skills/bridgectl-ops/evals/` directory is ignored by Git, so trigger and behavior evals remain available for local iteration without entering the release commit.

## Scope

### In scope

- Update `skills/bridgectl-ops/SKILL.md` frontmatter, routing, plugin mutation gates, deployment boundary, and verification handoff.
- Add `skills/bridgectl-ops/references/plugins.md` with manifest, auth, lifecycle, runtime, security, verification, and troubleshooting guidance.
- Add concise cross-links and plugin-specific operational/diagnostic branches to `references/operations.md`, `references/diagnostics.md`, `references/ecosystem.md`, and `references/onboarding.md`.
- Add safe field-guide templates for `Plugin` and `PluginBinding` resources under `skills/bridgectl-ops/assets/`.
- Add realistic plugin-operation behavior prompts and a 20-query description-trigger evaluation set under `skills/bridgectl-ops/evals/`.
- Validate skill packaging, links, trigger wording, secret-safety wording, description length, and exclusion of Console/UI content.

### Intentionally out of scope

- ERPBridge Console routes, topology, UI, screenshots, or frontend behavior.
- Changes to plugin runtime, CLI implementation, schemas, integration fixtures, or public docs.
- Plugin deployment automation, image installation, container orchestration, or plugin code changes.
- Changes to the existing MCP tool manifest, bug-report template, or unrelated skills.

## Tasks

- [x] **Task 1: Add the plugin operational reference and safe manifest field guides.**
  - Add `references/plugins.md` covering prerequisites, `Plugin` and `PluginBinding` fields, exact SemVer references, endpoint/timeout limits, bearer/API-key behavior, `PLUGIN_*` references, `API_AUTH_TOKEN`, `PLUGIN_ENDPOINT_ALLOWLIST`, strict resource formats, apply/get/validate/delete commands, soft versus hard deletion, `/v1/process` payload and response contract, priority and failure policies, cache-miss behavior, and safe verification/diagnosis.
  - Add `assets/plugin.yaml` and `assets/plugin-binding.yaml` as minimal examples with placeholder endpoint and environment-reference values only.
  - Link to `../../../docs/plugin-schema.md` and the generated plugin CLI pages instead of copying command help that can drift.
  - **Seam:** Existing authoritative schema and CLI/runtime seams in `docs/plugin-schema.md`, `internal/cli/plugin.go`, `internal/cli/plugin_binding.go`, and `internal/mcp/plugin_pipeline.go`.
  - **Files:** `skills/bridgectl-ops/references/plugins.md`, `skills/bridgectl-ops/assets/plugin.yaml`, `skills/bridgectl-ops/assets/plugin-binding.yaml`.
  - **Verify:** Every plugin claim in the new reference is traceable to `docs/plugin-schema.md` or the cited source/tests; all relative links resolve; a raw-secret audit finds no credential values or secret-like manifest fields.

- [x] **Task 2: Route plugin work through the existing skill workflow and guard mutations.**
  - Update the `bridgectl-ops` description so model invocation includes external plugins and bindings.
  - Add a plugin workflow row, a dedicated plugin change gate requiring context/identity/endpoint/tool/cache-impact confirmation, and a verification handoff for plugin and binding readback plus safe MCP/direct invocation.
  - Add concise plugin lifecycle guidance to `references/operations.md`, plugin failure classes and redaction guidance to `references/diagnostics.md`, the separately operated plugin boundary to `references/ecosystem.md`, and an optional post-response-plugin handoff to `references/onboarding.md`.
  - Bump the skill metadata version from `3.0.0` to `3.1.0`.
  - Preserve the existing API/tool/token/cache guidance and do not mention ERPBridge Console or UI concepts.
  - **Seam:** `skills/bridgectl-ops/SKILL.md` router and its four existing reference branches.
  - **Files:** `skills/bridgectl-ops/SKILL.md`, `skills/bridgectl-ops/references/operations.md`, `skills/bridgectl-ops/references/diagnostics.md`, `skills/bridgectl-ops/references/ecosystem.md`, `skills/bridgectl-ops/references/onboarding.md`.
  - **Verify:** `npx --yes skills-ref validate skills/bridgectl-ops`; plugin prompts route to `references/plugins.md`; `rg -ni 'console|topology|frontend|UI' skills/bridgectl-ops` returns no new Console/UI guidance; existing secret and stale-name audits remain clean.

- [x] **Task 3: Add skill behavior and description evaluation cases.**
  - Add 2–3 realistic plugin-operation prompts to `evals/evals.json` for qualitative skill-output review, including safe plugin onboarding, exact-version binding, and a plugin failure/rollback diagnosis. Keep this evaluation directory local-only.
  - Add approximately 20 realistic description-trigger queries to `evals/description-trigger-evals.json`: 8–10 should-trigger cases covering plugin registration, plugin auth, bindings, lifecycle, and runtime failures; 8–10 should-not-trigger near misses covering adjacent API/tool operations without plugin work. Keep the split fixed at approximately 60% train and 40% validation. Keep this evaluation directory local-only.
  - Evaluate the revised description three times per query when the local skill-evaluation client is available, inspect held-out results, and keep the best generalizing description. Do not add query-specific keywords to overfit failures, and keep the final frontmatter description below 1,024 characters. The local `claude` CLI is unavailable in this environment, so the model-trigger loop is recorded as deferred after static eval validation.
  - Use existing direct commands for package validation; do not add a bundled script unless repeated evaluation work demonstrates a deterministic helper need. Any added helper must meet the non-interactive, `--help`, structured-output, bounded-output, safe-default, and exit-code requirements above.
  - **Seam:** Agent-skill discovery/activation plus qualitative plugin-operation runs and held-out description-trigger runs.
  - **Files:** `skills/bridgectl-ops/evals/evals.json`, `skills/bridgectl-ops/evals/description-trigger-evals.json`.
  - **Verify:** The eval files are valid JSON, labels cover both positive and near-miss negative cases, and the description remains under 1,024 characters. Run the repeated held-out comparison when the local skill-evaluation client is available; this environment records that client as unavailable.

- [x] **Task 4: Run the skill quality and repository regression checks.**
  - Review the changed skill as an agent-facing document for progressive disclosure, one source of truth, executable completion criteria, and positive security instructions.
  - Run the existing plugin-focused unit and black-box checks without modifying implementation code; use the integration fixture only when its external prerequisites are available.
  - **Seam:** Skill package validation plus existing plugin tests and integration fixture.
  - **Files:** `.gitignore`, `.agents/plans/README.md`, `.agents/plans/active/README.md`, plus the files changed in Tasks 1–3.
  - **Verify:** `skills-ref@0.1.5 validate skills/bridgectl-ops`; `go test ./internal/cli ./internal/mcp`; `make test`; `go test -tags pluginintegration ./internal/integration -run TestPluginSystemBlackBox -count=1` when the plugin stack is available; `git diff --check`; no task-unrelated files are staged. The plugin integration stack was not running in this environment, so that optional black-box command was not executed.

## Verification

- The skill package validates successfully and all skill-internal and source-reference links resolve.
- A prompt about plugin registration, plugin auth, plugin binding, plugin timeout, plugin transformation failure, or plugin cache invalidation reaches the plugin reference.
- The description trigger set contains realistic should-trigger and near-miss should-not-trigger prompts, uses a fixed train/validation split, and is evaluated across repeated runs rather than a single invocation.
- Instructions require `bridgectl plugin validate` before apply, protected admin admission for credentialed resources, `name@version` plugin identity, exact plugin/tool binding references, readback, and safe invocation verification.
- The skill states that plugin payloads exclude caller and ERP credentials, and reports exclude endpoints where sensitive, auth headers, payloads, response bodies, personal data, and opaque tokens.
- The skill explains that soft deletion retains records, hard deletion needs explicit confirmation, and an active binding blocks hard deletion of its exact plugin version.
- Existing CLI and runtime plugin tests remain green; no Console/UI guidance is introduced.
- Any bundled helper script, if evidence requires one, is non-interactive, documented with `--help`, structured, bounded, safe by default, and explicit about exit codes; otherwise validation uses existing one-off commands.

## Open Questions

None. The requested boundary is clear: enrich `bridgectl-ops` with external plugin operations while excluding ERPBridge Console details.
