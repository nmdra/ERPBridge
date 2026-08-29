# Plan: Refresh the bridgectl-ops Skill

## Goal

Align the bundled, model-invoked `bridgectl-ops` skill with the ERPBridge
`v0.5.0-alpha.1` CLI and runtime. Agents must be able to use file-backed
credential rotation, split tool generation, MCP annotation hints, and
namespaced discovery metadata without weakening authorization or secret
handling. The refreshed source must also install cleanly through the bundled
skill command.

## Current State

- The repository source of truth is `skills/bridgectl-ops/`. Its `SKILL.md`
  remains model-invoked, is version `3.2.0`, and routes agents through
  onboarding, operations, plugins, diagnostics, and manifest-template
  references (`skills/bridgectl-ops/SKILL.md:1-68`).
- The current skill requires an environment or secret-manager reference during
  onboarding but does not describe the resource-level `credentialSource: file`
  contract (`skills/bridgectl-ops/references/onboarding.md:62-80`). The plugin
  branch still describes plugin credentials as environment variables only
  (`skills/bridgectl-ops/references/plugins.md:15-50`).
- The CLI accepts `--credential-source env|file` when registering an API and
  carries that source into server-side probes (`internal/cli/api.go:63-81,263-270,385-397`).
  `ERPBRIDGE_CREDENTIALS_DIR` and the fail-closed file behavior are documented
  in `docs/environment-variables.md:29-30,79-110`; file-backed resources bypass
  the response cache in `docs/caching.md:102-108`.
- `bridgectl tool generate --output-dir` writes one `.json` or `.yaml` file per
  generated tool and keeps the count message on stderr
  (`internal/cli/tool.go:100-124,476-503,569-571`; `docs/cli/bridgectl_tool_generate.md:1-25`).
  The current onboarding reference only describes temporary stream output and
  does not document this split-file path (`skills/bridgectl-ops/references/onboarding.md:62-94`).
- Generated tools now receive optional method-based annotation hints
  (`internal/idp/generator.go:381-406`). The tool schema documents the four
  annotation fields and their draft-only meaning (`docs/tool-schema.md:52-65`).
  The current `assets/mcp-tool.yaml` has no annotation, output-schema, or
  `credentialSource` guidance (`skills/bridgectl-ops/assets/mcp-tool.yaml:1-34`).
- MCP discovery projects reviewed intent and allowed roles into namespaced
  `_meta`, while `RegisterTool` projects annotations and output schemas
  (`internal/mcp/server.go:535-557,598-613`). These values are explicitly
  informational and do not replace authorization (`docs/tool-schema.md:67-80`;
  `internal/mcp/auth_test.go:276-346`). The current skill does not explain
  these client-visible fields.
- The bundled skill is embedded from `SKILL.md`, `references`, and `assets`
  (`skills/bridgectl-ops/embed.go:6-10`). `bridgectl skill install` supports
  global, project, and explicit-directory destinations, with `--force`
  replacing an existing directory, and does not contact a server
  (`internal/cli/skill.go:15-30,69-106`). Existing tests verify the complete
  embedded tree and intentionally exclude local evaluation files
  (`internal/cli/skill_test.go:15-19,67-87`; `.gitignore:54`).
- Agent Skills guidance recommends a concise model-invoked description,
  progressive disclosure through references, a core under roughly 500 lines,
  and repeated positive/near-miss trigger evaluation:
  [best practices](https://agentskills.io/skill-creation/best-practices),
  [description optimization](https://agentskills.io/skill-creation/optimizing-descriptions),
  and [skill evaluation](https://agentskills.io/skill-creation/evaluating-skills).

## Decisions

1. **Keep `skills/bridgectl-ops/` authoritative.** Update the repository skill
   and let `embed.go` distribute it. Treat
   `~/.agents/skills/bridgectl-ops/` as a derived installation; do not edit it
   by hand. Refresh it only after building the updated CLI and validating the
   embedded tree.
2. **Keep the same model-invoked skill and name.** Preserve `name:
   bridgectl-ops`, add the new user-intent branches to the description, and
   bump the skill metadata to `3.3.0`. Keep detailed contracts behind the
   existing references instead of expanding the always-loaded core.
3. **Treat credential source as a per-resource choice.** Environment lookup
   remains the default. File mode uses `ERPBRIDGE_CREDENTIALS_DIR`, resolves
   immediately before each operation, fails closed on invalid files, and does
   not fall back to the environment. The skill must distinguish environment
   rotation, which needs process refresh and normal cache handling, from
   file-backed rotation, which bypasses the response cache.
4. **Treat annotations and `_meta` as hints, not controls.** Review generated
   method-based annotations and preserve explicit boolean values. Explain all
   four `io.erpbridge/*` metadata keys as optional discovery guidance that a
   client may ignore or omit from model context. Server-side identity,
   authorization, and schemas remain authoritative.
5. **Keep runtime behavior unchanged.** Do not change CLI, runtime, protocol,
   schema, integration fixture, or public documentation behavior. Update the
   repository agent instructions where their security wording conflicts with
   the supported per-resource file source. Link to the existing source-of-truth
   docs rather than copying command contracts that can drift. Add only the
   release-note entry needed to record the agent-guidance refresh.
6. **Use local-only evaluation artifacts.** Store behavior prompts and trigger
   cases under the already ignored `skills/bridgectl-ops/evals/` directory so
   they can guide iteration without being embedded or released.

## Scope

### In scope

- Refresh `SKILL.md` routing, description, version, credential guardrails,
  and bundled-skill installation guidance.
- Correct `AGENTS.md` so repository security guidance distinguishes environment
  credentials, `API_AUTH_TOKEN`, and explicit mounted-file sources.
- Update onboarding, operations, diagnostics, plugin, and ecosystem references
  for file-backed credentials, hot rotation, split generation, annotations,
  namespaced `_meta`, and current verification seams.
- Extend MCP tool and plugin field-guide templates with safe optional fields
  and file-source comments.
- Add local behavior and description-trigger evaluation cases, then compare
  the refreshed skill with the current skill when the local evaluation client
  is available.
- Rebuild `bridgectl`, verify embedded delivery in a temporary destination, and
  refresh the local installed skill only after the source passes validation.
- Record the skill refresh in `CHANGELOG.md` under `Unreleased`.

### Intentionally out of scope

- Changes to Go CLI or server implementation, MCP protocol negotiation, tool
  schemas, plugin wire behavior, or authorization logic.
- New cloud secret-provider integrations or plugin deployment automation.
- ERPBridge Console, frontend, or unrelated agent skills.
- Public documentation changes unless implementation finds a source page that
  contradicts the already documented behavior; no such contradiction is
  currently known.
- Runtime or protocol changes; the `AGENTS.md` edit only reconciles repository
  instructions with behavior already supported and documented.
- Committing evaluation outputs, tokens, credentials, or the derived global
  skill installation.

## Tasks

- [x] **Task 1: Refresh the skill core and routing.** Update the model-facing
  description and `3.3.0` metadata to trigger for credential-source rotation,
  split tool generation, annotation review, discovery metadata, and bundled
  skill installation. Add a short distribution section that uses
  `bridgectl skill install --project` for project scope, the default global
  destination when appropriate, and an explicit destination for inspection;
  require confirmation before `--force` replaces an existing tree. Keep
  `SKILL.md` as a router and guardrail surface, not a copy of the detailed
  contracts. **Seam:** model invocation plus `bridgectl skill install`.
  **Files:** `skills/bridgectl-ops/SKILL.md`,
  `skills/bridgectl-ops/references/ecosystem.md`.
  **Verify:** frontmatter parses; the description is below 1,024 characters;
  `SKILL.md` remains below 500 lines; every new router link resolves; and the
  core contains no endpoint, credential, token, or personal-data examples.

- [x] **Task 2: Bring onboarding and the MCP tool template up to date.** Add
  preflight checks for `credentialSource` and `ERPBRIDGE_CREDENTIALS_DIR`
  without printing values. Document `--credential-source file`, file-backed
  rotation and its failure behavior, `tool generate --output-dir` with exact
  tool-name filenames, JSON/YAML selection, stderr status output, and directory
  apply. Require review of generated descriptions, output schemas, optional
  method-based annotations, and the four namespaced `_meta` values while
  keeping authorization server-side. Extend `assets/mcp-tool.yaml` with safe
  `outputSchema`, annotation, and `credentialSource` examples or comments.
  **Seam:** `bridgectl api register`, `tool generate`, local validation, and
  MCP `tools/list`. **Files:**
  `skills/bridgectl-ops/references/onboarding.md`,
  `skills/bridgectl-ops/assets/mcp-tool.yaml`.
  **Verify:** each command and field matches `docs/environment-variables.md`,
  `docs/tool-schema.md`, and `docs/cli/bridgectl_tool_generate.md`; the YAML
  template parses; links resolve; and a static audit finds no raw secret or
  endpoint values.

- [x] **Task 3: Correct lifecycle, plugin, and diagnosis guidance.** Explain
  file-backed plugin credentials, `ERPBRIDGE_CREDENTIALS_DIR`, fail-closed
  reads, atomic local replacement, projected-secret eventual consistency,
  cache bypass for file-backed resources, and the separate cache implications
  of environment rotation. Preserve exact plugin/tool version references,
  endpoint allowlisting, soft-delete behavior, and redacted evidence. Add
  generated-tool annotation and `_meta` troubleshooting guidance where it
  affects client-visible behavior. Extend the plugin template with a safe
  `credentialSource: file` comment. **Seam:** plugin/binding validate, apply,
  readback, cache stats, bounded logs, and MCP/direct invocation.
  **Files:** `skills/bridgectl-ops/references/operations.md`,
  `skills/bridgectl-ops/references/plugins.md`,
  `skills/bridgectl-ops/references/diagnostics.md`,
  `skills/bridgectl-ops/assets/plugin.yaml`.
  **Verify:** every new runtime claim maps to `docs/plugin-schema.md`,
  `docs/environment-variables.md`, `docs/caching.md`, or the cited source;
  plugin and security audits remain clean; and no guidance suggests that
  discovery metadata grants permission.

- [x] **Task 4: Evaluate the refreshed skill without releasing evaluation data.**
  Add three behavior prompts covering file-backed onboarding and rotation,
  split generation plus annotation/_meta review, and exact-version plugin
  diagnosis. Add 20 realistic description-trigger cases with a fixed 60/40
  train/validation split: 10 should trigger for bridgectl operations and 10
  should not trigger for near-miss API/tool or unrelated tasks. Snapshot the
  current skill, run with-skill and old-skill comparisons, and repeat each
  trigger query three times when the local evaluation client is available.
  If that client is unavailable, complete the static JSON and description
  checks and record the deferred dynamic comparison in the plan; do not
  overfit the description to one query. **Seam:** skill activation and
  qualitative output review. **Files:**
  `skills/bridgectl-ops/evals/evals.json`,
  `skills/bridgectl-ops/evals/description-trigger-evals.json` (local-only).
  **Verify:** both files are valid JSON, labels are balanced and realistic,
  the held-out split is fixed, and the selected description generalizes better
  on validation than the prior description or the dynamic comparison is
  explicitly recorded as unavailable.

- [x] **Task 5: Validate packaging, embedding, and repository quality.** Run
  Agent Skills package validation, relative-link and secret audits, focused
  skill/CLI tests, and the repository test gate. Build a temporary updated
  `bridgectl`, install the embedded skill into a temporary directory with
  `--dir`, and compare its tracked `SKILL.md`, references, and assets with the
  source tree. After source validation, refresh the existing local installation
  with the rebuilt binary only after confirming the exact destination; never
  commit that derived tree. Add the concise `Unreleased` changelog entry.
  **Seam:** package validator, `internal/cli` embedding tests, and temporary
  `bridgectl skill install` output. **Files:** `AGENTS.md`, `CHANGELOG.md`;
  verification also covers `skills/bridgectl-ops/embed.go`,
  `internal/cli/skill.go`, and `internal/cli/skill_test.go` without requiring
  source changes there.
  **Verify:** `npx --yes skills-ref validate skills/bridgectl-ops`;
  `go test ./internal/cli ./internal/idp ./internal/mcp`;
  `make test`; JSON/YAML/link/secret audits; `git diff --check`; and a clean
  tracked working tree after committing only the intended skill, agent-guidance,
  plan, and changelog changes. The temporary installed tree must contain every
  embedded source file except ignored `evals/`.

## Verification

- `skills-ref` accepts the unchanged skill name and model-invoked frontmatter.
- The core remains concise and routes detailed work to the correct reference.
- File-backed credentials are described as a per-resource, fail-closed source;
  no instruction prints values, sources `.env`, or treats a file rotation as an
  environment rotation.
- Generation guidance covers both unchanged stdout drafts and
  `--output-dir` split files, with exact-name filenames and explicit review
  before apply.
- Annotation fields and `io.erpbridge/whenToUse`,
  `io.erpbridge/whenNotToUse`, `io.erpbridge/examples`, and
  `io.erpbridge/allowedRoles` are documented as optional, informational values;
  server-side authorization remains the only permission check.
- Plugin lifecycle guidance keeps exact versions, endpoint allowlists,
  cache-safety behavior, redaction, and deployment ownership correct.
- The embedded binary installs the same validated references and assets as the
  source skill, while local evaluation files remain excluded.
- Focused Go tests, `make test`, package validation, audits, and diff checks are
  green. `AGENTS.md` now matches the supported credential-source contract; no
  runtime, public API, or unrelated skill behavior changes.
- Static evaluation validation passed for three behavior prompts and 20
  trigger cases (10 positive, 10 near-miss negatives; 12 train, 8 validation).
  Dynamic with-skill/old-skill comparison was deferred because the `claude` CLI
  is unavailable in this environment.

## Open Questions

None. The repository skill is the authority; the locally installed skill is a
post-validation derived copy.
