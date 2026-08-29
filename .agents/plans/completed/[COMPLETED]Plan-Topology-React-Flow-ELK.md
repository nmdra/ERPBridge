# Topology Refactor — React Flow + ELK

## Goal

Refactor the topology console into a progressive topology explorer. Keep the existing backend topology contract, MCP behavior, accessibility relationship table, filters, inspector, and context/freshness semantics. Use a domain presentation model, a visual view model, ELK for layout, and React Flow for rendering.

## Scope

- Reduce visual noise: semantic edge styling, no permanent exact labels, no inferred-edge animation, conditional minimap, theme-token styling.
- Add `topologyViewModel.ts` for overview/focused/expanded graphs, component summaries, clustering, selection/dimming, and aggregate edges.
- Add `topologyLayout.ts` and async layout lifecycle/cache using `elkjs`; remove fixed coordinate layout from `topologyFlow.ts`.
- Add custom transport, component, entity, cluster, and topology edge renderers with shared visual primitives and accessible text/icon/status semantics.
- Add progressive expansion, component sidebar, topology/relationships tabs, read-only canvas behavior, stable fit-view behavior, and high-degree port support where justified.
- Preserve `TopologyList` as the complete non-canvas relationship representation.
- Add safe, explicit ERP endpoint nodes and API-to-endpoint edges so focused
  views show MCP tool → ERP API component → exact method/path endpoint chains.
- Place the component navigator and inspector below the full-width canvas.
- Do not change `internal/web/topology.go` unless a frontend contract gap is proven.
- Update focused frontend tests, docs, changelog, and synchronized public documentation as required.

## Execution order and verification

1. **Baseline and dependency** — record current frontend tests/typecheck/build; add `elkjs` and lockfile. Verify: `npm --prefix web run typecheck` and `npm --prefix web test -- --run`.
2. **View model** — add overview/focused/cluster visual graph types and tests; preserve current presentation semantics. Verify: focused topology presentation/view-model tests pass.
3. **Visual system and node/edge families** — add shared shells, icons, tones, status rails, custom nodes/edges, and Phase 1 edge/minimap/read-only improvements. Verify: topology component/canvas/page tests, lint, and typecheck pass.
4. **ELK layout** — implement isolated ELK conversion, stable dimensions, async cancellation, structural layout key/cache, and fit-view integration. Verify: layout invariant tests plus frontend tests/typecheck/build pass.
5. **Progressive explorer** — add cluster expansion, component sidebar, toolbar/legend, topology/relationships tabs, selection isolation, and inspector integration. Verify: view-model, route, canvas, accessibility, and large-graph tests pass.
6. **Port-aware layout** — add multiple handles/ELK ports only for high-degree nodes after the base layout is stable. Verify: port mapping tests and full frontend checks pass.
7. **Docs and quality** — update `docs/web-console.md`, `CHANGELOG.md`, and the matching public docs repository. Run focused lint, `make test`, embedding/security checks as applicable, and inspect the rendered UI in light/dark modes. Verify: all stated gates are green.

## Non-goals

- No topology backend/API contract change beyond additive safe endpoint-node
  projections required for the focused relationship chain.
- No host-only endpoint matching fallback; the ERP endpoint issue is resolved by registering the ERP API at its base/root URL.
- No Web Worker until profiling demonstrates that main-thread ELK layout is a problem.

## Completion

After all verification commands pass, prefix this filename with `[COMPLETED]` and move it to `.agents/plans/completed/`. Commit implementation changes atomically with a Conventional Commit; keep unrelated pending work out of the commit.
