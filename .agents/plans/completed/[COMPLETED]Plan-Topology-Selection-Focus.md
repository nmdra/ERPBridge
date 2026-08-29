# Topology Selection Focus

## Goal

When a user selects an MCP tool, show only its direct execution path in the
canvas rather than rendering unrelated component members in a dimmed state.

## Tasks

1. Add a focused-view-model test for a selected MCP tool with ERP and plugin
   relationships. Verify the canvas graph contains only MCP transport, the
   selected tool, ERP API, exact ERP endpoint, plugin binding, and plugin.
2. Implement selection-path pruning in the focused topology view model. Keep
   the complete accessible relationship table unchanged.
3. Update topology console documentation and run focused frontend tests,
   typecheck, lint, format check, and `make test`.

## Completion

Prefix this file with `[COMPLETED]` and move it to `../completed/` after all
verification commands pass.
