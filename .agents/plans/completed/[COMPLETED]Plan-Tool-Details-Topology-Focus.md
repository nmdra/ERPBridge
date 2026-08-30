# Plan: Tool Details Topology Focus

## Goal

Let an operator open the Integration topology from a tool detail page and see
that tool's connected execution path focused immediately.

## Current State

- `ToolDetails` shows tool metadata and manifest details but has no topology
  navigation action (`web/src/routes/Tools.tsx:433-511`).
- `/topology` renders `Topology` without route parameters (`web/src/App.tsx:125-128`).
- `Topology` owns focused-component and node-selection state; it derives the
  related graph from `componentForNode` and `buildTopologyView`
  (`web/src/routes/Topology.tsx:346-455`; `web/src/routes/topologyPresentation.ts:210-224`).
- The topology view already supports a selected MCP tool focusing its related
  endpoint component (`web/src/routes/Topology.tsx:478-493`).

## Decisions

1. Use a URL query parameter containing the tool name. It makes the focused
   view bookmarkable and avoids adding shared mutable client state.
2. Resolve the parameter only after topology data loads. If the named tool is
   absent, leave the normal compact overview intact.
3. Reuse existing component and selection behavior so the rendered view remains
   the established focused direct-path topology, not a second visualization.

## Scope

In scope: a Tool Details navigation control, topology query parsing and focus
initialization, focused UI regression tests, and Console documentation/changelog.

Out of scope: mutations, tool invocation, new topology APIs, changing backend
projections, or changing normal topology selection behavior.

## Tasks

- [x] Add a failing Tool Details test for the topology link and a failing
  Topology test for query-driven focused tool selection; implement the
  query-driven navigation and reuse existing focus selection. **Seam:**
  `ToolDetails` link → `/topology?tool=<name>` → `Topology` selection state →
  `buildTopologyView`. **Files:** `web/src/routes/Tools.tsx`,
  `web/src/routes/Tools.test.tsx`, `web/src/routes/Topology.tsx`,
  `web/src/routes/Topology.test.tsx`. **Verify:** `npm --prefix web test -- --run src/routes/Tools.test.tsx src/routes/Topology.test.tsx && npm --prefix web run typecheck`.
- [x] Document the read-only topology drill-down navigation and add an
  Unreleased entry in both documentation repositories. **Seam:** Console tool
  details → topology workflow. **Files:** `docs/web-console.md`,
  `CHANGELOG.md`, `/home/nimendra/Documents/Projects/erpbridge-docs/docs/erpbridge/console.mdx`,
  `/home/nimendra/Documents/Projects/erpbridge-docs/CHANGELOG.md`. **Verify:**
  `npm --prefix /home/nimendra/Documents/Projects/erpbridge-docs run build`.

## Verification

- A tool-details action opens `/topology?tool=<encoded-tool-name>`.
- The topology page selects that MCP tool and focuses its connected component.
- A missing or unmatched query parameter preserves the ordinary compact view.
- Focused web tests and type checking pass.
