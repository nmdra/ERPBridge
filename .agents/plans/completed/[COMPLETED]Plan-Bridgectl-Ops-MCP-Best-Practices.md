# Plan: Bridgectl operations skill, MCP and Agent Skills alignment

## Goal

Make `bridgectl-ops` a concise, portable operations skill for CLI and management MCP use. Preserve security, confirmation, and optional memory gates. Add MCP usage, scope preflight to the selected deployment and action, and load operational memory only when relevant. Correct confirmed content and portability defects. Do not claim measured performance gains from static improvements.

## Current state and evidence

- The main skill has no management MCP route and ends with an empty handoff section (`skills/bridgectl-ops/SKILL.md:1-145`). Its metadata version is 3.4.1.
- The ecosystem reference already distinguishes management from runtime and requires write readback (`skills/bridgectl-ops/references/ecosystem.md`, Management MCP server). Extend that pointer instead of duplicating its detailed guidance.
- The management contract defines launch-time pinning, write groups, confirmation targets, envelopes, redacted inspection, unknown outcomes, and serial execution (`docs/bridgectl-mcp.md:32-232`). Discover the live catalogue rather than hard-code its size.
- Embedded skill tests cover bundle contents and installation (`internal/cli/skill_test.go:16-27,72-99`). The embed directive already includes reference files (`skills/bridgectl-ops/embed.go:6-12`).
- The active automatic-knowledge-checkpoint plan owns existing memory instructions and pending evaluations. Preserve the one-record terminal checkpoint, authority order, redaction, retrieval budgets, and absent-store behavior. This proposal changes retrieval routing only; it must not close or take over that plan.
- The user supplied `/home/nimendra/Downloads/bridgectl_ops_skill_review_2026-10-18.md`. Its scores and token estimates are qualitative/static, not measured runtime results. Repository inspection confirms its empty handoff, malformed local-demo prose, unconditional local Compose checks, and plugin links that escape the distributable skill tree (`skills/bridgectl-ops/references/onboarding.md`, Preflight; `references/plugins.md`, Boundary and prerequisites / Apply and verify).
- Operations and diagnostics currently route narrow reads through the full onboarding preflight (`references/operations.md`, Inspect before changing; `references/diagnostics.md`, Triage loop). The main skill loads the complete memory reference whenever the optional store exists (`SKILL.md`, Use accumulated operational knowledge).
- Agent Skills requires matching names, a description of at most 1024 characters, compatibility of at most 500 characters, and string metadata. It recommends a short main file and focused relative references: https://agentskills.io/specification.
- Official guidance recommends domain-specific procedures, clear defaults, progressive disclosure with explicit loading conditions, real-execution gotchas, and validation loops: https://agentskills.io/skill-creation/best-practices.

## Decisions

1. Keep the existing model-invoked name and scope. Shorten the description and include management MCP as a trigger. Bump metadata to 3.5.0 for the added supported workflow.
2. Prefer an available authorized management MCP connection for catalogue-covered operations. Use CLI for capabilities MCP does not expose. Discover names and schemas rather than assume client namespace names or a tool count.
3. Add a focused `references/mcp.md`. Keep only route selection and essential serial-call, result-checking, and confirmation rules in the main file. Include a host-neutral example and a Pi example with placeholder context and absolute executable path, not personal configuration.
4. Preserve separate runtime and management entries. Changing the host configuration requires approval. Never replace runtime access implicitly or enable write groups as a workaround.
5. Keep one management operation in flight. Inspect MCP isError and the JSON success/error.code fields. Restart after credential, endpoint, or grant changes; read back an unknown write outcome before considering a retry.
6. Inspection views are not applicable manifests. Generation is a separate executable-manifest path. Token creation, remote OpenAPI imports, and filesystem output remain CLI-only.
7. Preserve all existing confirmation/redaction/memory requirements. Complete the handoff with outcome, context/version, verification, remaining risk, and memory status. No new runtime behavior or agent hooks.
8. Validate the bundle with a temporary local build only. Do not replace the user's installed GitHub release binary. Refresh the installed skill only if the exact destination is approved.
9. Add `references/preflight.md` as the single preflight router: core selected-context/endpoint/credential/authorization/target evidence; remote control-plane checks; conditional local Compose checks; API-onboarding registry/manifest/probe checks; and plugin-binding prerequisites. Exact-action approval and separate post-change verification apply to every mutation. A registered API probe may cause downstream effects and still requires its MCP probe grant and explicit approval; do not use it as a universal read-only health check.
10. Keep a small in-session evidence table and reuse unchanged evidence within the same workflow. Recheck affected evidence after a mutation, changed context/endpoint/credential/revision, or new failure. Context listing and successful local validation alone do not prove remote authentication or ERP connectivity.
11. Replace eager memory loading with a short root routing contract. Read the optional index, select one relevant area, search exact identifiers, and stop on no match. Load detailed knowledge sections only for conflicts, record writing, consolidation, or evolution. Retain aggregate limits of five full knowledge entries and two historical records, untrusted-memory handling, and one redacted terminal record for each completed major task. The checkpoint still applies when retrieval found no match.
12. Replace repository-depth-dependent plugin links with installed CLI help and verified-checkout or published-doc fallbacks. Make all deployment commands use the selected explicit context; MCP calls use the pinned context instead. Add short task-to-section maps to long references. Make compatibility requirements conditional for local-only operations.
13. Use baseline/candidate fixture-driven dry runs for read-only MCP, auth/busy recovery, unknown writes, remote preflight without Docker, plugin portability/context, and lazy-memory/no-match/adversarial-memory cases. Fixtures are definitions until executed. Record observed pass/fail and limitations. Any paid model benchmark, trigger optimizer, live mutation, or performance study requires separate approval.

## Scope

In scope: repository skill; MCP and profiled-preflight references; conditional memory routing; onboarding, operations, plugin, diagnostics, ecosystem, and knowledge cross-references; installed-copy portability; format, link, and embedding regression tests; focused dry-run behavior cases; relevant developer and public docs; Unreleased changelog notes.

Out of scope: runtime or CLI API changes, a new executable preflight system, memory storage or checkpoint redesign, credentials, MCP host configuration, write grants, release publication, replacing the installed bridgectl release binary, unrelated plans and edits, automatic commits, paid trigger-optimization runs, and unapproved live mutations or performance benchmarks.

## Tasks

- [x] **Task 1: Correct root routing and add the management MCP workflow.**
  **Files:** `skills/bridgectl-ops/SKILL.md`, new `references/mcp.md`, `references/ecosystem.md`, `internal/cli/skill_test.go`.
  **Seam:** embedded main instructions and reference installation.
  **Sequence:** snapshot baseline; add an embedding regression and show red for the missing reference; add discovery-first MCP guidance, a sequential example, result/error handling, grants and readback, concise metadata, and a complete handoff. Preserve the checkpoint contract.
  **Verify:** `go test ./internal/cli -run 'TestSkill|TestInstallEmbeddedSkill' -count=1`; check metadata limits, complete headings, and serial example. Show the new reference installs byte-for-byte.

- [x] **Task 2: Scope preflight and make reference guidance portable.**
  **Files:** new `skills/bridgectl-ops/references/preflight.md`; `SKILL.md`; `references/onboarding.md`, `operations.md`, `plugins.md`, `diagnostics.md`; `internal/cli/skill_test.go`.
  **Seam:** selected-action routing and standalone embedded reference tree.
  **Sequence:** add core and conditional profiles, evidence reuse/invalidation rules, and narrow final verification. Remove unconditional localhost/Compose requirements for remote work. Fix malformed RedisInsight prose, escape-path links, and missing explicit context examples. Add contents maps where useful.
  **Verify:** `go test ./internal/cli -run 'TestSkill|TestInstallEmbeddedSkill' -count=1`; audit every relative Markdown target from a temporary installed skill; inspect installed `bridgectl plugin --help`, `plugin binding --help`, and root context support. Dry-run remote diagnostics and cache administration without Docker, retaining exact-action confirmation and post-change readback. Record scenario evidence, not just fixture existence.

- [x] **Task 3: Make memory retrieval lazy without weakening safety.**
  **Files:** `skills/bridgectl-ops/SKILL.md`, `references/knowledge.md`; local ignored evaluation cases.
  **Seam:** optional index lookup, exact-match retrieval, and terminal checkpoint.
  **Sequence:** replace full-manual-first loading with the short routing contract; add a contents map and section pointers; exercise no-match, matched, stale, malicious, absent-store, and terminal-record cases against baseline and candidate instructions.
  **Verify:** parse local evaluation JSON with unique IDs; record dry-run outputs showing bounded retrieval and retained authority/redaction/checkpoint rules; compare candidate against the existing checkpoint plan. No task is considered behavior-tested merely because its fixtures parse.

- [x] **Task 4: Synchronize docs and run final acceptance.**
  **Files:** `docs/bridgectl-mcp.md`, `CHANGELOG.md`, public `docs/bridgectl/skills.mdx`, `docs/bridgectl/mcp.mdx`, and `CHANGELOG.md` in the sibling `erpbridge-docs` repository; plan evidence updates.
  **Seam:** source guidance, distributable install, and public documentation.
  **Verify:** `make test`; `git diff --check` in both repositories; active diagnostics on changed Go files; temporary CLI build and `skill install --dir <fresh temporary directory>` with byte comparison; public docs `npm run build`; independent safety/portability review. Report pass/fail, unrun checks, and evaluation limits separately. Refresh the installed skill only after destination approval. Do not claim precision/recall, speed, or token savings without executed controlled measurements.

## Acceptance evidence

- Added regressions failed first for the missing MCP reference and empty handoff. Installed-link tests then failed for plugin links escaping both global and project bundles. All focused skill/install tests now pass.
- `quick_validate.py skills/bridgectl-ops`: valid; frontmatter limits and string metadata checked; main file stays below 500 lines.
- Local evaluation JSON parses with unique IDs. Baseline and candidate policy outputs cover nine scenarios, saved in ignored `skills/bridgectl-ops/evals/workspace/iteration-1/`. Independent Task 2 acceptance also passed remote logs, pending cache approval, and plugin staging dry runs. These are policy simulations, not live execution or measured performance. No trigger precision/recall is claimed.
- Baseline evaluation initially lacked access to its temporary snapshot; an exact copy was supplied under the ignored evaluation directory and the run completed. Candidate evaluation found ambiguous wording in one fixture; it was clarified to prohibit repository-depth-dependent links, not approved external documentation.
- Independent review found no blockers and three documentation concerns: manifest unknown-field validation is resource-specific, retained free text/enums still need redaction review, and cache/context writes have distinct result shapes. All three corrections were made in bundled and relevant developer/public docs without runtime changes.
- `env -u BRIDGE_API_TOKEN make test`: passed. An initial plain `make test` failed because the inherited bridge-token variable overrode fixture credentials in two existing web tests. The variable was removed only from the test subprocess; no saved credentials changed. Temporary failure-log authorization values were redacted.
- Public docs `npm run build`: passed after final edits. Existing missing-blog and Node localStorage warnings remain non-fatal.
- Active LSP full probe of changed Go test file: zero diagnostics. Repository-wide lint was not run; only tests/documentation changed.
- Temporary helper built and installed to `/tmp/bridgectl-ops-review.Y6jTCp/final-installed/bridgectl-ops`; every source instruction/reference/asset matched byte-for-byte. Evaluation files stayed excluded. Local Markdown files and anchors passed portability checks.
- `git diff --check` passed in both repositories. SHA-256 comparison confirms `/home/nimendra/.local/bin/bridgectl` unchanged.
- Static qualitative comparison viewer: `/tmp/bridgectl-ops-review.Y6jTCp/policy-review.html`. No benchmark gain or runtime safety outcome is inferred from policy text.
- Source implementation is complete. After explicit global-destination approval, synchronized `/home/nimendra/.agents/skills/bridgectl-ops` with a temporary source-built helper and backup. All 16 installed files match source version 3.5.0; the installed release CLI checksum remains unchanged. No live ERP operation, MCP configuration change, grant expansion, or release publication occurred.

## Approval and delivery

Approved for source implementation by the user's "now update skill" request; promoted to active before edits. The user subsequently approved replacing the global skill destination, and installation completed with backup and byte verification. `/home/nimendra/.local/bin/bridgectl` remains the GitHub release binary. The user approved two cohesive commits, one per repository, and direct pushes to both `main` branches instead of separate commits per plan task. Leave unrelated existing plans and local execution records uncommitted.
