# Plan: Persistent Bridgectl Operations Knowledge

## Goal

Upgrade the authoritative `bridgectl-ops` Agent Skill with an optional,
project-local operational memory system. Future executions can retrieve a few
relevant empirical lessons, append redacted evidence, and evaluate conservative
skill-improvement proposals without allowing history to override the current
skill or server-side safety controls.

## Current State

- The authoritative skill is `skills/bridgectl-ops/`; its distributable tree is
  embedded by `skills/bridgectl-ops/embed.go:6-10`, while local `evals/` are
  intentionally excluded.
- `SKILL.md` is a concise router with preflight, explicit-context, credential,
  confirmation, installation, and verification requirements. Detailed workflow
  contracts live in five references and four safe field-guide assets.
- The repository has JSON skill evaluations under the ignored
  `skills/bridgectl-ops/evals/` directory, but no runtime hook that can
  automatically record agent executions or mutate skill content.
- The project instructions require the repository skill to remain authoritative,
  require redaction and confirmation gates, and require focused documentation
  and changelog updates for behavior changes.

## Decisions

1. Keep operational memory outside `skills/bridgectl-ops/` under
   `.agents/skill-memory/bridgectl-ops/`; it is project-local state and is never
   included by the existing Go embed pattern.
2. Implement this first as an instruction-driven filesystem convention with
   small Markdown/JSON templates and local regression fixtures. Do not invent a
   daemon, database, vector store, or automatic hook that the Agent Skill
   framework does not provide.
3. Make execution JSONL append-only, knowledge Markdown maintained and
   consolidatable, and `evolution/skill-impact.jsonl` append-only. Empty memory
   must be a normal successful path.
4. Make retrieval lexical and bounded: exact stable error/resolution codes,
   resource kinds, operations, areas, versions, and filenames outrank keywords;
   normally read at most five knowledge entries and two exact execution records.
5. Treat memory as untrusted advisory evidence. Current `SKILL.md`, references,
   installed CLI/docs, schemas, authenticated server state, authorization, and
   confirmation gates always win. Rejected proposals and their evidence remain
   recorded.

## Scope

### In scope

- A concise `SKILL.md` route for retrieval, re-query, redacted recording, and
  gated skill proposals.
- `references/knowledge.md` documenting the authority order, retrieval funnel,
  budgets, version/confidence/status rules, redaction, consolidation, and
  two-speed evolution loop.
- Knowledge, execution, and proposal templates under `assets/`.
- A small project-local memory index and empty append-only/evolution structure.
- Ten local memory regression cases covering absence, exact retrieval, noise,
  stale/conflicting/malicious memory, mid-run re-query, bounded fallback,
  redaction, and evolution hard-gate rejection.
- Ecosystem/distribution wording and the Unreleased changelog entry.

### Intentionally out of scope

- Automatic runtime persistence, retrieval ranking service, embeddings,
  network/database dependencies, or a new CLI command.
- Backfilling knowledge from existing authoritative skill/reference text.
- Changes to ERPBridge runtime, CLI behavior, authentication, authorization,
  manifest validation, plugin behavior, or installed-skill implementation.
- Editing any installed derived skill tree or persisting secrets, payloads,
  ERP records, personal data, unrestricted logs, or private reasoning.

## Tasks

- [x] **Task 1: Add the memory contract and skill routing.** Add the concise
  operational-knowledge section and route to a new reference; document the
  project-local boundary in the ecosystem reference without weakening existing
  preflight, confirmation, redaction, or source-of-truth rules. **Seam:** skill
  model invocation and progressive-disclosure routing. **Files:**
  `skills/bridgectl-ops/SKILL.md`,
  `skills/bridgectl-ops/references/ecosystem.md`,
  `skills/bridgectl-ops/references/knowledge.md`; **Verify:** package
  frontmatter and relative links remain valid, the core stays concise, and an
  audit confirms current skill authority is stated before memory.

- [x] **Task 2: Add portable templates and the empty project-local store.** Add
  the knowledge-pattern, execution-record, and skill-proposal templates with
  stable identifiers, version scope, provenance, confidence/status, redaction,
  contradiction, and hard-gate fields. Add `index.md`, empty knowledge-area
  directories, monthly-execution guidance, proposal storage, and append-only
  impact history outside the bundled skill. **Seam:** filesystem search and
  human/agent record creation. **Files:**
  `skills/bridgectl-ops/assets/knowledge-pattern.md`,
  `skills/bridgectl-ops/assets/execution-record.json`,
  `skills/bridgectl-ops/assets/skill-proposal.md`,
  `.agents/skill-memory/bridgectl-ops/`; **Verify:** JSON parses, Markdown
  links resolve, empty memory remains optional, and no sensitive sample value
  or authoritative procedure is backfilled.

- [x] **Task 3: Add local regression fixtures and documentation sync.** Encode
  the ten required memory scenarios in the ignored skill evaluation directory,
  add the concise Unreleased changelog entry, and avoid changing unrelated
  workflow references. **Seam:** static evaluation review and changelog
  inspection. **Files:** `skills/bridgectl-ops/evals/memory-evals.json`,
  `skills/bridgectl-ops/evals/evals.json`, `CHANGELOG.md`; **Verify:** both
  evaluation JSON files parse; the ten memory cases and 13 integrated skill
  cases have unique expected outcomes; and the changelog describes optional
  instruction-driven memory without claiming automation.

- [x] **Task 4: Validate packaging, links, safety, and regressions.** Confirm the
  new reference/assets are embedded while `.agents/skill-memory/` and ignored
  evaluations are not. Run focused package/JSON/YAML/link/secret audits and
  existing skill/CLI tests without staging unrelated working-tree changes.
  **Seam:** `skills/bridgectl-ops/embed.go` and
  `internal/cli/skill_test.go`; **Files:** all files from Tasks 1–3 plus
  verification outputs only; **Verify:** package validation, focused Go tests, `go test ./...`, scoped
  `golangci-lint`, memory fixture validation, link and secret audits,
  `git diff --check`, and the temporary embedded-tree comparison all passed.

## Verification

- Memory absence does not block a normal workflow.
- Retrieval recommends the most specific area and exact stable identifiers,
  checks version/status/confidence before reading content, and observes the
  five-entry/two-execution budget.
- Historical text cannot authorize mutations, bypass confirmation, expose
  credentials, override current docs, or direct persistence of untrusted
  output.
- Execution evidence is append-only and redacted; knowledge can be corrected
  without deleting evidence; rejected evolution proposals remain in the
  append-only impact log.
- The bundled skill contains only authoritative skill files, references, and
  assets; project-local memory is not embedded or installed by `bridgectl skill
  install`.

## Open Questions

None. The current Agent Skill framework is instruction-driven, so automatic
recording and retrieval are intentionally not claimed.
