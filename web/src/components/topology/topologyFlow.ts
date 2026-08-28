import { MarkerType, type Edge, type Node } from "@xyflow/react";

import type {
  TopologyEdge,
  TopologyNode,
  TopologySelection,
} from "../../hooks/useTopology";

export type TopologyShape =
  "pill" | "rectangle" | "database" | "diamond" | "hexagon" | "tag" | "rounded";

type TopologyNodePresentation = {
  label: string;
  shape: TopologyShape;
  shapeClass: string;
};

export type FlowNodeData = {
  label: string;
  kind: string;
  presentationLabel: string;
  shape: TopologyShape;
  shapeClass: string;
  summary?: string;
  compact: boolean;
  selected: boolean;
  dimmed: boolean;
  onSelect: () => void;
};

const nodePresentations: Record<string, TopologyNodePresentation> = {
  "mcp-transport": {
    label: "MCP transport",
    shape: "pill",
    shapeClass: "rounded-full",
  },
  "mcp-tool": {
    label: "MCP tool",
    shape: "rectangle",
    shapeClass: "rounded-sm",
  },
  "erp-api": {
    label: "ERP API",
    shape: "database",
    shapeClass: "rounded-[45%] border-2",
  },
  "ambiguous-endpoint": {
    label: "Ambiguous endpoint",
    shape: "diamond",
    shapeClass:
      "rounded-none [clip-path:polygon(50%_0%,100%_50%,50%_100%,0%_50%)]",
  },
  "unresolved-endpoint": {
    label: "Unresolved endpoint",
    shape: "hexagon",
    shapeClass:
      "rounded-none [clip-path:polygon(25%_0%,75%_0%,100%_50%,75%_100%,25%_100%,0%_50%)]",
  },
  "plugin-binding": {
    label: "Plugin binding",
    shape: "tag",
    shapeClass: "rounded-bl-xl rounded-br-sm rounded-tl-sm rounded-tr-xl",
  },
  "external-plugin": {
    label: "External plugin",
    shape: "rounded",
    shapeClass: "rounded-2xl",
  },
};

const defaultNodePresentation: TopologyNodePresentation = {
  label: "Topology component",
  shape: "rectangle",
  shapeClass: "rounded-sm",
};

export function topologyNodePresentation(
  kind: string,
): TopologyNodePresentation {
  return nodePresentations[kind] ?? defaultNodePresentation;
}

export function handleIDs(kind: string) {
  return {
    source: `source-${kind}`,
    target: `target-${kind}`,
  };
}

const toolColumns = 6;
const columnGap = 190;
const rowGap = 100;

type FlowTopologyNode = Node<FlowNodeData>;

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
): FlowTopologyNode[] {
  const transports = groupNodes(nodes, "mcp-transport");
  const tools = groupNodes(nodes, "mcp-tool");
  const bindings = groupNodes(nodes, "plugin-binding");
  const plugins = groupNodes(nodes, "external-plugin");
  const apis = nodes.filter(
    (node) =>
      node.kind === "erp-api" ||
      node.kind === "ambiguous-endpoint" ||
      node.kind === "unresolved-endpoint",
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
  ): FlowTopologyNode => ({
    id: node.id,
    position,
    data: {
      label: node.label,
      kind: node.kind,
      presentationLabel: topologyNodePresentation(node.kind).label,
      shape: topologyNodePresentation(node.kind).shape,
      shapeClass: topologyNodePresentation(node.kind).shapeClass,
      summary: summaries[node.id],
      compact,
      selected: selection?.kind === "node" && selection.id === node.id,
      dimmed: Boolean(selection) && !connectedIDs.has(node.id),
      onSelect: () => undefined,
    },
    type: "topology",
  });

  if (compact) {
    const componentNodes = nodes.filter(
      (node) => node.kind !== "mcp-transport",
    );
    return [
      ...transports.map((node, index) =>
        place(node, {
          x: 0,
          y: centerY + (index - (transports.length - 1) / 2) * rowGap,
        }),
      ),
      ...componentNodes.map((node, index) =>
        place(node, {
          x: 280 + (index % 3) * 280,
          y: Math.floor(index / 3) * 150,
        }),
      ),
    ];
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

export function buildTopologyFlow(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
  selection: TopologySelection,
  compact: boolean,
  summaries: Record<string, string>,
): { nodes: FlowTopologyNode[]; edges: Edge[] } {
  const labels = new Map(nodes.map((node) => [node.id, node.label]));
  const flowNodes = layoutTopologyNodes(
    nodes,
    edges,
    selection,
    compact,
    summaries,
  );
  const flowEdges = edges.map((edge) => {
    const selected = selection?.kind === "edge" && selection.id === edge.id;
    const connected =
      !selection ||
      selected ||
      (selection.kind === "node" &&
        (edge.source === selection.id || edge.target === selection.id));
    const color = edgeColor(edge.matchKind);
    const source = nodes.find((node) => node.id === edge.source);
    const target = nodes.find((node) => node.id === edge.target);

    return {
      id: edge.id,
      source: edge.source,
      sourceHandle: handleIDs(source?.kind ?? "topology-component").source,
      target: edge.target,
      targetHandle: handleIDs(target?.kind ?? "topology-component").target,
      animated: edge.matchKind === "base-prefix",
      ariaLabel: `${labels.get(edge.source) ?? edge.source} to ${labels.get(edge.target) ?? edge.target}: ${edge.matchKind}${edge.authoritative ? " authoritative" : ""}`,
      selected,
      label: edge.matchKind,
      labelStyle: { fontSize: 10, fontWeight: selected ? 700 : 500 },
      labelBgStyle: { fill: "hsl(var(--card))", fillOpacity: 0.9 },
      labelBgPadding: [4, 2] as [number, number],
      labelBgBorderRadius: 4,
      markerEnd: { type: MarkerType.ArrowClosed, color },
      style: {
        stroke: color,
        strokeWidth: selected ? 3 : 1.5,
        opacity: connected ? 1 : 0.2,
      },
      zIndex: selected ? 10 : 0,
    };
  });

  return { nodes: flowNodes, edges: flowEdges };
}
