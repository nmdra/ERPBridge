import {
  Background,
  Controls,
  Handle,
  MiniMap,
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

import type { TopologyNode } from "../../hooks/useTopology";

type FlowNodeData = {
  label: string;
  kind: string;
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

function nodeClass(kind: string) {
  if (kind === "erp-api") return "border-emerald-500/60 bg-emerald-500/10";
  if (kind === "mcp-tool") return "border-primary/60 bg-primary/10";
  if (kind === "mcp-transport") return "border-sky-500/60 bg-sky-500/10";
  if (kind === "external-plugin")
    return "border-violet-500/60 bg-violet-500/10";
  if (kind === "plugin-binding") return "border-amber-500/60 bg-amber-500/10";
  if (kind === "unresolved-endpoint") {
    return "border-destructive/60 bg-destructive/10";
  }
  return "border-border bg-card";
}

function TopologyFlowNode({ data }: { data: FlowNodeData }) {
  return (
    <div
      aria-label={`${data.label}, ${data.kind}`}
      className={`relative w-40 min-w-40 max-w-40 rounded-md border px-3 py-2 text-card-foreground shadow-sm outline-none focus-visible:ring-2 focus-visible:ring-ring ${nodeClass(data.kind)}`}
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
        <span className="min-w-0 truncate" title={data.label}>
          {data.label}
        </span>
      </div>
      <span className="mt-1 block text-[10px] uppercase tracking-wider text-muted-foreground">
        {data.kind}
      </span>
      <Handle position={Position.Bottom} type="source" />
    </div>
  );
}

const nodeTypes = { topology: TopologyFlowNode };
const toolColumns = 8;
const columnGap = 190;
const rowGap = 100;

type TopologyFlowNode = Node<FlowNodeData>;

function groupNodes(nodes: TopologyNode[], kind: string) {
  return nodes.filter((node) => node.kind === kind);
}

function layoutTopologyNodes(nodes: TopologyNode[]): TopologyFlowNode[] {
  const transports = groupNodes(nodes, "mcp-transport");
  const tools = groupNodes(nodes, "mcp-tool");
  const bindings = groupNodes(nodes, "plugin-binding");
  const plugins = groupNodes(nodes, "external-plugin");
  const apis = nodes.filter(
    (node) => node.kind === "erp-api" || node.kind === "unresolved-endpoint",
  );
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
      onSelect: () => undefined,
    },
    type: "topology",
  });

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

export function TopologyCanvas({
  nodes,
  edges,
  onSelect,
}: {
  nodes: TopologyNode[];
  edges: Array<{
    id: string;
    source: string;
    target: string;
    matchKind: string;
  }>;
  onSelect: (node: TopologyNode) => void;
}) {
  const flowNodes: Node<FlowNodeData>[] = layoutTopologyNodes(nodes).map(
    (node) => ({
      ...node,
      data: {
        ...node.data,
        onSelect: () => {
          const selected = nodes.find((candidate) => candidate.id === node.id);
          if (selected) onSelect(selected);
        },
      },
    }),
  );
  const nodeKey = nodes.map((node) => node.id).join("|");
  const transportCount = nodes.filter(
    (node) => node.kind === "mcp-transport",
  ).length;
  const toolCount = nodes.filter((node) => node.kind === "mcp-tool").length;
  const apiCount = nodes.filter((node) => node.kind === "erp-api").length;
  const pluginCount = nodes.filter(
    (node) => node.kind === "external-plugin",
  ).length;
  const bindingCount = nodes.filter(
    (node) => node.kind === "plugin-binding",
  ).length;
  const flowEdges: Edge[] = edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    animated: edge.matchKind === "base-prefix",
    style: {
      stroke:
        edge.matchKind === "unresolved"
          ? "hsl(var(--destructive))"
          : "hsl(var(--primary))",
    },
  }));
  return (
    <div className="space-y-2">
      <div
        aria-label="Interactive API to MCP topology"
        className="h-[36rem] overflow-hidden rounded-lg border border-border bg-muted/30"
      >
        <ReactFlow
          edges={flowEdges}
          fitView={false}
          key={nodeKey}
          minZoom={0.1}
          nodeTypes={nodeTypes}
          nodes={flowNodes}
          onInit={(instance) => {
            window.setTimeout(() => {
              instance.fitView({ padding: 0.2, duration: 0 });
            }, 500);
          }}
        >
          <Background />
          <Controls />
          <MiniMap />
        </ReactFlow>
      </div>
      <div
        aria-label="Topology legend"
        className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground"
      >
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-sky-500/60 bg-sky-500/20" />
          MCP transport ({transportCount})
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-primary/60 bg-primary/20" />
          MCP tools ({toolCount})
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-emerald-500/60 bg-emerald-500/20" />
          ERP APIs ({apiCount})
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-amber-500/60 bg-amber-500/20" />
          Bindings ({bindingCount})
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-sm border border-violet-500/60 bg-violet-500/20" />
          Plugins ({pluginCount})
        </span>
        <span>Animated edges indicate inferred base-prefix matches.</span>
      </div>
    </div>
  );
}
