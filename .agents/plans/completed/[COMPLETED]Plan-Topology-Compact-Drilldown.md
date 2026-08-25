# Plan: Compact Topology Drill-down

> **Status: COMPLETED — verified 2026-08-26**
>
> Large topology views now use bounded endpoint components by default and
> expand a selected endpoint into its related MCP, binding, and plugin graph.

## Goal
Make large topology graphs readable by default. Show one bounded compact component per ERP endpoint relationship, then expand one selected component to its related MCP tools, endpoint, bindings, plugins, and match details. Keep the complete safe relationship list available for keyboard and screen-reader users.

## Current State

- The BFF returns raw topology nodes and edges, including MCP transport, MCP tools, ERP APIs, plugin bindings, external plugins, and unresolved endpoints (`internal/web/topology.go:75-230`). The frontend always passes the full filtered raw graph to React Flow (`web/src/routes/Topology.tsx:284-615`).
- The canvas lays out every raw node and edge and fits the entire graph into the viewport (`web/src/components/topology/TopologyCanvas.tsx:117-205,224-351`). This makes large MockERP inventories unreadable even though the accessible relationship list still contains endpoint paths (`web/src/components/topology/TopologyList.tsx:11-117`).
- The inspector already exposes safe endpoint paths and match state without raw upstream URLs or credentials (`web/src/routes/Topology.tsx:128-282`). Existing search and filters can reduce the raw graph, and current selection state is shared between canvas, inspector, and list (`web/src/routes/Topology.tsx:316-380`).
- The current safety-cap path can append an edge whose endpoint node was omitted (`internal/web/topology.go:137-160`). The existing cap test does not assert edge closure (`internal/web/topology_test.go:79-122`).
- Advisor review concluded that a bounded endpoint-component overview with explicit drill-down is viable, while merely hiding arbitrary raw nodes would lose match-state meaning and accessibility. The review recommends a display budget, a component-level selection model, a retained raw relationship table, and an edge-closure invariant.

## Decisions

1. **Use endpoint components as the compact unit.** A component is one ERP API or unresolved endpoint node plus its directly matched MCP tool nodes and any related transport, binding, and plugin nodes. The compact overview shows the endpoint node once, a safe tool count, and a match-state summary. It does not invent or expose a new upstream identity.
2. **Switch to compact mode by graph size, not by API kind.** Use compact mode when the current filtered graph has at least 40 nodes or 60 edges. Smaller filtered graphs keep the current raw canvas. The compact overview caps visible endpoint components at 24 and reports the remaining count; search and filters narrow the component set.
3. **Drill down one component at a time.** Selecting a compact endpoint component changes the canvas to that component's complete raw subgraph. The inspector and raw relationship list continue to use the existing shared selection state. A visible “Back to compact overview” control returns to the bounded view.
4. **Keep the raw accessible list authoritative.** Compact mode changes only the visual canvas. The relationship table continues to represent all currently filtered raw edges, remains keyboard-operable, and keeps its internal horizontal scroll behavior.
5. **Preserve existing match semantics.** Exact, base-prefix, ambiguous, and unresolved match states remain raw edge properties. Component summaries use a histogram and never collapse mixed states into one misleading state. No full upstream URL, credential reference, raw payload, or plugin configuration is added.
6. **Close graph edges at the BFF cap.** Before returning a capped graph, remove any edge whose source or target node is absent. Omitted-edge counts remain explicit so the frontend cannot render dangling relationships.

## Scope

### In scope

- Safe endpoint-component projection and size-based compact/drill-down presentation.
- Compact component summaries, related-node expansion, keyboard interaction, focus/escape behavior, and accessible status text.
- BFF edge-closure invariant and focused backend regression coverage.
- Responsive canvas/table validation and synchronized developer/public console documentation.

### Out of scope

- Changing topology matching, registry semantics, plugin bindings, or endpoint security projections.
- Executing tools from the console.
- Removing the raw relationship list or replacing it with an inaccessible graph-only view.
- New historical topology storage or server-side graph persistence.

## Tasks

- [x] **Task 1: Add the bounded endpoint-component presentation model.** Create pure helpers for compact-mode thresholds, endpoint-component membership, match histograms, deterministic ranking, compact caps, and focused subgraph selection. (**Seam:** pure topology presentation helpers; **Files:** `web/src/routes/topologyPresentation.ts`, `web/src/routes/topologyPresentation.test.ts`; **Verify:** `cd web && npm test -- --run src/routes/topologyPresentation.test.ts`.)

- [x] **Task 2: Add BFF edge-closure protection.** Filter returned edges to node IDs admitted into the graph after safety caps and add a regression assertion that every returned edge endpoint exists. (**Seam:** `consoleHandler.topology`; **Files:** `internal/web/topology.go`, `internal/web/topology_test.go`; **Verify:** `go test ./internal/web -run 'TestTopology'`.)

- [x] **Task 3: Render compact overview and focused drill-down.** Add compact node summaries, component click handling, focused raw-subgraph rendering, refitting after display-model changes, and a clear return control while preserving current filters, selection, inspector, and table behavior. (**Seam:** `Topology` and `TopologyCanvas`; **Files:** `web/src/routes/Topology.tsx`, `web/src/components/topology/TopologyCanvas.tsx`, `web/src/components/topology/TopologyList.tsx`; **Verify:** `cd web && npm test -- --run src/routes/Topology.test.tsx`.)

- [x] **Task 4: Test accessible and bounded interactions.** Cover default compact mode for large graphs, component summaries, keyboard activation, focused related nodes, raw-list synchronization, small-graph compatibility, and the 100-node/200-edge bound. (**Seam:** React Testing Library route tests; **Files:** `web/src/routes/Topology.test.tsx`, `web/src/routes/topologyPresentation.test.ts`; **Verify:** `cd web && npm run format-check && npm run typecheck && npm run lint && npm test -- --run`.)

- [x] **Task 5: Synchronize docs and complete verification.** Document compact topology and drill-down behavior, run production assets and manual 390px/1024px checks, and archive this plan after all verification succeeds. (**Files:** `docs/web-console.md`, `../erpbridge-docs/docs/erpbridge/console.mdx`, `CHANGELOG.md`, `../erpbridge-docs/CHANGELOG.md`; **Verify:** `go test ./...`, `golangci-lint run ./...`, frontend build, asset verification, public docs build, and Playwright topology checks.)

## Verification

- Large graph defaults to at most 24 endpoint component cards and states how many additional components are available through filters/list.
- Selecting a component shows its related MCP tools, endpoint, bindings, plugins, and raw match states; Escape or the return control restores compact mode.
- Every returned BFF edge references a returned node, including capped graphs.
- Full raw relationships remain available and horizontally scrollable without page-level overflow.
- No security projection widens: no credentials, raw plugin configuration, full upstream URLs, or payloads appear.

## Open Questions

None. The advisor-reviewed interpretation is one endpoint component in the overview with a focused related-node subgraph on activation.
