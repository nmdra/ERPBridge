# Bridgectl operational knowledge

Search the most specific directory with exact error codes, resource kinds,
operations, resolution codes, and versions. Do not read the complete tree by
default. Read `skills/bridgectl-ops/references/knowledge.md` for the authority
order, retrieval budget, redaction rules, consolidation loop, and proposal gate.

| Area | Directory |
| --- | --- |
| API registration/testing | `knowledge/api/` |
| MCP tools/manifests | `knowledge/tools/` |
| Plugins/bindings | `knowledge/plugins/` |
| Authentication/roles | `knowledge/auth/` |
| Cache/rotation | `knowledge/cache/` |
| Runtime/server | `knowledge/runtime/` |
| Diagnosis/reporting | `knowledge/diagnostics/` |

Execution records are append-only monthly JSONL files under `executions/`.
Skill proposals live under `evolution/proposals/`. Evaluated proposals are
recorded append-only in `evolution/skill-impact.jsonl`.
