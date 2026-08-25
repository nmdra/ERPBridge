import { lazy, Suspense, useState } from "react";

import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { Skeleton } from "../components/ui/skeleton";
import { TopologyList } from "../components/topology/TopologyList";
import { useTopology, type TopologyNode } from "../hooks/useTopology";

const TopologyCanvas = lazy(() =>
  import("../components/topology/TopologyCanvas").then((module) => ({
    default: module.TopologyCanvas,
  })),
);

export function Topology({ contextName }: { contextName: string }) {
  const topology = useTopology(contextName);
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<TopologyNode | null>(null);
  if (topology.loading) return <Skeleton className="h-48 w-full" />;
  if (!topology.data || topology.error || topology.data.state !== "available")
    return (
      <EmptyState
        title="Topology is unavailable"
        message={
          topology.error ??
          "The selected context has no readable tool inventory."
        }
      />
    );
  const visibleNodes = topology.data.nodes.filter((node) =>
    `${node.label} ${node.kind} ${node.contextState ?? ""}`
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  const visibleEdges = topology.data.edges.filter(
    (edge) =>
      visibleNodes.some((node) => node.id === edge.source) &&
      visibleNodes.some((node) => node.id === edge.target),
  );
  const apiNodes = visibleNodes.filter((node) => node.kind === "erp-api");
  const toolNodes = visibleNodes.filter((node) => node.kind === "mcp-tool");
  const transportNodes = visibleNodes.filter(
    (node) => node.kind === "mcp-transport",
  );
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="font-medium">API to MCP topology</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {topology.data.nodes.length} nodes and{" "}
              {topology.data.edges.length} edges. Match states remain visible.
            </p>
          </div>
          <label className="text-sm" htmlFor="topology-filter">
            Filter
            <input
              className="ml-2 h-9 rounded-md border border-border bg-card px-2"
              id="topology-filter"
              placeholder="module, tool, or status"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
            />
          </label>
        </CardContent>
      </Card>
      <div
        aria-label="Topology node summary"
        className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card p-3 text-sm"
      >
        <span className="rounded-md border border-sky-500/60 bg-sky-500/10 px-2 py-1">
          MCP transport ({transportNodes.length})
        </span>
        <span className="rounded-md border border-primary/60 bg-primary/10 px-2 py-1">
          MCP tools ({toolNodes.length})
        </span>
        <span className="rounded-md border border-emerald-500/60 bg-emerald-500/10 px-2 py-1 font-medium">
          ERP APIs ({apiNodes.length})
        </span>
        {apiNodes.map((node) => (
          <button
            className="rounded-md border border-emerald-500/40 px-2 py-1 text-left text-emerald-700 hover:bg-emerald-500/10 dark:text-emerald-300"
            key={node.id}
            onClick={() => setSelected(node)}
            type="button"
          >
            {node.label}
          </button>
        ))}
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <Suspense fallback={<Skeleton className="h-[30rem] w-full" />}>
          <TopologyCanvas
            nodes={visibleNodes}
            edges={visibleEdges}
            onSelect={setSelected}
          />
        </Suspense>
        {selected ? (
          <Card>
            <CardContent>
              <h2 className="font-medium">{selected.label}</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {selected.kind}{" "}
                {selected.contextState ? `· ${selected.contextState}` : ""}
              </p>
              <p className="mt-2 text-sm">
                {selected.tool?.endpointPath ??
                  selected.api?.endpointPath ??
                  "No endpoint path"}
              </p>
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardContent>
              <h2 className="font-medium">Path inspector</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Select a node to inspect its safe endpoint path and resolution
                state.
              </p>
            </CardContent>
          </Card>
        )}
      </div>
      <TopologyList nodes={visibleNodes} edges={visibleEdges} filter={filter} />
    </div>
  );
}
