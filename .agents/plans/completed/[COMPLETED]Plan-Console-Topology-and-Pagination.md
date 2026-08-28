# Plan: Console topology redesign and pagination

## Goal

Make the Console topology resolve APIs from the selected `bridgectl` context, present a clean compact overview by default, let an operator drill into a selected ERP API or MCP tool with its connected nodes and edges, and paginate operational tables across the web interface.

## Current State

- `bridgectl web` builds the Console handler with configuration but no API registry (`internal/cli/web.go:60-66`). The topology handler then falls back to the legacy global registry when no registry is injected (`internal/web/topology.go:104-112`). Context-scoped `bridgectl api register` data is therefore absent from the graph, so unmatched tools become `unresolved-endpoint` nodes (`internal/web/topology.go:149-161`).
- API matching already distinguishes exact, prefix, ambiguous, and unresolved states, but both ambiguous and unresolved outcomes create an `unresolved-endpoint` node (`internal/web/topology.go:337-376`, `internal/web/topology.go:149-161`).
- The UI only switches to endpoint-component compact mode after a size threshold; compact mode renders endpoint nodes alone with no edges (`web/src/routes/Topology.tsx:342-375`). The existing component drill-down includes the member nodes and edges (`web/src/routes/topologyPresentation.ts:160-172`).
- `TopologyCanvas` already uses React Flow 12 custom nodes, controls, minimap, and a fixed-column layout (`web/src/components/topology/TopologyCanvas.tsx:252-396`; `web/package.json:20`). React Flow documents custom nodes/handles, `Controls`, `MiniMap`, fit-view, and built-in keyboard/screen-reader behavior. [React Flow custom nodes](https://reactflow.dev/learn/customization/custom-nodes), [handles](https://reactflow.dev/learn/customization/handles), [accessibility](https://reactflow.dev/learn/advanced-use/accessibility).
- The client fetches the topology for the chosen context and does not create unresolved nodes itself (`web/src/hooks/useTopology.ts:62-68`).
- Operations tables render complete, bounded client-side arrays: tools (`web/src/routes/Tools.tsx:359-401`), plugins and bindings (`web/src/routes/Plugins.tsx:37-149`), topology edges (`web/src/components/topology/TopologyList.tsx:29-111`), logs (`web/src/routes/Logs.tsx:199-267`), and metrics (`web/src/routes/Metrics.tsx:281-320`). Their API responses do not expose cursors or totals, so client-side pagination is the compatible first implementation.

## Decisions

- Resolve the API registry per requested Console context. Injected registries remain an explicit test seam; they must not silently replace every browser-selected context. This makes topology use the same context-scoped inventory that `bridgectl api register` maintains.
- Make component overview the default whenever there are ERP API or unresolved/ambiguous endpoint components. Render a compact, connected flow of transport → component summary rather than the full raw graph. Selecting an ERP API or MCP tool from the component menu/drill-down renders only that node’s connected component and preserves the node/edge inspector.
- Model ambiguous endpoint matches separately from unresolved matches. Do not falsely call a collision unresolved; keep the graph safe by showing no winning API until an operator resolves the duplicate registration.
- Retain React Flow and build the redesign with explicit custom node shapes, semantic handle IDs, `Controls`, `MiniMap`, fit-view, keyboard focus, and accessible labels. Do not add a layout dependency unless the redesigned fixed layered layout proves insufficient.
- Add one shared, dependency-free client-side pagination hook and control. Filter/sort first, then paginate; reset or clamp pages when the source/filter changes. Use independent state for independent tables. Defer server pagination because current responses are bounded arrays and do not define pagination contracts.

## Scope

In scope:

- Context-correct topology API resolution, explicit ambiguous representation, and safe diagnostic presentation.
- Compact topology overview, focused component graph, React Flow node/edge redesign, and accessible component selection.
- Pagination for tools, plugins, bindings, topology relationships, logs, and metrics.
- Focused Go and React tests plus Console documentation/changelog updates.

Out of scope:

- Changing MCP protocol behavior, tool execution, or credential handling.
- Server-side cursor pagination or unbounded browser retention.
- Replacing React Flow or adding a new graph-layout library without measured need.
- Pagination of small static/detail tables such as manifest input fields and configured contexts.

## Tasks

- [x] Task 1: Make topology load APIs from the requested context registry and distinguish unavailable registry data from an empty registry. (**Seam:** `consoleHandler.topology` registry resolution; **Files:** `internal/cli/web.go`, `internal/web/topology.go`, `internal/web/topology_test.go`, related registry constructor tests; **Verify:** `go test ./internal/web ./internal/cli`.)
- [x] Task 2: Preserve ambiguity as a first-class, non-destructive topology result and expose safe mismatch reasons without URLs or credentials. (**Seam:** `matchAPI` and `TopologyNode`/`TopologyEdge` projection; **Files:** `internal/web/topology.go`, `internal/web/topology_test.go`, `web/src/hooks/useTopology.ts`; **Verify:** focused Go tests prove selected-context exact match, other-context unresolved match, duplicate match ambiguity, and redaction.)
- [x] Task 3: Redesign topology presentation around a compact connected overview and explicit component selection/drill-down. (**Seam:** endpoint-component derivation and route selection state; **Files:** `web/src/routes/topologyPresentation.ts`, `web/src/routes/Topology.tsx`, `web/src/routes/topologyPresentation.test.ts`, `web/src/routes/Topology.test.tsx`; **Verify:** `npm --prefix web test -- --run src/routes/topologyPresentation.test.ts src/routes/Topology.test.tsx`.)
- [x] Task 4: Rebuild React Flow visuals using distinct component shapes, labelled directional handles, selected/dimmed states, adaptive fit-view, minimap, controls, and keyboard-accessible node/edge selection. (**Seam:** `TopologyCanvas` node and edge adapters; **Files:** `web/src/components/topology/TopologyCanvas.tsx`, new focused topology component/style tests as needed; **Verify:** `npm --prefix web run typecheck && npm --prefix web test -- --run src/routes/Topology.test.tsx`.)
- [x] Task 5: Add reusable client-side pagination primitives with accessible range and page controls. (**Seam:** pure pagination hook/control; **Files:** new `web/src/hooks/usePagination.ts`, new `web/src/components/ui/pagination.tsx`, colocated tests; **Verify:** `npm --prefix web test -- --run <pagination-test>`.)
- [x] Task 6: Apply pagination after filtering/sorting to tools, plugins, bindings, topology relationships, logs, and metrics; preserve selection and live-log page stability. (**Seam:** each route’s rendered row collection; **Files:** `web/src/routes/Tools.tsx`, `web/src/routes/Plugins.tsx`, `web/src/components/topology/TopologyList.tsx`, `web/src/routes/Logs.tsx`, `web/src/routes/Metrics.tsx`, and their existing tests; **Verify:** `npm --prefix web test -- --run src/routes/Tools.test.tsx src/routes/Plugins.test.tsx src/routes/Topology.test.tsx src/routes/LogsMetrics.test.tsx`.)
- [x] Task 7: Document the new topology interaction, diagnosis states, and table pagination; record the user-visible behavior. (**Seam:** Console user guide; **Files:** `docs/web-console.md`, `CHANGELOG.md`; **Verify:** `npm --prefix web run build`.)

## Verification

- A context-scoped API registered for context A is an ERP API node with an exact/prefix relationship in A and is not leaked into context B.
- Duplicate API candidates render as ambiguous, never unresolved or falsely exact; output contains no credentials or full endpoint URLs.
- The default view is compact and connected; selecting an ERP API or MCP tool shows only its connected component, then returns to the overview.
- React Flow remains keyboard-operable and exposes useful labels for diagram, nodes, edges, controls, and component selection.
- Every scoped operational table shows a page range, disabled/enabled previous/next controls, and resets/clamps correctly after filters or source data changes.
- `go test ./...`, `npm --prefix web run typecheck`, `npm --prefix web test`, `npm --prefix web run lint`, and `npm --prefix web run build` pass before completion.

## Open Questions

No open questions remain. “component shsper” was implemented as distinct React Flow component shapes, and standalone MCP tools remain selectable compact components so unassigned tools stay diagnosable.
