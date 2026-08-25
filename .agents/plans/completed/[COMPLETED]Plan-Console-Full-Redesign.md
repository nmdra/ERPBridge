# Plan: ERPBridge Console Full UI/UX Redesign

> **Status: COMPLETED — verified 2026-08-26**
>
> The console now has an operations-first information architecture, green brand
> accent, responsive navigation, context-aware dashboard, trustworthy
> session-local metrics, consistent live-data states, and preserved topology
> behavior.

## Goal

Make the read-only ERPBridge Console a trustworthy operational workspace rather than a collection of independent pages. Operators should understand the selected context, current health, freshness, traffic, tools, plugins, logs, metrics, and safe integration relationships at a glance, then drill into detail without losing orientation. Use green as the brand accent while preserving distinct warning, danger, information, and topology-category colors. Preserve all existing read-only, loopback-only, capability-protected, credential-free BFF boundaries.

The topology functionality is a protected contract for this redesign. Its filters, synchronized node/edge selection, inspector, accessible relationship list, confidence states, truncation reporting, and bounded canvas remain behaviorally unchanged. Only shared shell and token styling may affect its presentation.

## Current State

- `web/src/App.tsx:18-111` mounts a flat shell and routes. The home route adds an introductory card, then `Overview` renders deployment/server metadata and a permanent “Operational data is loading” state (`web/src/routes/Overview.tsx:9-100`).
- `web/src/components/layout/AppShell.tsx:21-49` exposes a flat navigation list. The visible “Deployments” page is a context inventory (`web/src/routes/Deployments.tsx:6-42`), while the shell defaults an unmatched page title to “Settings” (`AppShell.tsx:49`). Mobile currently renders the full sidebar above the page content (`AppShell.tsx:58-101`).
- The BFF already exposes safe GET-only health, cache, server-info, tools, logs, metrics, plugin, binding, and topology projections (`internal/web/context_api.go:32-54`, `internal/web/observability_api.go:17-121`). The frontend does not currently consume health or cache in the dashboard.
- `web/src/routes/Metrics.tsx:8-96` shows counts of returned samples/rates and a table, but does not chart the installed Recharts dependency. The BFF emits labels with cumulative samples and rates (`internal/web/metrics.go:20-55`). Rates must be joined by metric name plus the complete sorted label set, not name alone.
- `web/src/hooks/useObservability.ts:228-260` filters logs, and `useLogs` appends bounded stream events, but loading, stale-data, stream-error, and filtered-empty states are not expressed consistently. `useMetrics` clears data on each poll and does not retain a session history.
- The current context selection starts at `local` and only changes when that value is not present (`web/src/App.tsx:18-31`), which can silently select a non-current context when several contexts exist.
- Shared semantic tokens use blue for primary, focus, links, and sidebar (`web/src/styles/globals.css:8-31`). Status tokens are already separate and must remain separate. Recharts `^3.1.2` is installed (`web/package.json:20-36`) but unused.
- The topology workspace already has the required interaction contract in `web/src/routes/Topology.tsx`, `web/src/components/topology/TopologyCanvas.tsx`, and `web/src/components/topology/TopologyList.tsx`; its existing tests cover accessible relationships, plugin filtering, scale, and edge inspection (`web/src/routes/Topology.test.tsx:7-175`).
- The public console contract says the UI is read-only, keeps credentials and raw upstream responses server-side, and shows safe paths only (`docs/web-console.md:1-20`, `docs/web-console.md:148-190`).

## Research Evidence

- WAI-ARIA guidance requires keyboard-operable interactive controls, visible and persistent focus, predictable focus movement, and a clear distinction between focus and selection: <https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/>.
- W3C complex-image guidance recommends a concise summary plus a detailed, public text alternative for complex charts, including values, relationships, and trends: <https://www.w3.org/WAI/tutorials/images/complex/>.
- Material navigation guidance recommends grouping related tasks, prioritizing frequent destinations, and using a navigation drawer for many top-level destinations: <https://m1.material.io/patterns/navigation.html>.
- Recharts documents `ResponsiveContainer` for charts that follow a defined parent size; charts still require an accessible table or text summary: <https://recharts.github.io/en-US/api/ResponsiveContainer/> and <https://www.w3.org/WAI/tutorials/images/complex/>.
- React documents `useMemo` as a performance optimization only when dependencies are stable and recalculation is material; it must not be used to make correctness depend on caching: <https://react.dev/reference/react/useMemo>.

## Decisions

1. **Use an operations-first information architecture.** Group navigation into Monitor (Overview, Logs, Metrics), Inventory (Contexts, Tools, Plugins), and Diagnose (Integration topology). Rename visible “Deployments” to “Contexts” because it lists configured CLI contexts. Preserve `/deployments`, `/tools`, `/plugins`, `/topology`, `/logs`, `/metrics`, and detail URLs; add `/contexts` as the preferred alias rather than breaking bookmarks.
2. **Make context scope explicit and trustworthy.** Initialize the selector from the BFF-marked current context, preserve a deliberate user selection, show the selected context in every live-data page header, and never silently fall back to `local` when a configured current context exists.
3. **Use a compact command dashboard.** Overview shows health and freshness first, then no more than four KPI cards, then compact trend summaries and drill-down links. It uses existing safe health, cache, server-info, tools, and metrics routes; it adds no mutation or upstream-proxy route.
4. **Make live-data uncertainty visible.** Every polling/streaming page distinguishes loading, unavailable, stale-but-retained, true empty, filtered empty, and fresh data. A failed refresh keeps the last successful safe projection and displays its age and a scoped retry action.
5. **Make charts session-local and table-equivalent.** Metrics charts show only the current browser session’s observed snapshots, label the window explicitly, and provide a semantic table with canonical metric labels and values. No UI wording may imply historical Prometheus storage or percentile data that the BFF does not provide.
6. **Use green only as the brand accent.** Change primary, focus ring, links, selected navigation, chart series, and positive emphasis to an accessible green scale in light and dark themes. Keep semantic success, warning, danger, info, and topology node-category colors distinct so color is not the only status signal.
7. **Prefer native accessible controls and predictable focus.** Add a skip link, `aria-current` navigation, named page headings/breadcrumbs, an accessible mobile navigation drawer, live status announcements for refresh states, and persistent visible focus. Do not add undocumented keyboard shortcuts.
8. **Keep topology behavior frozen.** Do not change its data model, filtering semantics, selection synchronization, accessible relationship table, inspection fields, or canvas/list contract. Add only regression checks proving shell/theme migrations preserve those behaviors.

## Scope

### In scope

- Full frontend navigation and information-architecture redesign.
- Green design tokens, light/dark/system contrast-safe presentation, shared page headers, metric cards, state banners, filter toolbars, and responsive data-surface primitives.
- Trustworthy context selection and page orientation.
- Overview dashboard using existing safe health/cache/server/tools/metrics projections.
- Metrics session history, canonical labeled-series joins, compact Recharts visualizations, and accessible table alternatives.
- Logs stale/error/filter UX, context inventory UX, Tools and Plugins inventory/detail presentation, and Settings appearance/accessibility preferences.
- Responsive navigation drawer and 320/768/1024px behavior.
- Focus, reduced-motion, empty/loading/error/stale-state, security, topology-regression, component, and Playwright coverage.
- In-repository console documentation, public documentation sync, and `CHANGELOG.md`.

### Out of scope

- Any mutation, invocation, deployment, credential, raw-payload, or upstream-URL route.
- Changes to topology behavior or backend topology matching/projections.
- Historical metrics storage, Prometheus querying, percentile claims, or a new telemetry backend.
- Adding a state-management framework, charting dependency, or component library beyond installed packages.
- Breaking existing deep links or changing the loopback/capability security model.

## Tasks

Every implementation task follows red → green → refactor. Each task requires its focused verification before its Conventional Commit.

- [x] **Task 1: Establish the green design system and shared UI primitives.** Add semantic green light/dark tokens while preserving status/topology colors; add `PageHeader`, `MetricCard`, `StateBanner`, `FilterToolbar`, and accessible data-surface styles; add skip-link and focus/reduced-motion coverage. **Seam:** shared components and CSS tokens. **Files:** `web/src/styles/globals.css`, `web/src/components/ui/*`, `web/src/components/status/StatusBadge.tsx`, new shared components under `web/src/components/layout/` and `web/src/components/ui/`, related tests, `CHANGELOG.md`. **Verify:** `cd web && npm run format-check && npm run typecheck && npm test -- --run`.
- [x] **Task 2: Rebuild navigation, context scope, and route orientation.** Group desktop navigation, add an accessible mobile drawer, add skip-to-content and `aria-current`, rename visible Deployments to Contexts while retaining its old route, fix current-context initialization, and make page headers/breadcrumbs route-owned. **Seam:** `AppShell` and `ConsoleApp`. **Files:** `web/src/App.tsx`, `web/src/components/layout/AppShell.tsx`, `web/src/routes/Deployments.tsx`, new `web/src/routes/Contexts.tsx` if needed, `web/src/hooks/useConsole.ts`, shell/app tests, root and public console docs, `CHANGELOG.md`. **Verify:** `cd web && npm test -- --run src/app.test.tsx` plus Playwright checks at 320px and 1024px.
- [x] **Task 3: Build the operational Overview dashboard.** Add safe health/cache hooks, a context health/freshness banner, KPI cards for health, active tools, cache, and server version, compact operational summaries, and links to Logs, Metrics, Tools, and Integration topology. Keep partial data visible with stale-state messaging. **Seam:** existing GET-only BFF projections. **Files:** `web/src/hooks/useConsole.ts`, `web/src/hooks/useObservability.ts`, `web/src/routes/Overview.tsx`, new dashboard components/tests, `web/src/App.tsx`, `CHANGELOG.md`, console docs. **Verify:** focused Overview tests with healthy, unavailable, partial, and stale fixtures; `cd web && npm test -- --run`.
- [x] **Task 4: Make Metrics trustworthy and useful.** Correct canonical `(name, labels)` joins, preserve prior successful snapshots, collect a bounded browser-session history, render compact responsive Recharts trends, expose last-updated/window labels, and keep an accessible labeled table plus text summary. **Seam:** `useMetrics` and Metrics route. **Files:** `web/src/hooks/useObservability.ts`, `web/src/routes/Metrics.tsx`, new chart/metric components, `web/src/routes/LogsMetrics.test.tsx`, `CHANGELOG.md`, console/public docs. **Verify:** labeled-series regression tests, chart/table parity tests, and `cd web && npm test -- --run src/routes/LogsMetrics.test.tsx`.
- [x] **Task 5: Redesign Logs and inventory surfaces.** Add reusable filter toolbar/state handling to Logs, preserve recent events on stream failure with stale warnings, improve true-empty versus filtered-empty copy, make Contexts a responsive status-oriented inventory, and migrate Tools/Plugins tables and detail pages to shared page headers, filter controls, badges, and mobile-readable surfaces without changing safe fields or routes. **Seam:** existing projection hooks and route components. **Files:** `web/src/routes/Logs.tsx`, `web/src/routes/Deployments.tsx`, `web/src/routes/Tools.tsx`, `web/src/routes/Plugins.tsx`, shared components/tests, docs/changelog. **Verify:** route tests for filtering, stale stream, responsive labels, and plugin/tool security projections; `cd web && npm test -- --run`.
- [x] **Task 6: Rework Settings and topology presentation without changing topology behavior.** Make Settings a real appearance/accessibility page, expose light/dark/system choices, migrate topology shell presentation to green tokens only where semantically safe, and add regression tests for topology filters, node/edge selection, inspector safety, and list/canvas parity. **Seam:** ThemeProvider, Settings, existing topology tests. **Files:** `web/src/theme/*`, `web/src/components/layout/ThemeToggle.tsx`, `web/src/routes/Settings.tsx`, topology components/tests, `CHANGELOG.md`, docs. **Verify:** `cd web && npm test -- --run src/theme src/routes/Topology.test.tsx` and manual light/dark/reduced-motion checks.
- [x] **Task 7: Complete responsive, accessibility, documentation, and release verification.** Run browser checks at 320/768/1024px, keyboard-only checks, production build, asset-size verification, Go tests/lint, public docs build, and security assertions that no credentials/raw payloads/upstream URLs appear. Archive this plan only after all checks pass. **Seam:** built console and existing BFF integration tests. **Files:** Playwright artifacts/tests, docs, `CHANGELOG.md`, plan index. **Verify:** `cd web && npm run format-check && npm run typecheck && npm test -- --run && npm run lint && npm run build`; `go test ./...`; `golangci-lint run ./...`; `scripts/verify-console-assets.sh`; public docs build; manual browser matrix.

## Verification

- Existing routes and detail deep links continue to resolve; `/contexts` is added without removing `/deployments`.
- Selected context always matches the configured current context on first load unless the operator explicitly changes it.
- Overview identifies health state and freshness, shows partial failures honestly, and provides safe drill-down links.
- Metrics never joins different label series together, never implies durable history, and provides chart-equivalent text/table data.
- Logs retain safe recent data when streaming fails and identify the stream as stale/disconnected.
- Green accent passes light/dark contrast review; warnings, danger, info, success, and topology categories remain distinguishable without color alone.
- Skip link, named landmarks, `aria-current`, visible focus, keyboard navigation, mobile drawer focus handling, and reduced-motion behavior work in browser checks.
- Topology filters, node/edge selection, inspector safety, truncation warning, and accessible list remain unchanged and pass existing regression coverage.
- No console route mutates state, invokes tools, exposes credentials, displays raw payloads, or displays full upstream URLs.

## Open Questions

None. The user selected the full redesign and explicitly excluded topology functionality changes; the plan treats topology behavior as frozen while allowing safe shell/theme presentation updates.
