import { useCallback, useMemo, type MouseEvent } from "react";

import {
  Background,
  Controls,
  Handle,
  MarkerType,
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

type FlowNodeData = {
  label: string;
  kind: string;
  summary?: string;
  compact: boolean;
  selected: boolean;
  dimmed: boolean;
  onSelect: () => void;
};

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
              : kind === "unresolved-endpoint"
                ? "border-destructive/60 bg-destructive/10"
                : "border-border bg-card";
  return [
    `relative ${compact ? "w-56 min-w-56 max-w-56" : "w-40 min-w-40 max-w-40"} rounded-md border px-3 py-2 text-card-foreground shadow-sm outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring`,
    colorClass,
    selected ? "z-20 ring-2 ring-primary shadow-lg" : "",
    dimmed ? "opacity-25" : "opacity-100",
  ].join(" ");
}

function TopologyFlowNode({ data }: { data: FlowNodeData }) {
  return (
    <div
      aria-label={`${data.label}, ${data.kind}`}
      aria-pressed={data.selected}
      className={nodeClass(data.kind, data.compact, data.selected, data.dimmed)}
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
      <Handle position={Position.Top} type="target" />
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
      <Handle position={Position.Bottom} type="source" />
    </div>
  );
}

const nodeTypes = { topology: TopologyFlowNode };
const toolColumns = 6;
const columnGap = 190;
const rowGap = 100;

type TopologyFlowNode = Node<FlowNodeData>;

function groupNodes(nodes: TopologyNode[], kind: string) {
  return nodes.filter((node) => node.kind === kind);
}

function selectedConnectedIDs(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
  selection: TopologySelection,
) {
  if (!selection) return new Set<string>();
  if (selection.kind === "node") {
    const connected = new Set([selection.id]);
    for (const edge of edges) {
      if (edge.source === selection.id) connected.add(edge.target);
      if (edge.target === selection.id) connected.add(edge.source);
    }
    return connected;
  }
  const edge = edges.find((candidate) => candidate.id === selection.id);
  return edge
    ? new Set([edge.source, edge.target])
    : new Set(nodes.map((node) => node.id));
}

function layoutTopologyNodes(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
  selection: TopologySelection,
  compact: boolean,
  summaries: Record<string, string>,
): TopologyFlowNode[] {
  const transports = groupNodes(nodes, "mcp-transport");
  const tools = groupNodes(nodes, "mcp-tool");
  const bindings = groupNodes(nodes, "plugin-binding");
  const plugins = groupNodes(nodes, "external-plugin");
  const apis = nodes.filter(
    (node) => node.kind === "erp-api" || node.kind === "unresolved-endpoint",
  );
  const connectedIDs = selectedConnectedIDs(nodes, edges, selection);
  const toolRows = Math.max(1, Math.ceil(tools.length / toolColumns));
  const rightRows = Math.max(1, bindings.length, plugins.length, apis.length);
  const centerY = ((Math.max(toolRows, rightRows) - 1) * rowGap) / 2;
  const rightX = 280 + toolColumns * columnGap;
  const pluginX = rightX + 220;
  const apiX = pluginX + 220;

  const place = (
    node: TopologyNode,
    position: { x: number; y: number },
  ): TopologyFlowNode => ({
    id: node.id,
    position,
    data: {
      label: node.label,
      kind: node.kind,
      summary: summaries[node.id],
      compact,
      selected: selection?.kind === "node" && selection.id === node.id,
      dimmed: Boolean(selection) && !connectedIDs.has(node.id),
      onSelect: () => undefined,
    },
    type: "topology",
  });

  if (compact) {
    return apis.map((node, index) =>
      place(node, {
        x: (index % 3) * 280,
        y: Math.floor(index / 3) * 150,
      }),
    );
  }

  return [
    ...transports.map((node, index) =>
      place(node, {
        x: 0,
        y: centerY + (index - (transports.length - 1) / 2) * rowGap,
      }),
    ),
    ...tools.map((node, index) =>
      place(node, {
        x: 280 + (index % toolColumns) * columnGap,
        y: Math.floor(index / toolColumns) * rowGap,
      }),
    ),
    ...bindings.map((node, index) =>
      place(node, {
        x: rightX,
        y: centerY + (index - (bindings.length - 1) / 2) * rowGap,
      }),
    ),
    ...plugins.map((node, index) =>
      place(node, {
        x: pluginX,
        y: centerY + (index - (plugins.length - 1) / 2) * rowGap,
      }),
    ),
    ...apis.map((node, index) =>
      place(node, {
        x: apiX,
        y: centerY + (index - (apis.length - 1) / 2) * rowGap,
      }),
    ),
  ];
}

function edgeColor(matchKind: string) {
  if (matchKind === "unresolved") return "hsl(var(--destructive))";
  if (matchKind === "ambiguous") return "hsl(38 92% 50%)";
  if (matchKind === "base-prefix") return "hsl(221 83% 53%)";
  return "hsl(var(--primary))";
}

function miniMapNodeColor(node: Node<FlowNodeData>) {
  if (node.data?.kind === "external-plugin") return "#8b5cf6";
  if (node.data?.kind === "mcp-tool") return "#2563eb";
  if (node.data?.kind === "plugin-binding") return "#d97706";
  if (node.data?.kind === "erp-api") return "#059669";
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
  const flowNodes = useMemo<Node<FlowNodeData>[]>(
    () =>
      layoutTopologyNodes(nodes, edges, selection, compact, nodeSummaries).map(
        (node) => ({
          ...node,
          data: {
            ...node.data,
            onSelect: () => onSelectNode(node.id),
          },
        }),
      ),
    [compact, edges, nodeSummaries, nodes, onSelectNode, selection],
  );
  const nodeKey = useMemo(
    () => nodes.map((node) => node.id).join("|"),
    [nodes],
  );
  const flowEdges = useMemo<Edge[]>(
    () =>
      edges.map((edge) => {
        const selected = selection?.kind === "edge" && selection.id === edge.id;
        const connected =
          !selection ||
          selected ||
          (selection.kind === "node" &&
            (edge.source === selection.id || edge.target === selection.id));
        const color = edgeColor(edge.matchKind);
        return {
          id: edge.id,
          source: edge.source,
          target: edge.target,
          animated: edge.matchKind === "base-prefix",
          selected,
          label: edge.matchKind,
          labelStyle: { fontSize: 10, fontWeight: selected ? 700 : 500 },
          labelBgStyle: { fill: "hsl(var(--card))", fillOpacity: 0.9 },
          labelBgPadding: [4, 2],
          labelBgBorderRadius: 4,
          markerEnd: { type: MarkerType.ArrowClosed, color },
          style: {
            stroke: color,
            strokeWidth: selected ? 3 : 1.5,
            opacity: connected ? 1 : 0.2,
          },
          zIndex: selected ? 10 : 0,
        };
      }),
    [edges, selection],
  );
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
              ? "Select an endpoint component to show related MCP nodes."
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
          <span className="h-2.5 w-2.5 rounded-sm border border-amber-500/60 bg-amber-500/20" />
          Binding
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-violet-500/60 bg-violet-500/20" />
          Plugin
        </span>
        <span>
          {compact
            ? "Endpoint components are collapsed until selected."
            : "Edge labels show match confidence."}
        </span>
      </div>
    </div>
  );
}
