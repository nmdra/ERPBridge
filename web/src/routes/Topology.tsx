import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { AlertTriangle, Filter, X } from "lucide-react";

import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { Freshness } from "../components/ui/freshness";
import { Skeleton } from "../components/ui/skeleton";
import { TopologyComponentSidebar } from "../components/topology/TopologyComponentSidebar";
import { TopologyList } from "../components/topology/TopologyList";
import {
  topologyNodeKinds,
  useTopology,
  type TopologyEdge,
  type TopologyMatchKind,
  type TopologyNode,
  type TopologyNodeKind,
  type TopologySelection,
} from "../hooks/useTopology";
import {
  buildEndpointComponents,
  canDrillIntoNode,
  compactTopologyComponentLimit,
  componentForNode,
} from "./topologyPresentation";
import {
  buildTopologyView,
  type TopologyVisualNode,
} from "./topologyViewModel";

const TopologyCanvas = lazy(() =>
  import("../components/topology/TopologyCanvas").then((module) => ({
    default: module.TopologyCanvas,
  })),
);

const nodeKindLabels: Record<TopologyNodeKind, string> = {
  "mcp-transport": "MCP transport",
  "mcp-tool": "MCP tools",
  "erp-api": "ERP APIs",
  "erp-endpoint": "ERP endpoints",
  "plugin-binding": "Bindings",
  "external-plugin": "Plugins",
  "unresolved-endpoint": "Unresolved endpoints",
  "ambiguous-endpoint": "Ambiguous endpoints",
};

const matchKindLabels: Record<TopologyMatchKind, string> = {
  exact: "Exact",
  "base-prefix": "Base prefix",
  ambiguous: "Ambiguous",
  unresolved: "Unresolved",
};

const matchKinds: TopologyMatchKind[] = [
  "exact",
  "base-prefix",
  "ambiguous",
  "unresolved",
];
const contextStates = ["context matched", "unassigned"] as const;
const safeDiagnosticReasons = new Set([
  "The tool has no endpoint.",
  "No ERP APIs are registered.",
  "No registered ERP API matches the endpoint host.",
  "Registered ERP APIs use a different method.",
  "No registered ERP API matches this endpoint.",
  "More than one registered ERP API matches this endpoint.",
]);

function safeDiagnosticReason(reason?: string) {
  return reason && safeDiagnosticReasons.has(reason) ? reason : null;
}

type FilterCheckboxProps = {
  label: string;
  count?: number;
  checked: boolean;
  disabled?: boolean;
  onChange: () => void;
};

function FilterCheckbox({
  label,
  count,
  checked,
  disabled = false,
  onChange,
}: FilterCheckboxProps) {
  return (
    <label className="flex min-h-9 items-center gap-2 rounded-md px-2 text-sm hover:bg-muted has-[:disabled]:opacity-50">
      <input
        checked={checked}
        className="h-4 w-4 rounded border-border text-primary focus:ring-ring"
        disabled={disabled}
        onChange={onChange}
        type="checkbox"
      />
      <span className="min-w-0 flex-1">{label}</span>
      {count !== undefined ? (
        <span className="text-xs tabular-nums text-muted-foreground">
          {count}
        </span>
      ) : null}
    </label>
  );
}

function toggleValue<T extends string>(values: T[], value: T) {
  return values.includes(value)
    ? values.filter((item) => item !== value)
    : [...values, value];
}

function nodeSearchText(node: TopologyNode) {
  return [
    node.label,
    node.kind,
    node.contextState,
    node.tool?.name,
    node.tool?.version,
    node.tool?.endpointPath,
    node.api?.name,
    node.api?.module,
    node.api?.endpointPath,
    node.endpoint?.method,
    node.endpoint?.path,
    node.plugin?.name,
    node.plugin?.version,
    node.binding?.name,
    node.binding?.pluginRef.name,
    node.binding?.pluginRef.version,
    node.binding?.toolRef.name,
    node.binding?.toolRef.version,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
}

function edgeSearchText(edge: TopologyEdge) {
  return `${edge.matchKind} ${edge.contextState ?? "unassigned"}`.toLowerCase();
}

function nodeContextState(node: TopologyNode) {
  return node.contextState || "unassigned";
}

function TopologyInspector({
  selectedNode,
  selectedEdge,
  nodes,
}: {
  selectedNode: TopologyNode | null;
  selectedEdge: TopologyEdge | null;
  nodes: TopologyNode[];
}) {
  if (selectedEdge) {
    const source = nodes.find((node) => node.id === selectedEdge.source);
    const target = nodes.find((node) => node.id === selectedEdge.target);
    return (
      <Card aria-label="Selected topology relationship">
        <CardContent>
          <p className="text-xs uppercase tracking-wider text-muted-foreground">
            Selected relationship
          </p>
          <h2 className="mt-1 font-medium">Selected relationship</h2>
          <p className="mt-1 break-words text-sm text-muted-foreground [overflow-wrap:anywhere]">
            {source?.label ?? "Unknown"} → {target?.label ?? "Unknown"}
          </p>
          <dl className="mt-3 space-y-2 text-sm">
            <div>
              <dt className="text-muted-foreground">Match confidence</dt>
              <dd>
                {matchKindLabels[selectedEdge.matchKind as TopologyMatchKind] ??
                  selectedEdge.matchKind}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Authority</dt>
              <dd>
                {selectedEdge.authoritative
                  ? "Authoritative"
                  : "Inferred or unresolved"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Context state</dt>
              <dd>{selectedEdge.contextState || "Unassigned"}</dd>
            </div>
            {safeDiagnosticReason(selectedEdge.diagnosticReason) ? (
              <div>
                <dt className="text-muted-foreground">Diagnostic reason</dt>
                <dd>{safeDiagnosticReason(selectedEdge.diagnosticReason)}</dd>
              </div>
            ) : null}
            <div>
              <dt className="text-muted-foreground">Source path</dt>
              <dd className="break-words [overflow-wrap:anywhere]">
                {source?.tool?.endpointPath ??
                  source?.api?.endpointPath ??
                  source?.endpoint?.path ??
                  source?.label ??
                  "—"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Target path</dt>
              <dd className="break-words [overflow-wrap:anywhere]">
                {target?.tool?.endpointPath ??
                  target?.api?.endpointPath ??
                  target?.endpoint?.path ??
                  target?.label ??
                  "—"}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>
    );
  }

  if (selectedNode) {
    return (
      <Card aria-label="Selected topology node">
        <CardContent>
          <p className="text-xs uppercase tracking-wider text-muted-foreground">
            Selected node
          </p>
          <h2 className="mt-1 break-words font-medium [overflow-wrap:anywhere]">
            {selectedNode.label}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {nodeKindLabels[selectedNode.kind as TopologyNodeKind] ??
              selectedNode.kind}
          </p>
          <dl className="mt-3 space-y-2 text-sm">
            <div>
              <dt className="text-muted-foreground">Context state</dt>
              <dd>{nodeContextState(selectedNode)}</dd>
            </div>
            {safeDiagnosticReason(selectedNode.diagnosticReason) ? (
              <div>
                <dt className="text-muted-foreground">Diagnostic reason</dt>
                <dd>{safeDiagnosticReason(selectedNode.diagnosticReason)}</dd>
              </div>
            ) : null}
            {selectedNode.tool ? (
              <div>
                <dt className="text-muted-foreground">Endpoint path</dt>
                <dd className="break-words [overflow-wrap:anywhere]">
                  {selectedNode.tool.endpointPath ?? "—"}
                </dd>
              </div>
            ) : null}
            {selectedNode.api ? (
              <div>
                <dt className="text-muted-foreground">Endpoint path</dt>
                <dd className="break-words [overflow-wrap:anywhere]">
                  {selectedNode.api.endpointPath}
                </dd>
              </div>
            ) : null}
            {selectedNode.endpoint ? (
              <>
                <div>
                  <dt className="text-muted-foreground">Method</dt>
                  <dd>{selectedNode.endpoint.method}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Endpoint path</dt>
                  <dd className="break-words [overflow-wrap:anywhere]">
                    {selectedNode.endpoint.path}
                  </dd>
                </div>
              </>
            ) : null}
            {selectedNode.plugin ? (
              <>
                <div>
                  <dt className="text-muted-foreground">Version</dt>
                  <dd>{selectedNode.plugin.version}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Endpoint</dt>
                  <dd>
                    {selectedNode.plugin.endpointConfigured
                      ? "Configured"
                      : "Not configured"}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Health</dt>
                  <dd>Unknown</dd>
                </div>
              </>
            ) : null}
            {selectedNode.binding ? (
              <>
                <div>
                  <dt className="text-muted-foreground">Plugin</dt>
                  <dd>
                    {selectedNode.binding.pluginRef.name}@
                    {selectedNode.binding.pluginRef.version}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Phase and priority</dt>
                  <dd>
                    {selectedNode.binding.phase} ·{" "}
                    {selectedNode.binding.priority}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Failure policy</dt>
                  <dd>{selectedNode.binding.failurePolicy}</dd>
                </div>
              </>
            ) : null}
          </dl>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card aria-label="Topology path inspector">
      <CardContent>
        <h2 className="font-medium">Path inspector</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Select a node or relationship to inspect its safe identity, match
          confidence, and endpoint path.
        </p>
      </CardContent>
    </Card>
  );
}

export function Topology({ contextName }: { contextName: string }) {
  const topology = useTopology(contextName);
  const [search, setSearch] = useState("");
  const [selectedKinds, setSelectedKinds] = useState<TopologyNodeKind[]>([]);
  const [selectedMatches, setSelectedMatches] = useState<TopologyMatchKind[]>(
    [],
  );
  const [selectedContexts, setSelectedContexts] = useState<string[]>([]);
  const [selection, setSelection] = useState<TopologySelection>(null);
  const [focusedComponentID, setFocusedComponentID] = useState<string | null>(
    null,
  );
  const [expandedClusters, setExpandedClusters] = useState<Set<string>>(
    () => new Set(),
  );
  const [activeTab, setActiveTab] = useState<"topology" | "relationships">(
    "topology",
  );

  const data = topology.data;
  const allNodes = useMemo(() => data?.nodes ?? [], [data?.nodes]);
  const allEdges = useMemo(() => data?.edges ?? [], [data?.edges]);
  const normalizedSearch = search.trim().toLowerCase();

  const visibleNodes = useMemo(() => {
    const edgeMatchedNodeIDs = new Set(
      normalizedSearch
        ? allEdges
            .filter((edge) => edgeSearchText(edge).includes(normalizedSearch))
            .flatMap((edge) => [edge.source, edge.target])
        : [],
    );
    return allNodes.filter((node) => {
      const kindMatches =
        selectedKinds.length === 0 ||
        selectedKinds.includes(node.kind as TopologyNodeKind);
      const queryMatches =
        !normalizedSearch ||
        nodeSearchText(node).includes(normalizedSearch) ||
        edgeMatchedNodeIDs.has(node.id);
      const contextMatches =
        selectedContexts.length === 0 ||
        selectedContexts.includes(nodeContextState(node));
      return kindMatches && queryMatches && contextMatches;
    });
  }, [allEdges, allNodes, normalizedSearch, selectedContexts, selectedKinds]);

  const visibleEdges = useMemo(() => {
    const visibleNodeIDs = new Set(visibleNodes.map((node) => node.id));
    return allEdges.filter((edge) => {
      const matchMatches =
        selectedMatches.length === 0 ||
        selectedMatches.includes(edge.matchKind as TopologyMatchKind);
      const contextMatches =
        selectedContexts.length === 0 ||
        selectedContexts.includes(edge.contextState || "unassigned");
      return (
        visibleNodeIDs.has(edge.source) &&
        visibleNodeIDs.has(edge.target) &&
        matchMatches &&
        contextMatches
      );
    });
  }, [allEdges, selectedContexts, selectedMatches, visibleNodes]);

  const allEndpointComponents = useMemo(
    () => buildEndpointComponents(allNodes, allEdges),
    [allEdges, allNodes],
  );
  const endpointComponents = useMemo(
    () => buildEndpointComponents(visibleNodes, visibleEdges),
    [visibleEdges, visibleNodes],
  );
  const compactMode = !focusedComponentID;
  const compactComponents = endpointComponents.slice(
    0,
    compactTopologyComponentLimit,
  );
  const focusedComponent = allEndpointComponents.find(
    (component) => component.endpoint.id === focusedComponentID,
  );
  const topologyMode = focusedComponent ? "focused" : "overview";
  const visualGraph = useMemo(
    () =>
      buildTopologyView({
        components: focusedComponent
          ? allEndpointComponents
          : compactComponents,
        edges: focusedComponent ? allEdges : visibleEdges,
        expandedClusters,
        focusedComponentID,
        mode: topologyMode,
        nodes: focusedComponent ? allNodes : visibleNodes,
        selection,
      }),
    [
      allEdges,
      allEndpointComponents,
      allNodes,
      compactComponents,
      expandedClusters,
      focusedComponent,
      focusedComponentID,
      selection,
      topologyMode,
      visibleEdges,
      visibleNodes,
    ],
  );

  const selectedNode =
    selection?.kind === "node"
      ? (allNodes.find((node) => node.id === selection.id) ?? null)
      : null;
  const selectedEdge =
    selection?.kind === "edge"
      ? (allEdges.find((edge) => edge.id === selection.id) ?? null)
      : null;

  useEffect(() => {
    setSelection(null);
    setFocusedComponentID(null);
    setExpandedClusters(new Set());
    setActiveTab("topology");
  }, [contextName]);

  useEffect(() => {
    if (
      focusedComponentID &&
      !allEndpointComponents.some(
        (component) => component.endpoint.id === focusedComponentID,
      )
    ) {
      setFocusedComponentID(null);
      setSelection(null);
    }
  }, [allEndpointComponents, focusedComponentID]);

  useEffect(() => {
    if (!selection) return;
    const visible =
      selection.kind === "node"
        ? visibleNodes.some((node) => node.id === selection.id)
        : visibleEdges.some((edge) => edge.id === selection.id);
    if (!visible) setSelection(null);
  }, [selection, visibleEdges, visibleNodes]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setSelection(null);
      setExpandedClusters(new Set());
      setFocusedComponentID(null);
      setActiveTab("topology");
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const focusComponent = useCallback((componentID: string, nodeID: string) => {
    setExpandedClusters(new Set());
    setFocusedComponentID(componentID);
    setSelection({ kind: "node", id: nodeID });
  }, []);
  const expandCluster = useCallback((node: TopologyVisualNode) => {
    if (!node.componentId) return;
    setFocusedComponentID(node.componentId);
    setExpandedClusters((current) => {
      const next = new Set(current);
      next.add(node.id);
      return next;
    });
    setSelection({ kind: "node", id: node.componentId });
  }, []);
  const selectNode = useCallback(
    (id: string) => {
      setActiveTab("topology");
      const component = componentForNode(allEndpointComponents, id);
      const selected = allNodes.find((node) => node.id === id);
      setSelection({ kind: "node", id });
      if (component && canDrillIntoNode(selected)) {
        setFocusedComponentID(component.endpoint.id);
      }
    },
    [allEndpointComponents, allNodes],
  );
  const selectEdge = useCallback((id: string) => {
    setActiveTab("topology");
    setSelection({ kind: "edge", id });
  }, []);
  const clearFilters = useCallback(() => {
    setSearch("");
    setSelectedKinds([]);
    setSelectedMatches([]);
    setSelectedContexts([]);
    setExpandedClusters(new Set());
    setFocusedComponentID(null);
    setSelection(null);
    setActiveTab("topology");
  }, []);
  const filterCount =
    selectedKinds.length +
    selectedMatches.length +
    selectedContexts.length +
    (normalizedSearch ? 1 : 0);

  if (topology.loading) {
    return (
      <div className="space-y-6" aria-busy="true">
        <PageHeader
          description="Investigate safe relationships across tools, APIs, bindings, plugins, and unresolved endpoints."
          eyebrow="Diagnose"
          title="Integration topology"
        />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }
  if (!data || topology.error || data.state !== "available") {
    return (
      <div className="space-y-6">
        <PageHeader
          description="Investigate safe relationships across tools, APIs, bindings, plugins, and unresolved endpoints."
          eyebrow="Diagnose"
          title="Integration topology"
        />
        <div className="space-y-3">
          <EmptyState
            title="Topology is unavailable"
            message={
              topology.error ??
              "The selected context has no readable tool inventory."
            }
          />
          <button
            className="rounded-md border border-border px-3 py-2 text-sm font-medium hover:bg-muted"
            onClick={topology.refresh}
            type="button"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <button
            className="rounded-md border border-border px-3 py-2 text-sm font-medium hover:bg-muted"
            onClick={topology.refresh}
            type="button"
          >
            Refresh
          </button>
        }
        description="Investigate safe relationships across tools, APIs, bindings, plugins, and unresolved endpoints."
        eyebrow="Diagnose"
        title="Integration topology"
      />
      <Freshness lastUpdated={topology.lastUpdated} stale={topology.stale} />
      <Card>
        <CardContent className="space-y-4">
          <div
            className="flex flex-wrap items-center gap-2 text-sm tabular-nums text-muted-foreground"
            role="status"
            aria-live="polite"
          >
            <span>
              {visibleNodes.length} of {allNodes.length} nodes
            </span>
            <span aria-hidden="true">·</span>
            <span>
              {visibleEdges.length} of {allEdges.length} edges
            </span>
          </div>
          {data.truncated ? (
            <div
              className="flex items-start gap-2 rounded-md border border-amber-500/50 bg-amber-500/10 p-3 text-sm"
              role="alert"
            >
              <AlertTriangle
                aria-hidden="true"
                className="mt-0.5 shrink-0"
                size={16}
              />
              <span>
                This topology is incomplete. {data.omitted?.nodes ?? 0} nodes
                and {data.omitted?.edges ?? 0} edges were omitted by safety
                limits.
              </span>
            </div>
          ) : null}
          <div className="flex flex-wrap items-center gap-2">
            <label
              className="flex min-w-[15rem] flex-1 items-center gap-2 text-sm"
              htmlFor="topology-filter"
            >
              <span className="sr-only">Search topology</span>
              <Filter aria-hidden="true" size={16} />
              <input
                className="h-9 w-full rounded-md border border-border bg-card px-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                id="topology-filter"
                placeholder="Search names, paths, versions, or match states"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </label>
            {filterCount ? (
              <button
                className="inline-flex h-9 items-center gap-1 rounded-md border border-border px-3 text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                onClick={clearFilters}
                type="button"
              >
                <X aria-hidden="true" size={15} />
                Clear filters ({filterCount})
              </button>
            ) : null}
          </div>
          <div className="grid gap-4 border-t border-border pt-4 md:grid-cols-3">
            <fieldset>
              <legend className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Node types
              </legend>
              <div className="mt-1 grid gap-1">
                {topologyNodeKinds.map((kind) => (
                  <FilterCheckbox
                    checked={selectedKinds.includes(kind)}
                    count={allNodes.filter((node) => node.kind === kind).length}
                    disabled={!allNodes.some((node) => node.kind === kind)}
                    key={kind}
                    label={nodeKindLabels[kind]}
                    onChange={() =>
                      setSelectedKinds((current) => toggleValue(current, kind))
                    }
                  />
                ))}
              </div>
            </fieldset>
            <fieldset>
              <legend className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Match confidence
              </legend>
              <div className="mt-1 grid gap-1">
                {matchKinds.map((kind) => (
                  <FilterCheckbox
                    checked={selectedMatches.includes(kind)}
                    count={
                      allEdges.filter((edge) => edge.matchKind === kind).length
                    }
                    disabled={!allEdges.some((edge) => edge.matchKind === kind)}
                    key={kind}
                    label={matchKindLabels[kind]}
                    onChange={() =>
                      setSelectedMatches((current) =>
                        toggleValue(current, kind),
                      )
                    }
                  />
                ))}
              </div>
            </fieldset>
            <fieldset>
              <legend className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Context state
              </legend>
              <div className="mt-1 grid gap-1">
                {contextStates.map((state) => (
                  <FilterCheckbox
                    checked={selectedContexts.includes(state)}
                    count={
                      allNodes.filter(
                        (node) => nodeContextState(node) === state,
                      ).length
                    }
                    disabled={
                      !allNodes.some((node) => nodeContextState(node) === state)
                    }
                    key={state}
                    label={
                      state === "context matched"
                        ? "Context matched"
                        : "Unassigned"
                    }
                    onChange={() =>
                      setSelectedContexts((current) =>
                        toggleValue(current, state),
                      )
                    }
                  />
                ))}
              </div>
            </fieldset>
          </div>
        </CardContent>
      </Card>

      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card p-3 text-sm">
        {topologyNodeKinds.map((kind) => {
          const count = visibleNodes.filter(
            (node) => node.kind === kind,
          ).length;
          if (!count) return null;
          return (
            <span
              className="rounded-md border border-border px-2 py-1"
              key={kind}
            >
              {nodeKindLabels[kind]} ({count})
            </span>
          );
        })}
        {selectedNode || selectedEdge ? (
          <button
            className="ml-auto inline-flex items-center gap-1 text-sm text-primary hover:underline"
            onClick={() => setSelection(null)}
            type="button"
          >
            <X aria-hidden="true" size={14} />
            Clear selection
          </button>
        ) : null}
      </div>

      {compactMode ? (
        <div
          className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm"
          role="status"
        >
          <span>
            Compact overview: showing {compactComponents.length} of{" "}
            {endpointComponents.length} components. Select a component to show
            its related MCP graph.
          </span>
          {endpointComponents.length > compactTopologyComponentLimit ? (
            <span className="text-muted-foreground">
              {endpointComponents.length - compactTopologyComponentLimit} more
              available through search and filters.
            </span>
          ) : null}
        </div>
      ) : null}
      {focusedComponent ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm">
          <span>
            Showing related nodes for {focusedComponent.endpoint.label}.
          </span>
          <button
            className="text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => {
              setExpandedClusters(new Set());
              setFocusedComponentID(null);
              setSelection(null);
            }}
            type="button"
          >
            Back to compact overview
          </button>
        </div>
      ) : null}

      <div
        aria-label="Topology views"
        className="flex flex-wrap gap-1 border-b border-border"
        role="tablist"
      >
        {(["topology", "relationships"] as const).map((tab) => (
          <button
            aria-controls={`${tab}-panel`}
            aria-selected={activeTab === tab}
            className={`rounded-t-lg px-3 py-2 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${activeTab === tab ? "border-b-2 border-primary text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}
            key={tab}
            onClick={() => setActiveTab(tab)}
            onKeyDown={(event) => {
              if (
                event.key !== "ArrowLeft" &&
                event.key !== "ArrowRight" &&
                event.key !== "Home" &&
                event.key !== "End"
              ) {
                return;
              }
              event.preventDefault();
              if (event.key === "Home") setActiveTab("topology");
              else if (event.key === "End") setActiveTab("relationships");
              else {
                setActiveTab(tab === "topology" ? "relationships" : "topology");
              }
            }}
            role="tab"
            tabIndex={activeTab === tab ? 0 : -1}
            type="button"
          >
            {tab === "topology" ? "Topology" : "Relationships"}
          </button>
        ))}
      </div>
      {activeTab === "topology" ? (
        <div
          className="space-y-4"
          id="topology-panel"
          role="tabpanel"
          aria-label="Topology canvas"
        >
          <Suspense fallback={<Skeleton className="h-[36rem] w-full" />}>
            <TopologyCanvas
              graph={visualGraph}
              mode={topologyMode}
              onClearSelection={() => {
                setExpandedClusters(new Set());
                setSelection(null);
                setFocusedComponentID(null);
              }}
              onExpandCluster={expandCluster}
              onSelectEdge={selectEdge}
              onSelectNode={selectNode}
            />
          </Suspense>
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
            <TopologyComponentSidebar
              components={endpointComponents}
              focusedComponentID={focusedComponentID}
              onFocus={focusComponent}
              tools={visibleNodes.filter((node) => node.kind === "mcp-tool")}
            />
            <TopologyInspector
              nodes={allNodes}
              selectedEdge={selectedEdge}
              selectedNode={selectedNode}
            />
          </div>
        </div>
      ) : null}
      <div
        aria-label="Topology relationships"
        aria-live="polite"
        className={activeTab === "topology" ? "sr-only" : undefined}
        inert={activeTab === "topology" ? true : undefined}
        id="relationships-panel"
        role="tabpanel"
      >
        <TopologyList
          edges={visibleEdges}
          nodes={visibleNodes}
          onSelectEdge={selectEdge}
          onSelectNode={selectNode}
          selection={selection}
        />
      </div>
    </div>
  );
}
