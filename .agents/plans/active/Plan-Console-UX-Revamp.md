# Plan: ERPBridge Console Production UX Revamp

## Goal

Make the read-only ERPBridge Console easier to investigate and safer to operate in
production. The primary outcome is a topology workspace that clearly separates
MCP tools, ERP APIs, bindings, plugins, and unresolved endpoints; supports useful
faceted filtering; and lets an operator select a node or edge and understand its
exact safe relationship without exposing credentials, raw configuration, or
upstream URLs.

The revamp must preserve loopback-only access, per-launch capability protection,
server-side upstream authentication, read-only behavior, and the CLI as the
mutation interface.

## Current State

- `web/src/routes/Topology.tsx:15-183` stores only a selected `TopologyNode`,
  computes search visibility inline, exposes static category counts, and sends a
  second filter path to `TopologyList`. Only ERP API summary items are selectable.
  There is no edge inspector or shared selection model.
- `web/src/components/topology/TopologyCanvas.tsx:55-81,96-208` renders
  focusable custom nodes, but does not pass selected state into nodes or edges.
  Edges carry `matchKind` only in the route data and are rendered without labels,
  click inspection, or relationship highlighting. The canvas remounts when node
  IDs change and schedules a delayed `fitView` on every initialization.
- `web/src/components/topology/TopologyList.tsx:13-74` filters again from the
  raw node set and renders edge rows as plain table text. It does not expose
  selection state or keyboard actions for relationships.
- `web/src/hooks/useTopology.ts:5-45` models nodes and edges but has no shared
  filter, selection, truncation, or graph completeness metadata.
- `internal/web/topology.go:18-20,109-211` caps the graph at 500 nodes and 1,000
  edges and truncates while aggregating tools, APIs, plugins, and bindings, but
  does not report that truncation to the browser.
- `web/src/components/layout/AppShell.tsx:56-67,137-167` has exact-path
  navigation state and a fixed header layout; subroutes require explicit
  production-grade active-state and responsive checks.
- The pinned React Flow dependency is `@xyflow/react` 12.9.0 in
  `web/package.json:19-30`.
- The completed safe BFF projections and read-only plugin separation are already
  in place. Plugin metadata is not part of the Tools inventory; plugin bindings
  are shown on the Plugins page and exact-version plugin details.

## Decisions

1. **One topology view model.** Derive search, category facets, match facets,
   visible nodes, visible edges, selected element, and counts once in
   `Topology.tsx`; pass the same result to the canvas, inspector, and accessible
   list. This prevents the current canvas/list disagreement.
2. **Discriminated ID selection.** Store `{ kind: "node" | "edge"; id: string }`
   or `null`, not object references. Clear selection on pane click/Escape and when
   filters remove the selected element. Selecting a node highlights that node,
   its directly connected edges and endpoints, and dims unrelated elements;
   selecting an edge highlights the edge and its source/target nodes.
3. **Facets are explicit and additive.** Provide text search plus multi-select
   facets for node category, edge match confidence (`exact`, `base-prefix`,
   `ambiguous`, `unresolved`), and context state. Show active-filter chips, visible
   node/edge counts, a clear-all action, and a visible incomplete-data warning.
4. **The accessible list is the semantic source of truth.** Canvas interaction
   enhances the list; it does not replace it. Node and edge rows are keyboard
   selectable, expose `aria-pressed`/selected state, and share the inspector
   selection with the canvas.
5. **Use React Flow's supported selection and labeling APIs.** The pinned
   library documents `onSelectionChange` for selected nodes and edges and
   `elevateEdgesOnSelect` for edge stacking:
   https://reactflow.dev/api-reference/react-flow. The official
   `useOnSelectionChange` documentation requires a memoized callback:
   https://reactflow.dev/api-reference/hooks/use-on-selection-change. Use direct
   `onNodeClick`/`onEdgeClick` plus controlled selected styling for the single
   selection inspector; use the selection callback for box/multi-selection only
   if it can be integrated without weakening the single-selection contract.
6. **Use labels sparingly and safely.** Render a short match label on edges and
   use a custom label only where it improves clarity. React Flow's
   `EdgeLabelRenderer` supports HTML labels but requires `pointerEvents: 'all'`
   and `nopan` for interactive labels:
   https://reactflow.dev/api-reference/components/edge-label-renderer. Labels
   contain only safe match metadata, never endpoint URLs or configuration.
7. **Profile before enabling visibility virtualization.** React Flow documents
   `onlyRenderVisibleElements` but warns that it adds overhead:
   https://reactflow.dev/api-reference/react-flow. Its performance guidance
   recommends memoized node/edge components, callbacks, arrays, and separate
   selection state:
   https://reactflow.dev/learn/advanced-use/performance. Memoize derived flow
   data and remove filter-triggered remounts first; benchmark the 100-node/200-
   edge target before deciding whether to enable the option.
8. **Report incomplete topology data.** Add safe truncation metadata to the BFF
   response so operators cannot mistake a capped graph for a complete graph.
   Counts must not include raw upstream data or credentials.

## Scope

### In scope

- Topology filters, selection state, edge labels, highlighting, inspector, and
  accessible list parity.
- Responsive topology layout and clearer category legends/empty states.
- React Flow memoization, stable viewport initialization, and a bounded scale
  fixture.
- Safe topology truncation metadata and documentation.
- Focused frontend/Go regression tests and synchronized changelog/docs.

### Out of scope

- Any topology mutation, tool invocation, plugin deployment, cache operation,
  or persistent console database.
- Exposing plugin endpoints, credentials, raw binding configuration, raw log or
  invocation payloads, or arbitrary proxy paths.
- Replacing React Flow with a canvas/WebGL renderer; the target graph remains
  within the current DOM-based scope.
- A historical metrics store, server-side saved filters, or cross-user state.

## Tasks

- [x] **Task 1: Define the shared topology view model and completeness contract.**
  Add discriminated selection, filter/facet types, visible graph derivation,
  result counts, stale-selection reset, and safe truncation metadata. Extend the
  Go topology response with `truncated` and omitted counts only when caps are
  reached; preserve existing node/edge data and read-only routing.
  (**Seam:** `useTopology` response and `Topology` derived state;
  **Files:** `internal/web/topology.go`, `internal/web/topology_test.go`,
  `web/src/hooks/useTopology.ts`, `web/src/routes/Topology.tsx`;
  **Verify:** `go test ./internal/web -run TestTopology`, focused topology
  frontend tests.)

- [x] **Task 2: Add production-grade topology facets and clear state.** Add a
  search input with a result count, category multi-select, match-confidence
  filter, context-state filter, active-filter chips, and clear-all control.
  Show disabled/empty facet states and a truncation warning. Ensure one
  memoized visibility derivation feeds the canvas, inspector, summary, and list.
  (**Seam:** topology filter state and visible graph derivation;
  **Files:** `web/src/routes/Topology.tsx`, `web/src/hooks/useTopology.ts`,
  `web/src/routes/Topology.test.tsx`, topology UI components if extracted;
  **Verify:** `npm test --prefix web -- --run src/routes/Topology.test.tsx`,
  `npm run typecheck --prefix web`, `npm run lint --prefix web`.)

- [x] **Task 3: Implement node and edge selection/highlighting.** Make canvas
  selection controlled by the shared ID model. Add node/edge click handling,
  pane/Escape clear behavior, selected and connected styles, dimmed unrelated
  nodes/edges, `elevateEdgesOnSelect`, accessible focus/selection announcements,
  and a safe inspector for node and edge semantics. The edge inspector must show
  source, target, direction, match kind, authority, context state, and safe
  endpoint paths where already projected; it must never show raw upstream URLs.
  (**Seam:** React Flow props and the existing topology inspector;
  **Files:** `web/src/components/topology/TopologyCanvas.tsx`,
  `web/src/routes/Topology.tsx`, `web/src/hooks/useTopology.ts`,
  `web/src/routes/Topology.test.tsx`;
  **Verify:** focused topology tests covering node selection, edge selection,
  keyboard clear, dimming, and safe inspector text.)

- [x] **Task 4: Make the accessible topology list fully interactive.** Replace
  plain relationship text with selectable node/edge rows or buttons, expose
  selected state and match labels, keep source/target names readable, and make
  list selection synchronize with the canvas and inspector. Add empty, filtered,
  and truncated states. (**Seam:** shared selection props into the existing HTML
  table;
  **Files:** `web/src/components/topology/TopologyList.tsx`,
  `web/src/routes/Topology.tsx`, `web/src/routes/Topology.test.tsx`;
  **Verify:** keyboard-focused React Testing Library assertions for row
  selection, `aria-pressed`, filter counts, and edge details.)

- [x] **Task 5: Clarify the React Flow canvas and responsive shell.** Add explicit
  category/relationship legend treatment, compact edge match labels, a stable
  initial viewport/fit action, and memoized flow arrays/callbacks. Remove the
  filter-driven remount/timer churn. Keep the accessible list immediately
  reachable on narrow screens, wrap the shell header controls, and ensure plugin
  and tool subroutes retain correct navigation state. (**Seam:** React Flow
  rendering and AppShell responsive layout;
  **Files:** `web/src/components/topology/TopologyCanvas.tsx`,
  `web/src/components/topology/TopologyList.tsx`,
  `web/src/components/layout/AppShell.tsx`, focused frontend tests;
  **Verify:** `npm run format-check --prefix web`, typecheck, lint, frontend
  tests, and a manual narrow viewport check.)

- [x] **Task 6: Add scale, security, and regression coverage.** Add a deterministic
  100-node/200-edge fixture and test that filtering and selection remain bounded
  and responsive enough for the console target. Verify no topology response or
  browser text contains credentials, plugin endpoints, raw binding config, or
  invocation payloads. Run the existing Go, frontend, asset, and documentation
  gates. (**Seam:** topology API projection plus frontend interaction tests;
  **Files:** `internal/web/topology_test.go`, `internal/web/integration_test.go`
  only if an existing integration seam is required, `web/src/routes/Topology.test.tsx`,
  `docs/web-console.md`, `CHANGELOG.md`, matching public docs;
  **Verify:** `go test ./...`, `golangci-lint run ./...`,
  `npm run format-check --prefix web`, `npm run typecheck --prefix web`,
  `npm test --prefix web -- --run`, `npm run lint --prefix web`,
  `scripts/verify-console-assets.sh`, and public `npm run build`.)

## Verification

- The topology displays one consistent visible-node/visible-edge result across
  summary, React Flow, inspector, and accessible list.
- Text, category, confidence, and context filters compose predictably and expose
  counts plus a clear-all action.
- Selecting a node highlights its immediate path; selecting an edge highlights
  its source/target and exposes exact match semantics. Escape, pane click, and
  filter changes cannot leave stale hidden selections.
- Keyboard and screen-reader users can select the same nodes and edges from the
  HTML list, with selected state announced and no canvas-only information.
- Plugin nodes never enter the MCP tool facet or Tools inventory; plugin details
  remain on the Plugins page and safe exact-version detail route.
- Capped responses identify incomplete topology data without exposing unsafe
  upstream fields.
- The current read-only/capability/security contract remains unchanged.

## Open Questions

- Whether the product wants `partial` or `full` box selection for multi-select;
  default to no box-selection behavior until a user workflow requires it.
- Whether the 100/200 target needs a formal latency budget; use a manual
  interaction check and browser profiler first, then record a numeric budget if
  evidence supports one.
