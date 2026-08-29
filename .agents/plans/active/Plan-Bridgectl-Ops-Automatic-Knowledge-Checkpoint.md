# Plan: Automatic Major-Task Knowledge Checkpoint

## Goal

Make the model-invoked `bridgectl-ops` skill explicitly save one redacted,
append-only execution record after each completed major ERPBridge operations
workflow, then consolidate reusable evidence when justified. Keep the current
skill, references, server state, authorization, validation, and confirmation
gates authoritative. Make the instruction portable across Agent Skills hosts;
do not claim that Markdown alone provides a guaranteed runtime hook.

## Current State

- The skill is model-invoked because it has a `description` and does not set
  `disable-model-invocation`; its description currently names operational
  knowledge reuse but not the post-major-task recording branch
  (`skills/bridgectl-ops/SKILL.md:1-8`).
- The main skill tells the agent to append a redacted record after a workflow,
  but does not define “major task,” distinguish one task from turns/commands,
  or place the save as an explicit final checkpoint before handoff
  (`skills/bridgectl-ops/SKILL.md:37-54,102-112`).
- The reference defines the execution-record schema, append-only JSONL rules,
  redaction gate, and consolidation loop (`skills/bridgectl-ops/references/knowledge.md:183-254`).
  It currently says that the Agent Skill framework has no automatic
  post-execution hook and that recording is an agent-followed convention
  (`skills/bridgectl-ops/references/knowledge.md:307-314`).
- Existing regression coverage has 13 integrated skill cases and 10
  operational-memory cases, but no explicit successful-major-task,
  terminal-blocked-task, or non-major-task checkpoint cases
  (`skills/bridgectl-ops/evals/evals.json:1-142`,
  `skills/bridgectl-ops/evals/memory-evals.json:1-97`).
- Project-local memory is optional and intentionally excluded from the
  distributable skill (`skills/bridgectl-ops/embed.go:6-12`).

## Research Findings

### Facts

1. The Agent Skills specification requires `description` to state what a skill
   does and when to use it, and loads the body only after activation. The
   description is therefore the automatic discovery pointer, not a completion
   callback: [Agent Skills specification](https://agentskills.io/specification).
2. The official description guidance says descriptions should be specific,
   imperative, and explicit about trigger contexts; it recommends realistic
   positive and near-miss negative queries, repeated runs, and a held-out split:
   [Optimizing skill descriptions](https://agentskills.io/skill-creation/optimizing-descriptions).
3. Pi scans names and descriptions at startup and asks the model to load the
   full `SKILL.md` when a task matches. Models may miss a match, while
   `/skill:name` is the reliable explicit fallback
   (`/home/nimendra/.local/lib/node_modules/@earendil-works/pi-coding-agent/docs/skills.md:65-72`).
4. Pi exposes lifecycle events through extensions. `turn_end` fires once per
   LLM turn, while `agent_settled` fires when retries, compaction, and queued
   follow-ups will not continue automatically
   (`/home/nimendra/.local/lib/node_modules/@earendil-works/pi-coding-agent/docs/extensions.md:567-613`).
   That is a host extension seam, not a portable Agent Skill feature, and it
   cannot infer the semantic boundary of a “major” task from an event alone.
5. Claude Code’s official hooks provide a separate deterministic lifecycle
   mechanism, including `Stop` and `TaskCompleted`, but those hooks are
   Claude-Code-specific and are not part of the standard fields used by this
   skill: [Claude Code hooks](https://docs.anthropic.com/en/docs/claude-code/hooks).
   Its skill documentation also confirms that `disable-model-invocation: true`
   prevents automatic loading, so it should remain absent here:
   [Claude Code skills](https://docs.anthropic.com/en/docs/claude-code/skills).
6. WikiSkill supports separating raw execution experience, accumulated
   knowledge, and executable skills, and reports that persistent knowledge is
   important to skill evolution. It is a design reference only, not ERPBridge
   operational authority: [WikiSkill, arXiv:2608.27454](https://arxiv.org/abs/2608.27454).

### Consequence

Use a strong, explicit completion instruction in `SKILL.md` as the recommended
portable behavior. Do not add a Pi extension, daemon, database, vector store,
or host-specific hook in this change. If a future requirement is a hard
machine guarantee, design that as a separately approved Pi extension with a
clear redaction and permission model; do not imply that the skill instruction
itself is guaranteed.

## Decisions

1. **Keep automatic model discovery enabled.** Preserve the existing
   model-invoked skill and add “record operational knowledge after every major
   ERPBridge workflow” as a distinct trigger branch in the frontmatter
   description. Do not add `disable-model-invocation`.
2. **Use a task-level completion checkpoint.** A major task is a completed
   multi-step workflow in this skill: onboarding; API/tool/plugin/binding
   lifecycle work; token, role, or cache administration; a multi-step
   diagnosis; bundled-skill distribution; or a cross-system operation that
   reaches verification or a terminal blocked/unresolved/abandoned outcome.
   A single read, explanation, command, or an operation still waiting for user
   confirmation is not a completed major task.
3. **Write one record per major task.** Do not write one record per command,
   tool call, turn, retry, or subtask. Record `success`, `resolved`,
   `unresolved`, `blocked`, or `abandoned` only when the task reaches its
   terminal handoff.
4. **Make the checkpoint the last operational step.** Verify the result first,
   run the redaction gate, append exactly one JSONL record, consolidate only a
   reusable lesson, and then produce the final handoff. A missing memory store
   or safe append failure never blocks the operational handoff; it must be
   reported as not recorded rather than invented.
5. **Keep memory advisory and optional.** Do not create the memory tree
   silently, let memory override current sources, persist sensitive material,
   or require a record when the memory store is absent. Preserve the existing
   authority order and all change gates.
6. **Bump the skill patch version.** Change the source metadata from `3.4.0`
   to `3.4.1`, rebuild the embedded distribution through the normal build, and
   synchronize the public documentation repository and Unreleased changelog.

## Scope

### In scope

- A clearer model-invocation description and explicit major-task checkpoint in
  `skills/bridgectl-ops/SKILL.md`.
- Reference wording that aligns the append, redaction, consolidation, and
  no-hook limitations with the new checkpoint.
- Local ignored trigger and behavior evaluations for save-after-success,
  save-after-terminal-block, no-save-for-minor-work, and absent-memory cases.
- Skill patch-version metadata, embedded-skill validation, and synchronized
  public documentation.

### Out of scope

- A Pi extension or Claude Code hook implementation.
- Automatic creation of `.agents/skill-memory/bridgectl-ops/`.
- Runtime changes to `bridgectl`, ERPBridge authorization, confirmation gates,
  or memory storage.
- Persisting prompts, payloads, records, unrestricted logs, credentials,
  private reasoning, or any other sensitive execution content.
- Editing installed derived skill copies by hand.

## Tasks

- [x] **Task 1: Add the main-skill completion checkpoint.** Extend the
  model-facing description with the major-workflow recording branch; replace
  the generic post-workflow sentence with a pointer to a named checkpoint; and
  add a concise ordered checkpoint before the final handoff. Define major-task
  boundaries, terminal outcomes, one-record-per-task behavior, the optional
  memory-store branch, and the safe failure report. Keep the checkpoint
  after observable verification and before the final summary. Bump the skill
  metadata from `3.4.0` to `3.4.1` in the same source change. **Files:**
  `skills/bridgectl-ops/SKILL.md`. **Seam:** loaded `SKILL.md` instructions at
  the verified workflow-to-handoff boundary. **Verify:** inspect the rendered
  Markdown and assert that it contains the major-task definition, one-record
  rule, redaction gate pointer, absent-store behavior, and final-handoff
  ordering; confirm frontmatter has no `disable-model-invocation`, the
  description remains under 1024 characters, and metadata is `3.4.1`.

- [x] **Task 2: Align the operational-memory reference.** Update
  `references/knowledge.md` so the detailed append/consolidation guidance
  explicitly supports every terminal major-task outcome, distinguishes a
  task from turns and commands, and states that the instruction is a
  best-effort checkpoint rather than a runtime hook. Preserve the existing
  schema, append-only behavior, redaction gate, authority order, retrieval
  budgets, and evolution hard gates. **Files:**
  `skills/bridgectl-ops/references/knowledge.md`. **Seam:** main-skill
  checkpoint pointer → memory reference → execution-record template.
  **Verify:** review the reference against
  `assets/execution-record.json`, `assets/knowledge-pattern.md`, and the
  current authority/redaction sections; confirm no sensitive field or unsafe
  fallback is introduced.

- [ ] **Task 3: Add checkpoint regression and trigger coverage.** Add realistic
  cases to the existing ignored evaluation files: a successful major workflow
  must save one redacted record; a terminal blocked or abandoned workflow must
  still record its bounded outcome; a minor explanation must not claim a
  record; and an absent memory store must not block or invent evidence. Add
  positive and near-miss negative trigger prompts for requests to record
  operational knowledge after ERPBridge work, keeping train/validation
  coverage balanced. **Files:**
  `skills/bridgectl-ops/evals/evals.json`,
  `skills/bridgectl-ops/evals/memory-evals.json`,
  `skills/bridgectl-ops/evals/description-trigger-evals.json` (local ignored
  evaluation artifacts; do not embed or commit them). **Seam:** skill
  activation decision and final workflow handoff. **Verify:** parse all three
  JSON files, assert unique IDs and balanced trigger labels, and run the
  description evaluator with three runs per query and a held-out split:

  ```bash
  cd /home/nimendra/.agents/skills/skill-creator
  python -m scripts.run_loop \
    --eval-set /home/nimendra/Documents/Projects/ERPBridge/skills/bridgectl-ops/evals/description-trigger-evals.json \
    --skill-path /home/nimendra/Documents/Projects/ERPBridge/skills/bridgectl-ops \
    --model "$PI_PROVIDER/$PI_MODEL" \
    --runs-per-query 3 \
    --holdout 0.4 \
    --max-iterations 1 \
    --report none \
    --verbose
  ```

  **Verification blocker:** the evaluator invocation was attempted, but this
  environment has no `claude` executable. JSON parsing, unique-ID checks, and
  train/validation balance pass; model-trigger scoring remains unverified.

- [x] **Task 4: Synchronize the public documentation.** Update the public
  Skill usage page to describe the completion checkpoint, optional memory
  behavior, and its non-guaranteed instruction boundary; add an Unreleased
  changelog entry; and keep the public version aligned with source `3.4.1`.
  Do not hand-edit an installed copy. **Files:**
  `/home/nimendra/Documents/Projects/erpbridge-docs/docs/bridgectl/skills.mdx`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/CHANGELOG.md`. **Seam:**
  source skill metadata and instructions → public docs. **Verify:** compare
  the public wording with the source skill and run `npm run build` in
  `/home/nimendra/Documents/Projects/erpbridge-docs`.

- [ ] **Task 5: Validate the distributable skill and repository gates.** Confirm
  the changed source is embedded, evaluation files remain excluded, no
  generated files are staged, and all existing safety and memory regressions
  remain green. **Files:** no planned file changes; inspect
  `skills/bridgectl-ops/embed.go` and `internal/cli/skill_test.go`. **Seam:**
  `bridgectl skill install` embedded filesystem and focused Go tests.
  **Verify:**

  ```bash
  go test ./internal/cli -run 'TestSkillEmbeddedFiles|TestInstallEmbeddedSkill'
  go test ./...
  make build
  make lint
  git diff --check
  ```

  Confirm the installed-skill comparison uses a fresh build and never edits a
  pre-existing installed destination directly.

## Verification

- A normal ERPBridge operations prompt still discovers the model-invoked skill.
- A prompt explicitly requesting a redacted post-major-task record also
  discovers the skill; unrelated code tasks remain near-miss negatives.
- Every completed major workflow attempts exactly one bounded append before the
  final handoff, including terminal blocked/unresolved outcomes.
- Minor one-step explanations do not create or claim execution evidence.
- Missing memory leaves the workflow operational and produces no invented
  record; append failures are reported honestly.
- Redaction, authority ordering, retrieval budgets, authorization, validation,
  and confirmation gates are unchanged.
- The source metadata is `3.4.1`, the embedded skill contains the source
  checkpoint, and no installed derived skill is hand-edited.
- `go test ./...`, `make build`, `make lint`, and the public docs `npm run build`
  pass.

## Open Questions

None for the recommended portable instruction-based implementation. A strict
machine-enforced save would require a separately approved host extension and a
new security/permission design; it is intentionally outside this plan.
