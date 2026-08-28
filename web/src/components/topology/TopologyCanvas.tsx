import { useCallback, useMemo, type MouseEvent } from "react";

import {
  Background,
  Controls,
  Handle,
  MiniMap,
  Panel,
  Position,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import {
  AlertTriangle,
  Database,
  Link2,
  Network,
  Plug,
  Server,
  Wrench,
} from "lucide-react";
import "@xyflow/react/dist/style.css";

import type {
  TopologyEdge,
  TopologyNode,
  TopologySelection,
} from "../../hooks/useTopology";

import {
  buildTopologyFlow,
  handleIDs,
  type FlowNodeData,
} from "./topologyFlow";

function NodeIcon({ kind }: { kind: string }) {
  if (kind === "erp-api") return <Database aria-hidden="true" size={15} />;
  if (kind === "mcp-tool") return <Wrench aria-hidden="true" size={15} />;
  if (kind === "mcp-transport") return <Network aria-hidden="true" size={15} />;
  if (kind === "external-plugin") return <Plug aria-hidden="true" size={15} />;
  if (kind === "plugin-binding") return <Link2 aria-hidden="true" size={15} />;
  if (kind === "unresolved-endpoint") {
    return <AlertTriangle aria-hidden="true" size={15} />;
  }
  return <Server aria-hidden="true" size={15} />;
}

function nodeClass(
  kind: string,
  shapeClass: string,
  compact: boolean,
  selected: boolean,
  dimmed: boolean,
) {
  const colorClass =
    kind === "erp-api"
      ? "border-emerald-500/60 bg-emerald-500/10"
      : kind === "mcp-tool"
        ? "border-primary/60 bg-primary/10"
        : kind === "mcp-transport"
          ? "border-sky-500/60 bg-sky-500/10"
          : kind === "external-plugin"
            ? "border-violet-500/60 bg-violet-500/10"
            : kind === "plugin-binding"
              ? "border-amber-500/60 bg-amber-500/10"
              : kind === "ambiguous-endpoint"
                ? "border-orange-500/60 bg-orange-500/10"
                : kind === "unresolved-endpoint"
                  ? "border-destructive/60 bg-destructive/10"
                  : "border-border bg-card";
  return [
    `relative ${compact ? "w-56 min-w-56 max-w-56" : "w-40 min-w-40 max-w-40"} ${shapeClass} border px-3 py-2 text-card-foreground shadow-sm outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring`,
    colorClass,
    selected ? "z-20 ring-2 ring-primary shadow-lg" : "",
    dimmed ? "opacity-25" : "opacity-100",
  ].join(" ");
}

function TopologyFlowNode({ data }: { data: FlowNodeData }) {
  const handles = handleIDs(data.kind);

  return (
    <div
      aria-label={`${data.label}, ${data.presentationLabel}`}
      aria-pressed={data.selected}
      className={nodeClass(
        data.kind,
        data.shapeClass,
        data.compact,
        data.selected,
        data.dimmed,
      )}
      onClick={data.onSelect}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          data.onSelect();
        }
      }}
      role="button"
      tabIndex={0}
    >
      <Handle
        aria-label={`Incoming connection to ${data.label}`}
        id={handles.target}
        position={Position.Left}
        type="target"
      />
      <div className="flex items-center gap-2 text-xs font-medium">
        <NodeIcon kind={data.kind} />
        <span
          className={`min-w-0 ${data.compact ? "break-words" : "truncate"}`}
          title={data.label}
        >
          {data.label}
        </span>
      </div>
      <span className="mt-1 block text-[10px] uppercase tracking-wider text-muted-foreground">
        {data.kind}
      </span>
      {data.summary ? (
        <span className="mt-1 block break-words text-[10px] text-muted-foreground">
          {data.summary}
        </span>
      ) : null}
      <Handle
        aria-label={`Outgoing connection from ${data.label}`}
        id={handles.source}
        position={Position.Right}
        type="source"
      />
    </div>
  );
}

const nodeTypes = { topology: TopologyFlowNode };

function miniMapNodeColor(node: Node<FlowNodeData>) {
  if (node.data?.kind === "external-plugin") return "#8b5cf6";
  if (node.data?.kind === "mcp-tool") return "#2563eb";
  if (node.data?.kind === "plugin-binding") return "#d97706";
  if (node.data?.kind === "erp-api") return "#059669";
  if (node.data?.kind === "ambiguous-endpoint") return "#f97316";
  if (node.data?.kind === "unresolved-endpoint") return "#dc2626";
  return "#64748b";
}

const fitViewOptions = { duration: 200, maxZoom: 1.2, padding: 0.2 };

export function TopologyCanvas({
  nodes,
  edges,
  selection,
  onSelectNode,
  onSelectEdge,
  onClearSelection,
  compact = false,
  nodeSummaries = {},
}: {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  selection: TopologySelection;
  onSelectNode: (id: string) => void;
  onSelectEdge: (id: string) => void;
  onClearSelection: () => void;
  compact?: boolean;
  nodeSummaries?: Record<string, string>;
}) {
  const topologyFlow = useMemo(
    () => buildTopologyFlow(nodes, edges, selection, compact, nodeSummaries),
    [compact, edges, nodeSummaries, nodes, selection],
  );
  const flowNodes = useMemo<Node<FlowNodeData>[]>(
    () =>
      topologyFlow.nodes.map((node) => ({
        ...node,
        data: {
          ...node.data,
          onSelect: () => onSelectNode(node.id),
        },
      })),
    [onSelectNode, topologyFlow.nodes],
  );
  const nodeKey = useMemo(
    () => nodes.map((node) => node.id).join("|"),
    [nodes],
  );
  const flowEdges = topologyFlow.edges;
  const handleEdgeClick = useCallback(
    (_event: MouseEvent, edge: Edge) => onSelectEdge(edge.id),
    [onSelectEdge],
  );
  const handleNodeClick = useCallback(
    (_event: MouseEvent, node: Node) => onSelectNode(node.id),
    [onSelectNode],
  );

  return (
    <div className="space-y-2">
      <div
        aria-label="Interactive API to MCP topology"
        className="h-[28rem] overflow-hidden rounded-lg border border-border bg-muted/30 sm:h-[36rem]"
        role="application"
      >
        <ReactFlow
          edges={flowEdges}
          key={nodeKey}
          edgesFocusable
          elevateEdgesOnSelect
          fitView
          fitViewOptions={fitViewOptions}
          maxZoom={1.5}
          minZoom={0.1}
          nodeTypes={nodeTypes}
          nodes={flowNodes}
          nodesFocusable
          onEdgeClick={handleEdgeClick}
          onNodeClick={handleNodeClick}
          onPaneClick={onClearSelection}
        >
          <Background />
          <Controls />
          <MiniMap nodeColor={miniMapNodeColor} pannable zoomable />
          <Panel
            className="rounded-md border border-border bg-card/90 px-2 py-1 text-xs text-muted-foreground shadow-sm"
            position="top-left"
          >
            {compact
              ? "Select a component below to show its connected MCP graph."
              : "Click a node or relationship to inspect it. Press Escape to clear."}
          </Panel>
        </ReactFlow>
      </div>
      <div
        aria-label="Topology legend"
        className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground"
      >
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-sky-500/60 bg-sky-500/20" />
          Transport
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-primary/60 bg-primary/20" />
          MCP tool
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-emerald-500/60 bg-emerald-500/20" />
          ERP API
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-orange-500/60 bg-orange-500/20" />
          Ambiguous
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-destructive/60 bg-destructive/20" />
          Unresolved
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-amber-500/60 bg-amber-500/20" />
          Binding
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-violet-500/60 bg-violet-500/20" />
          Plugin
        </span>
        <span>
          {compact
            ? "Components are collapsed until selected."
            : "Edge labels show match confidence."}
        </span>
      </div>
    </div>
  );
}
