# Plan: ERPBridge Developer Console in `bridgectl`

> **Status: COMPLETED — VERIFIED 2026-08-26**
>
> This plan covers the completed local, read-only web console launched by
> `bridgectl web`, including the feature-detected external-plugin UI added after
> the generic external-plugin implementation and contract tests were completed.
> The plan remains the execution record; all task checkboxes and verification
> gates are complete.
>
> **Completion verification:** `go test ./...`, `golangci-lint run ./...`,
> `make test-plugin-integration`, frontend format/typecheck/tests/lint/build,
> `scripts/verify-console-assets.sh`, and the public documentation build all
> pass. The plugin UI also has focused projection, topology, and frontend tests.

## Goal

Give ERPBridge developers a small, polished local web console that starts from
`bridgectl web`, reuses existing bridgectl contexts and authentication, and
makes deployment health, tool mappings, logs, metrics, and API-to-MCP paths
visible without replacing the CLI.

The console is a local operator experience, not a central Backstage-style
platform. `bridgectl` remains the automation and mutation interface. The first
release is read-only: it must not apply/delete tools, flush caches, invoke
MCP tools, manage tokens, deploy plugins, or mutate remote deployments.

## Current State

- `tools/bridgectl/main.go:1-14` only sets the CLI version and calls
  `cli.Execute`; there is no web command or frontend asset pipeline.
- `internal/cli/root.go:40-80` defines the Cobra root command, loads config in
  `PersistentPreRunE`, supports `--context`, `--token`, `--output`, and
  `--verbose`, and stores the loaded config in package state.
- `internal/config/config.go:16-37` defines contexts with `Server`,
  `MCPServer`, `ERPBase`, `APIToken`, and an `AuthConfig`; `Load` and
  `ActiveContext` are the existing configuration seams (`:40-47`, `:107-113`).
  Contexts are stored in `~/.bridgectl/config.yaml` (`:115-118`).
- `internal/cli/http.go:9-43` creates authenticated requests and resolves the
  explicit token, `BRIDGE_API_TOKEN`, or active-context token. This behavior
  must remain compatible with existing CLI commands.
- `internal/cli/context.go:16-68` already exposes named environments and their
  server URLs. `internal/idp/registry.go:12-91` stores the local API registry in
  `~/.bridgectl/registry.json`; its `API` type includes credential fields
  (`:14-32`) that must never be sent to the browser or rendered by the console.
- `internal/cli/api.go:15-164` lists/tests local ERP API definitions, while
  `internal/cli/tool.go:15-214` and `:250-501` list, describe, generate, apply,
  validate, and delete remote MCP tools. `internal/idp/generator.go:21-66` maps
  an API definition to an MCP tool and copies the HTTP method and endpoint into
  `Tool.Spec.Execution`.
- `internal/mcp/tool.go:18-91` defines versioned `MCPTool` resources, including
  execution method/endpoint, response path, schemas, cache, security, routing,
  and lifecycle metadata. `Tool.Execute` (`:104-230`) resolves the ERP request,
  response path, and output validation. This is the source data for the
  API-to-tool topology; endpoint matching may be unresolved when a tool was not
  generated from the local registry.
- `internal/mcp/server.go:546-647` exposes the MCP and management HTTP routes.
  Current read-oriented data sources include `/mcp/health`,
  `/apis/erpbridge.io/v1/tools`, `/api/cache/stats`, `/api/logs/recent`,
  `/api/logs/stream`, and `/metrics` (`:649-1020`; see also `docs/api.md:1-158`).
  The registry, cache, direct-invoke, and token routes are admin-only when
  authentication is enabled (`docs/connectivity.md:64-76`).
- `internal/mcp/server.go:983-1020` provides recent-log JSON and an SSE log
  stream. `internal/metrics/metrics.go:18-88` defines Prometheus counters,
  histograms, and gauges. `/metrics` is a scrape endpoint, not a historical
  query API (`services/erpbridge-server/main.go:159-164`).
- `Makefile:1-66` builds only Go binaries, runs `go test ./...`, and has no Node
  or frontend targets. `go.mod:1-24` has no frontend-related Go dependency.
- `.goreleaser.yaml:6-24` invokes `go mod tidy` and then builds the server and
  CLI directly; it has no frontend asset hook. `.github/workflows/release.yml:24-68`
  installs Go but not Node/npm before invoking GoReleaser. Release packaging
  must therefore build and verify the frontend before GoReleaser runs.
- The current plugin work is only an upcoming, not-active plan:
  `.agents/plans/upcoming/Plan-Generic-External-Plugins.md:1-185`. Its fixed
  boundary stores `Plugin` and exact-version `PluginBinding` resources, runs
  plugins after successful responses, leaves plugin deployment to operators,
  and explicitly excludes plugin health discovery and portal work. The portal
  must not implement or assume those routes until that plan is completed and
  its API contract is stable.
- External research supports the proposed shape: Minikube's Go `dashboard`
  command starts a local proxy, chooses a port, opens the browser, supports a
  URL-only mode, and waits for process shutdown
  ([source](https://github.com/kubernetes/minikube/blob/master/cmd/minikube/cmd/dashboard.go));
  Tailscale hosts a React/TypeScript/Vite/Tailwind UI behind a Go server,
  proxies Vite during development, and serves production assets from an
  embedded filesystem ([package](https://github.com/tailscale/tailscale/blob/main/client/web/package.json),
  [server](https://github.com/tailscale/tailscale/blob/main/client/web/web.go),
  [assets](https://github.com/tailscale/tailscale/blob/main/client/web/assets.go));
  Go officially supports serving `embed.FS` through `net/http`
  ([source](https://pkg.go.dev/embed)). The planned theme follows shadcn/ui's
  semantic CSS-variable model ([theming](https://ui.shadcn.com/docs/theming),
  [dark mode](https://ui.shadcn.com/docs/dark-mode)).

## Decisions

### 1. Product boundary

Build a local **ERPBridge Console**, not Backstage and not a second control
plane. The canonical command is `bridgectl web`; a root `--web` flag is not part
of the initial contract. `bridgectl` remains authoritative for declarative
changes and automation.

### 2. Local web-host lifecycle

`bridgectl web` binds to `127.0.0.1` on an ephemeral port by default, starts one
local HTTP server, prints the URL, and opens the default browser unless
`--no-open` or `--url` is supplied. The process owns its server lifetime and
shuts down on Ctrl-C. Non-loopback listening is out of scope for the first
release; the console is not a remote shared portal.

### 3. Same-origin local BFF and per-launch capability

The browser talks only to the local console server. The Go server makes
allow-listed requests to configured ERPBridge contexts and keeps bearer tokens,
ERP base URLs, and local registry credentials server-side. The console will
never expose an arbitrary URL proxy, raw context auth data, or the local API
registry's `AuthKey`/`AuthToken` fields.

Loopback is not treated as authentication. Each launch creates a high-entropy
capability. The browser receives it in the URL fragment, removes it from the
address bar immediately, and sends it in a dedicated header for local API and
SSE requests. The server validates the exact `Host` and same-origin `Origin`
when present, rejects non-loopback hostnames and cross-origin requests, sends
`Content-Security-Policy`, `frame-ancestors 'none'`, `X-Content-Type-Options`,
`Referrer-Policy`, and `Cache-Control: no-store` headers, and does not reflect
CORS origins. This protects against ordinary hostile websites and accidental
local callers; a privileged local process or browser extension remains outside
the threat model.

### 4. Contexts are the initial deployment inventory

The first release treats named bridgectl contexts as deployments/environments.
It does not discover Kubernetes, Docker, or cloud deployments. Each UI context
response contains only a safe display name, configured server identity, and
health/availability state.

### 5. Frontend technology

Use React + TypeScript + Vite, Tailwind CSS, source-owned shadcn/ui-style
components, only the required Radix UI primitives, `lucide-react`, Wouter,
`@xyflow/react` for interactive topology, and Recharts for curated metrics.
Use native `fetch` and `EventSource` first; add SWR only if polling/cache
coordination proves repetitive. Do not add Angular, Backstage, GraphQL,
Redux, WebSockets, or a full design-system runtime.

`shadcn/ui` is a component and token pattern rather than a monolithic runtime
library. Use semantic CSS variables for `background`, `foreground`, `card`,
`muted`, `primary`, `border`, `success`, `warning`, `destructive`, and chart
colors. Support light, dark, and system themes. Use Inter with a system-font
fallback, following Tailscale's existing typography direction. Generic icons
come from Lucide; no hand-maintained SVG icon set is needed.

### 6. Topology semantics

The topology view models the call path as:

```text
MCP client → MCP tool → ERP API endpoint
                         └→ future plugin binding → external plugin → result
```

The full view uses React Flow; the tool detail view uses a compact path
inspector. Relationships carry an explicit confidence: `exact` means the
normalized method and endpoint match; `base-prefix` means the local API URL is
only an inferred base; `ambiguous` means multiple local APIs match; and
`unresolved` means no safe match exists. The UI must never render an inferred
edge as authoritative. Because the local API registry is global and has no
context field, a selected deployment also shows whether an API is `context
matched` or `unassigned`; it does not claim cross-environment ownership.
Plugin nodes and bindings are feature-gated until the plugin plan is complete.

### 7. Read-only and data retention

Phase 1-5 expose health, safe metadata, tools, cache statistics, recent/live
logs, live Prometheus samples, and topology. The console keeps only a bounded
in-memory sample window for local sparklines while it is running. It does not
create a persistent metrics or log database. Historical charts require a
future, explicit Prometheus query integration and are out of scope here.

Metric cards distinguish cumulative totals from session-local estimates. Rates
are calculated only from successive local samples; latency cards use tested
histogram sum/count averages and do not claim percentiles. The UI displays the
sample-window start time and shows `unknown` when the required family is absent.

### 8. Security posture

The local server is loopback-only, requires the per-launch capability for data
routes, validates exact `Host`/`Origin` values, validates configured upstream
URLs once, disables upstream redirects, constructs only fixed relative paths,
forwards no browser headers, bounds response sizes, strips sensitive fields,
and allows only explicit read operations. It treats the configured context file
as trusted operator input but does not permit browser-controlled destinations.
The BFF is an intentionally privileged read deputy for tool inventory because
ERPBridge currently makes the registry admin-only; it returns a narrow safe DTO
and reports unavailable when no suitable configured credential exists.

Log and SSE data are parsed into a BFF-owned allow-list projection. Unknown
fields, raw payloads, credential-bearing strings, oversized/malformed events,
and upstream error bodies are not forwarded. If future mutation routes are
added, they require CSRF/origin protection, deliberate user confirmation, and
a separate approved plan. The console must not weaken ERPBridge
authentication or CORS behavior.

### 9. Plugin dependency gate

The plugin phase cannot start until the separate generic external-plugin plan is
promoted, implemented, tested, documented, merged into the implementation
base, and closed as completed. The entry gate requires the completed plan file,
its final API documentation, the merged contract/API commit, and a passing
contract fixture against the actual implementation; a filename check alone is
not sufficient. The portal will consume the final public API only; it will not
duplicate plugin models or invent provisional routes. It will show plugin
configuration and bindings, not claim runtime health when the completed plugin
contract does not provide health discovery.

## Scope

Every behavior-changing task below must update the relevant in-repository
`docs/` page and `CHANGELOG.md` Unreleased entry in the same ERPBridge commit.
A corresponding commit in `../erpbridge-docs` is required in the same phase;
that separate repository cannot share a Git commit with ERPBridge. Phase 5 is
an audit and release-documentation gate, not the first time behavior is
recorded. Every task follows red → green → refactor: add a failing focused
test, verify the failure, implement the minimum behavior, then refactor and
run the task gate before committing.

### In scope

- `bridgectl web` with loopback binding, browser/URL lifecycle, graceful
  shutdown, context selection, and safe local proxying.
- A single-repository React/Vite frontend whose production assets are embedded
  into the Go CLI build and whose development assets are served by Vite through
  a local Go proxy.
- Light/dark/system theming, semantic tokens, Lucide icons, responsive
  console layout, accessible loading/error/empty states, and keyboard-friendly
  navigation.
- Read-only context/deployment overview, health, tool inventory, cache stats,
  recent logs, live logs, live metrics, and API-to-MCP topology.
- Topology resolution using the local API registry plus remote tool definitions,
  with explicit unresolved-edge states and no credential exposure.
- A later, dependency-gated read-only plugin/binding view and topology overlay.
- Focused Go and frontend tests, build/lint targets, in-repository developer
  documentation, generated CLI documentation, changelog entry, and matching
  public documentation in `../erpbridge-docs`.

### Intentionally out of scope

- Backstage, a central multi-user portal, SSO, remote/public web hosting, or
  Kubernetes/Docker/cloud discovery.
- Tool apply/delete, cache flush, direct tool invocation, token creation/revocation,
  deployment restart, or any other mutation from the UI.
- Persistent log/metric storage, alerting, dashboards that replace Grafana,
  Prometheus historical query support, or tracing.
- Editing API registry credentials or displaying secrets.
- Plugin implementation, plugin deployment, plugin health discovery, plugin
  authentication, plugin mutation, or plugin routes before the external-plugin
  plan is completed.

## Tasks

### Phase 0 — Contract and dependency gate

- [x] **Task 0.1: Confirm the product contract and record the web-console data model.**
  Document the command lifecycle, local-only security boundary, safe context
  projection, BFF route contract, topology node/edge types, metric snapshot
  shape, plugin feature-gate behavior, and historical-metrics limitation.
  Include an explicit dependency check against
  `.agents/plans/upcoming/Plan-Generic-External-Plugins.md` and state that its
  completion is required before Phase 6. (**Seam:** existing context/API/tool
  contracts; **Files:** `docs/web-console.md` (new); **Verify:** `rg -n "bridgectl web|127.0.0.1|unresolved|PluginBinding|read-only" docs/web-console.md`; **Commit:** `docs: define ERPBridge console contract`.)

> **Phase 6 entry gate:** Before starting any Phase 6 task, verify the completed
> plugin plan, merged contract/API commit, final API docs, and passing contract
> fixture against the implementation base. If any item is missing, stop after
> Phase 5. This is a phase gate, not an implementation task or a commit.

### Phase 1 — `bridgectl web` host and lifecycle

- [x] **Task 1.1: Add the reusable local web server package and browser opener.**
  Create the `internal/web` package first, with an injected `http.Server`,
  listener, clock/URL, browser-opening function, and per-launch capability. Bind
  only to literal `127.0.0.1` or `::1` addresses; reject hostnames, wildcard,
  unspecified, and IPv4-mapped addresses. Implement default cross-platform
  browser opening, URL-only/headless behavior, graceful shutdown, exact Host
  validation, local capability bootstrap, security headers, request logging
  without tokens, and a health response for the local console itself. Tests
  must cover hostile Host/Origin, missing or wrong capability, cross-origin SSE,
  and shutdown. (**Seam:** `net.Listener` and `http.Server` lifecycle; **Files:**
  `internal/web/server.go` (new), `internal/web/browser.go` (new),
  `internal/web/security.go` (new), `internal/web/server_test.go` (new),
  `internal/web/security_test.go` (new), `docs/web-console.md`,
  `CHANGELOG.md`; **Verify:** first run the new focused
  tests red, then `go test ./internal/web -run 'Test(Server|Browser|Security|Capability)'`; **Commit:** `feat: host console web server locally`.)

- [x] **Task 1.2: Add the thin Cobra `web` command adapter.** Add a `web`
  subcommand that calls the reusable server, creates a `127.0.0.1:0` listener
  by default, supports `--no-open`, `--url`, `--dev`, and a loopback-only
  explicit listen option, prints the selected URL with a one-time fragment capability,
  and shuts down cleanly on cancellation. Keep existing root persistent flags
  and `PersistentPreRunE` behavior intact. Update the CLI reference and
  Unreleased changelog entry in this commit. (**Seam:** `RootCmd` registration
  and command context; **Files:** `internal/cli/web.go` (new),
  `internal/cli/web_test.go` (new), `internal/cli/root.go`,
  `docs/cli/bridgectl_web.md` (generated), `CHANGELOG.md`; **Verify:**
  `go test ./internal/cli -run 'TestWebCommand|TestWebListener'` and the CLI
  doc generator diff; **Commit:** `feat: add bridgectl web command`.)

- [x] **Task 1.3: Extract a secure context-aware upstream client without
  changing CLI behavior.** Introduce a small internal bridge client seam that
  accepts a concrete `config.Context` plus an optional token override, uses the
  same bearer-token precedence as `internal/cli/http.go`, and supports normal
  JSON requests and streaming responses. The web client must parse and validate
  each configured `Server`/`MCPServer` URL once (HTTP(S), host required, no
  userinfo/query/fragment), construct only fixed relative paths, disable
  redirects, forward no browser headers, set only the resolved bearer token,
  cap response/event sizes, and cancel on context/timeouts. Define the explicit
  route-to-target map: health and metrics use `MCPServer`; tools use
  `MCPServer`; cache and logs use `Server`, matching current CLI behavior.
  Adapt existing CLI helpers to the seam rather than duplicating
  authentication logic. Test malformed/userinfo URLs, redirects to private
  targets, path escapes, split Server/MCPServer endpoints, non-forwarded
  browser headers, token precedence, and bounded responses. (**Seam:**
  `newBridgeRequest`, `doBridgeRequestWithHeaders`, `bridgeAPIToken`, and
  `ValidateServerURL`; **Files:** `internal/bridgeclient/client.go` (new),
  `internal/bridgeclient/client_test.go` (new), `internal/cli/http.go`,
  `internal/cli/http_test.go`, `docs/web-console.md`; **Verify:**
  `go test ./internal/bridgeclient ./internal/cli -run '(BridgeRequest|BridgeClient|Token|Redirect|URL)'`; **Commit:** `refactor: share bridgectl HTTP client behavior`.)

### Phase 2 — Frontend build, embedding, and theme foundation

- [x] **Task 2.1: Add the minimal frontend toolchain.** Add a `web/` React
  TypeScript Vite application with npm lockfile and scripts for build, lint,
  typecheck, test, and format-check. Pin Node `22.14.0` in `web/.nvmrc` and
  `package.json` engines; use the npm version shipped with that Node release.
  Add Tailwind CSS, source-owned shadcn/ui-style components, only the required
  Radix primitives, Lucide React, Wouter, React Flow, Recharts, Vitest, jsdom,
  and React Testing Library. Do not add a second package manager or a separate
  prebuilt-assets repository. (**Seam:** frontend build boundary; **Files:**
  `web/package.json` (new), `web/package-lock.json` (new), `web/.nvmrc` (new),
  `web/tsconfig.json` (new), `web/vite.config.ts` (new), `web/index.html` (new),
  `web/src/main.tsx` (new), `web/src/app.test.tsx` (new); **Verify:**
  `node --version`, `npm ci --prefix web`, `npm run typecheck --prefix web`,
  and `npm test --prefix web -- --run`; **Commit:** `build: add embedded console frontend toolchain`.)

- [x] **Task 2.2: Implement semantic theme tokens and base UI primitives.** Add
  light, dark, and system themes using CSS variables and a root theme
  attribute/class. Define neutral surfaces, ERPBridge blue primary, semantic
  success/warning/destructive/info colors, node-type colors, chart colors,
  borders, focus rings, radii, typography, reduced-motion behavior, and
  high-contrast-safe status labels. Add layout primitives for app shell,
  sidebar, top bar, deployment selector, cards, badges, tabs, drawers,
  skeletons, empty states, errors, and toast notifications. Use Lucide for
  generic icons and do not add manual icon SVG files. Persist explicit theme
  choice locally, honor `prefers-color-scheme` and `prefers-reduced-motion`,
  set `color-scheme` for native controls, and require an HTML/list alternative
  for graph and chart information. (**Seam:** root theme provider and
  reusable UI components; **Files:** `web/src/styles/globals.css`,
  `web/src/theme/ThemeProvider.tsx`, `web/src/components/layout/`,
  `web/src/components/ui/`, `web/src/components/status/`, focused `*.test.tsx`,
  `docs/web-console.md`, `CHANGELOG.md`; **Verify:** `npm run typecheck --prefix web && npm test --prefix web -- --run`
  plus keyboard/focus and theme preference assertions; **Commit:** `feat: add console design system and themes`.)

- [x] **Task 2.3: Embed production assets, proxy Vite in development, and wire
  release packaging.** Configure Vite to emit hashed assets into
  `internal/web/prebuilt/build` without deleting the tracked fallback sentinel;
  `embed.FS` selects the generated `index.html` when present and otherwise
  serves the fallback page so a clean checkout can compile Go tests. Serve SPA
  fallback routes, cache hashed assets, and proxy only asset/HMR traffic to a
  loopback Vite dev server in explicit development mode. Add Makefile targets
  for frontend install/build/test/lint and make Go build depend on the asset
  check. Add the pinned Node setup to `.github/workflows/release.yml` and a
  `.goreleaser.yaml` pre-build hook that runs the frontend build. Add a
  release-like script that fails unless the built CLI serves a non-fallback
  hashed asset; set an initial compressed JS/CSS budget of 750 KiB and lazy-load
  React Flow/Recharts routes. (**Seam:** `embed.FS` asset handler, Vite output,
  GoReleaser hook, and release workflow; **Files:**
  `internal/web/assets.go` (new), `internal/web/assets_test.go` (new),
  `internal/web/prebuilt/build/index-fallback.html` (tracked fallback),
  `Makefile`, `.gitignore`, `web/vite.config.ts`,
  `.goreleaser.yaml`, `.github/workflows/release.yml`,
  `scripts/verify-console-assets.sh` (new), `docs/web-console.md`,
  `CHANGELOG.md`; **Verify:**
  `npm run build --prefix web`, `go test ./internal/web -run 'TestAssets'`,
  `make web-test`, `make build`, `go test ./...`, and
  `scripts/verify-console-assets.sh`; **Commit:** `build: embed console assets in bridgectl`.)

### Phase 3 — Read-only BFF and operational data

- [x] **Task 3.1: Add safe context and deployment projections.** Implement local
  BFF routes that require the per-launch capability and exact local Host/Origin
  checks, enumerate configured contexts without returning API tokens, auth
  keys, ERP credentials, or raw auth configuration, and accept only a known
  context name plus a fixed operation. Do not implement arbitrary upstream URL
  forwarding. Validate URL schemes/hosts, reject redirects, forward no browser
  headers, bound JSON body sizes, and return stable JSON with context name,
  server identity, MCP server identity, current selection, and
  local-console connectivity state. Cover missing contexts, hostile Host/Origin,
  missing capability, malformed configured URLs, redirect-to-private-target,
  token precedence, upstream timeout, and redacted errors with `httptest`.
  (**Seam:** `config.Config`, secure bridge client, local `http.ServeMux`;
  **Files:** `internal/web/context_api.go` (new),
  `internal/web/context_api_test.go` (new), `internal/web/security_test.go`,
  `internal/web/server.go`, `docs/web-console.md`, `CHANGELOG.md`; **Verify:** `go test ./internal/web -run 'TestContext|TestRedact|TestUpstream|TestCapability|TestOrigin'`; **Commit:** `feat: expose safe console contexts`.)

- [x] **Task 3.2: Add explicit read-only health, tools, and cache adapters.** Add
  typed local routes for configured contexts using the explicit target map:
  health from `/mcp/health`, tools from `/apis/erpbridge.io/v1/tools`, and cache
  stats from `/api/cache/stats`. Return a narrow safe DTO: omit tool security
  `credentialRef`, registry auth fields, raw ERP URLs where not required, raw
  response bodies, and upstream headers. Make the privileged admin-token
  delegation explicit and report `unavailable` when the configured credential
  cannot list tools. Keep all local methods read-only. (**Seam:** existing
  ERPBridge HTTP routes and `internal/mcp/server.go:634-646`; **Files:**
  `internal/web/observability_api.go` (new),
  `internal/web/safe_dto.go` (new),
  `internal/web/observability_api_test.go` (new), `internal/web/server.go`,
  `docs/web-console.md`, `CHANGELOG.md`; **Verify:** `go test ./internal/web -run 'Test(Health|Tools|Cache|SafeDTO)'`; **Commit:** `feat: proxy ERPBridge health and inventory data`.)

- [x] **Task 3.3: Add a safe log-event projection and SSE adapter.** Parse recent
  JSON and upstream SSE events into a strict DTO containing only timestamp,
  level, component, tool name, request ID, and a separately tested safe
  summary. Omit unknown fields and raw payloads; apply key and string redaction
  for credentials/PII; reject malformed or oversized events; cap concurrent
  streams and cancel upstream reads when the browser disconnects. Do not
  forward the current arbitrary JSON records verbatim. (**Seam:**
  `internal/mcp/server.go:983-1020`, `internal/logger.RedactArgs`, and local
  SSE response; **Files:** `internal/web/log_projection.go` (new),
  `internal/web/log_projection_test.go` (new),
  `internal/web/observability_api.go`,
  `internal/web/observability_api_test.go`, `docs/web-console.md`,
  `CHANGELOG.md`; **Verify:** `go test ./internal/web -run 'Test(Log|SSE|Redact|Malformed|SlowConsumer)'`; **Commit:** `feat: safely stream console logs`.)

- [x] **Task 3.4: Add bounded metrics parsing and snapshot adapters.** Parse only
  documented Prometheus families into typed snapshots. Preserve cumulative
  totals, compute session-local rates only from successive samples, use
  histogram sum/count for average latency, omit unsupported percentiles, and
  include sample-window start time. Reject malformed/oversized exposition and
  ignore unknown families without failing the page. (**Seam:** `/metrics` and
  `internal/metrics/metrics.go:18-88`; **Files:**
  `internal/web/metrics.go` (new), `internal/web/metrics_test.go` (new),
  `internal/web/observability_api.go`, `docs/web-console.md`, `CHANGELOG.md`;
  **Verify:** `go test ./internal/web -run 'TestMetrics'`; **Commit:** `feat: normalize live console metrics`.)

- [x] **Task 3.5: Add the console application shell and deployment overview.**
  Build the frontend routes for Overview, Deployments/Contexts, Logs, Metrics,
  Tools, Topology, and Settings for theme/about information. Implement context switching
  without changing the user's persistent active context, refresh/last-updated
  states, status cards, tool counts, cache summaries, current metric summaries,
  and clear unavailable/unauthorized/degraded states. Keep all actions
  read-only. (**Seam:** frontend route/data hooks and BFF JSON contracts;
  **Files:** `web/src/app/`, `web/src/routes/`, `web/src/hooks/`,
  `web/src/components/overview/`, `web/src/components/deployments/`, focused
  `*.test.tsx`, `docs/web-console.md`, `CHANGELOG.md`; **Verify:**
  `npm run typecheck --prefix web && npm test --prefix web -- --run`; **Commit:** `feat: add console overview and contexts`.)

- [x] **Task 3.6: Add live logs and metrics views.** Use `EventSource` for the
  safe proxied log stream, retain bounded recent entries in browser memory,
  support filters for level/component/tool/request ID, and show reconnect/error
  states. Consume typed metric snapshots, maintain a bounded in-memory sample
  window for current-session sparklines, label cumulative/session-local values,
  and show a clear message when historical Prometheus data is unavailable. Add
  a text/table alternative for every chart and log stream. Do not persist logs
  or metrics in the console. (**Seam:** safe SSE projection and typed metrics
  BFF contracts; **Files:** `web/src/routes/logs/`,
  `web/src/routes/metrics/`, `web/src/lib/metrics.ts`,
  `web/src/components/logs/`, `web/src/components/metrics/`, focused tests,
  `docs/web-console.md`, `CHANGELOG.md`; **Verify:** `npm run typecheck --prefix web && npm test --prefix web -- --run`; **Commit:** `feat: add live logs and metrics views`.)

### Phase 4 — API-to-MCP topology

- [x] **Task 4.1: Define and implement the topology aggregation contract.** Add a
  read-only local BFF endpoint that loads sanitized local API registry entries
  and remote tool definitions, then returns typed nodes and edges. Strip all
  credential fields, including `AuthHeader`, `AuthKey`, `AuthUsername`,
  `AuthToken`, and tool `credentialRef`. Normalize URL trailing slashes,
  methods, default ports, and local ERP base substitutions before matching.
  Return `exact`, `base-prefix`, `ambiguous`, or `unresolved` match kinds;
  include selected-context match state because the local registry has no
  context ownership; never claim an inferred base-prefix edge is authoritative.
  Include MCP transport nodes, stable identifiers, bounded node/edge counts,
  and a filtered/aggregated response for large inventories. Add fixtures for
  generated tools, manually authored tools, versioned tools, duplicate/base
  endpoint matches, cross-context candidates, and missing registry entries.
  (**Seam:** `idp.Registry.List`, tool registry GET, and
  `Tool.Spec.Execution`; **Files:** `internal/web/topology.go` (new),
  `internal/web/topology_test.go` (new), `internal/web/context_api.go`,
  `docs/web-console.md`, `CHANGELOG.md`; **Verify:**
  `go test ./internal/web -run 'TestTopology'`; **Commit:** `feat: aggregate API to MCP topology`.)

- [x] **Task 4.2: Build the topology canvas and path inspector.** Lazy-load the
  React Flow route and render API, MCP tool, MCP transport, and unresolved
  endpoint nodes with Lucide icons and semantic node colors. Add pan/zoom,
  fit view, selection, filtering by context/module/tool/status, edge labels,
  visible focus, and a right-side detail drawer. Cap rendered nodes and edges;
  retain filtered and aggregated modes for large inventories. Provide an
  equivalent keyboard-accessible HTML list/path view containing the same
  relationships and details. Keep a compact single-tool path view for
  troubleshooting. Show method, endpoint path, tool version, response path,
  schemas, lifecycle, security role presence, cache configuration, match kind,
  and resolution state while omitting credentials and full upstream URLs; show
  only the method, path, and selected-context label. (**Seam:** React Flow
  node/edge model and topology BFF contract; **Files:**
  `web/src/routes/topology/`, `web/src/components/topology/`,
  `web/src/components/details/`, `web/src/components/topology/TopologyList.tsx`,
  focused tests, `docs/web-console.md`, `CHANGELOG.md`; **Verify:**
  `npm run typecheck --prefix web && npm test --prefix web -- --run`, plus
  automated focus/contrast assertions and the manual WCAG 2.2 AA checklist;
  **Commit:** `feat: visualize API to MCP paths`.)

- [x] **Task 4.3: Add server metadata needed for deployment cards.** Add a
  minimal authenticated read-only server-info response containing ERPBridge
  version/build identity, runtime/cache backend label, active tool count, and
  observed timestamp without exposing credentials or ERP configuration. Pass a
  safe build/runtime-info struct from `services/erpbridge-server/main.go` into
  the MCP server and expose a narrow cache backend-name accessor instead of
  reaching into `cache.Manager` internals. Keep `/mcp/health` compatibility
  unchanged. Add server tests, API documentation, and console fallback behavior
  when older deployments return 404. (**Seam:**
  `services/erpbridge-server/main.go`, `internal/mcp/server.go` routing,
  `internal/cache/manager.go`, and existing auth wrapper; **Files:**
  `internal/mcp/info_api.go` (new), `internal/mcp/info_api_test.go` (new),
  `services/erpbridge-server/main.go`, `services/erpbridge-server/main_test.go`,
  `internal/cache/manager.go`, `internal/cache/manager_test.go`, `docs/api.md`,
  `docs/web-console.md`, `CHANGELOG.md`, `web/src/components/deployments/`;
  **Verify:** `go test ./internal/mcp ./internal/cache ./services/erpbridge-server -run 'Test.*Info|Test.*Health|Test.*Backend'` and frontend tests; **Commit:** `feat: expose safe ERPBridge server metadata`.)

### Phase 5 — Integration, packaging, and documentation

- [x] **Task 5.1: Test the complete local web flow and threat boundary.** Add
  integration tests that start an `httptest` ERPBridge-like upstream with
  split Server/MCPServer routes, health, tools, cache, safe recent logs, SSE,
  metrics, and info responses; load a temporary bridgectl config and registry;
  start the local console server; and verify capability bootstrap, exact
  Host/Origin checks, context projection, authentication forwarding, disabled
  redirects, route allow-listing, safe DTOs, SPA fallback, SSE cancellation,
  metric snapshots, and graceful shutdown. Include tests proving tokens,
  credential fields, raw log payloads, sensitive messages, and upstream error
  bodies never appear in HTML, JSON, logs, or error responses. (**Seam:** local
  `internal/web` server plus `httptest.Server`; **Files:**
  `internal/web/integration_test.go`, `internal/web/security_test.go`,
  `internal/web/log_projection_test.go`, `internal/bridgeclient/client_test.go`,
  `internal/config/config_test.go`; **Verify:**
  `go test ./internal/web ./internal/bridgeclient ./internal/config`; **Commit:** `test: cover embedded console integration`.)

- [x] **Task 5.2: Add deterministic developer and CI quality gates.** Ensure
  `Makefile` exposes `web-install`, `web-build`, `web-test`, and `web-lint`,
  `make build` produces a usable `bridgectl web`, ordinary Go tests compile
  against the tracked fallback asset, generated frontend output is ignored,
  and `make clean` removes only generated output. Add a new
  `.github/workflows/console.yml` that pins Node from `web/.nvmrc`, runs npm
  lockfile install/typecheck/test/build/lint, runs Go tests, and runs targeted
  lint without a browser or network at test time. (**Seam:** existing Makefile
  build/test targets and new CI job; **Files:** `Makefile`, `.gitignore`,
  `.github/workflows/console.yml` (new), `web/package-lock.json`; **Verify:**
  `make web-test`, `make build`, `go test ./...`, `git diff --check`, and the
  workflow's commands locally; **Commit:** `build: integrate console quality checks`.)

- [x] **Task 5.3: Audit documentation and public-doc synchronization.** Verify
  that each preceding behavior task already updated its relevant in-repository
  guide, generated CLI reference, and `CHANGELOG.md` entry in the owning
  commit. Add only missing usage, security, data-source, live-only metrics,
  topology-resolution, plugin-gate, and troubleshooting details. Update the
  matching pages in `../erpbridge-docs` using that repository's required plan
  workflow; plugin-aware documentation is added only after the completed plugin
  contract and implementation are verified. (**Seam:** in-repo docs and generated Cobra docs; **Files:**
  `docs/web-console.md`, `docs/cli/bridgectl_web.md` (generated),
  `docs/README.md`, `CHANGELOG.md`,
  `../erpbridge-docs/.agents/plans/Plan-erpbridge-console.md`, matching public
  docs and sidebar; **Verify:** run the CLI doc generator into a temporary
  directory and compare the generated web page, run `make test`, targeted
  `golangci-lint`, and `npm run build` in `../erpbridge-docs`; **Commit:** `docs: audit ERPBridge console documentation`.)

### Phase 6 — Plugin UI integration, completed after the plugin plan

- [x] **Task 6.1: Revalidate and map the completed plugin contract.** Only after
  the dependency gate passes, read the completed plugin plan, final API docs,
  and contract tests. Confirm the exact list/get routes, resource JSON shapes,
  version identity, active state, binding target, `after_response` phase,
  priority, failure policy, timeout, and the documented absence/presence of
  health discovery. Do not proceed if the final contract differs without a
  new plan review. (**Seam:** completed plugin API contract; **Files:**
  `docs/web-console.md`, `internal/web/plugin_contract.go` (new), contract
  fixture files; **Verify:** plugin plan final verification plus a focused
  contract fixture test; **Commit:** `chore: align console with plugin contract`.)

- [x] **Task 6.2: Add read-only plugin and binding BFF projections.** Add
  feature-detected local routes that list sanitized `Plugin` and
  `PluginBinding` resources for a selected context. Preserve exact versions,
  binding order, phase, failure policy, timeout, and target-tool references;
  expose only `endpointConfigured` and `configurationPresent` booleans, never
  plugin endpoint URLs, static binding configuration, credentials, or raw
  invocation payloads. A 404 from an older deployment must produce an
  unavailable feature state rather than a failed console. (**Seam:** final
  plugin registry GET routes and existing local BFF allow-list; **Files:**
  `internal/web/plugin_api.go` (new), `internal/web/plugin_api_test.go` (new),
  `internal/web/safe_dto.go`, `internal/web/context_api.go`,
  `docs/web-console.md`, `CHANGELOG.md`; **Verify:**
  `go test ./internal/web -run 'TestPlugin'`; **Commit:** `feat: expose read-only plugin metadata`.)

- [x] **Task 6.3: Add plugin nodes, binding edges, and tool detail panels.** Extend
  topology data with plugin and binding nodes only when the feature is
  available. Show exact tool/plugin versions, `after_response`, priority,
  failure policy, timeout, configuration-present state, and health as
  `unknown` unless the completed contract provides an approved health field.
  Preserve the unresolved state for tools without bindings and do not add
  mutation controls. Provide the same information in an accessible list view.
  (**Seam:** topology aggregation and React Flow node model; **Files:**
  `internal/web/topology.go`, `internal/web/topology_test.go`,
  `web/src/components/topology/`, `web/src/routes/plugins/`, focused tests,
  `docs/web-console.md`, `CHANGELOG.md`; **Verify:** plugin fixture integration
  tests plus `go test ./internal/web` and `npm test --prefix web -- --run`; **Commit:** `feat: visualize plugin bindings`.)

- [x] **Task 6.4: Update plugin-aware docs only after implementation is stable.**
  Document plugin visualization, version pinning, operator-owned plugin
  deployment, failure-policy meaning, and health limitations in both
  repositories. Add the Unreleased changelog entry and run the plugin plan's
  required final tests without duplicating plugin source or generated assets.
  (**Seam:** completed plugin docs and console docs; **Files:**
  `docs/web-console.md`, `docs/api.md`, `CHANGELOG.md`, matching
  `../erpbridge-docs` pages; **Verify:** plugin plan final gates, `make test`,
  targeted lint, `npm run build` in `../erpbridge-docs`, and `git diff --check`; **Commit:** `docs: document plugin console integration`.)

## Verification

### Phase gates

1. **Phase 0:** The console contract exists and the plugin dependency gate was
   recorded before plugin UI work started.
2. **Phase 1:** `bridgectl web --url --no-open` starts on literal loopback,
   reports a one-time capability URL, rejects hostile Host/Origin/capability
   requests, and stops cleanly; existing CLI tests remain green.
3. **Phase 2:** Pinned Node/npm frontend typecheck/tests/build pass; `go build`
   serves generated embedded assets rather than fallback; dev mode proxies Vite
   only when explicitly enabled; release and compressed-asset checks pass.
4. **Phase 3:** A temporary split-endpoint context can display safe health,
   tools, cache, logs, and metrics; no token, credential, raw payload, unsafe
   log message, redirect response, or upstream error body appears in any browser
   response, log, or error.
5. **Phase 4:** Generated and manually authored tools are represented with
   exact/inferred/ambiguous/unresolved match states; cross-context ownership is
   not falsely claimed; topology has an accessible list alternative and a
   bounded canvas.
6. **Phase 5:** `make build`, `make test`, frontend tests, targeted lint,
   integration/security tests, release-like asset verification,
   `git diff --check`, and documentation builds are green. Every behavior task
   has its local docs/changelog update in its owning commit and public docs are
   synchronized in the corresponding repository phase.
7. **Phase 6:** The completed plugin plan was revalidated before shipping the
   plugin UI. Plugin resources and exact bindings are read-only,
   feature-detected, version-accurate, endpoint/configuration-safe, and do not
   claim unsupported health.

### Final acceptance criteria

- `bridgectl web` is the only new user-facing command; `bridgectl` remains the
  authoritative automation interface.
- The local server is literal-loopback-only by default, requires a per-launch
  capability for data routes, validates Host/Origin, and has no arbitrary
  upstream proxy.
- Contexts, tokens, ERP credentials, registry auth fields, plugin endpoints or
  configuration, and unsafe/raw log payloads are not exposed to the browser.
- Existing ERPBridge endpoints, MCP behavior, CLI commands, auth behavior, and
  direct invocation envelopes are unchanged except for the explicitly tested,
  documented server-info endpoint.
- A developer can select a context, see deployment availability, inspect tools,
  follow an API-to-MCP path with honest match confidence, watch safely projected
  logs, and view correctly labeled live metrics without manual endpoint/CORS/
  token handling.
- The UI has light/dark/system themes, semantic colors, accessible status text,
  Lucide icons, responsive layout, HTML alternatives for graph/chart data, and
  usable loading/error/empty states.
- GoReleaser and CI build the frontend before the CLI and a release-like binary
  serves a hashed production asset, not the fallback page.
- No persistent console database, Backstage dependency, plugin deployment
  mechanism, or historical metrics store is introduced.

## Open Questions

- None block the finalized draft. Historical Prometheus query integration,
  remote listening, UI mutations, and a least-privilege read-only inventory
  token remain intentionally deferred and require separate decisions before
  entering scope.
- The configured context file is treated as trusted operator input. If a future
  threat model includes untrusted edits to `~/.bridgectl/config.yaml` or its
  environment overrides, destination allow-listing must be expanded before
  supporting those contexts.
