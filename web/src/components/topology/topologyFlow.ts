import { MarkerType, type Edge, type Node } from "@xyflow/react";

import type {
  TopologyVisualEdge,
  TopologyVisualGraph,
  TopologyVisualNode,
  TopologyVisualNodeKind,
} from "../../routes/topologyViewModel";

export type TopologyTone =
  "neutral" | "success" | "info" | "warning" | "danger";

export type TopologyNodePresentation = {
  label: string;
  structuralRole: "transport" | "entity" | "component" | "cluster";
  tone: TopologyTone;
};

export type TopologyFlowNodeData = {
  label: string;
  viewKind: TopologyVisualNodeKind;
  topologyKind?: string;
  presentationLabel: string;
  summary?: string;
  count?: number;
  originalNodeId?: string;
  componentId?: string;
  memberNodeIds?: string[];
  tone: TopologyTone;
  selected: boolean;
  dimmed: boolean;
  incomingPorts?: string[];
  outgoingPorts?: string[];
  onSelect: () => void;
  onExpand?: () => void;
};

export type TopologyFlowNode = Node<TopologyFlowNodeData>;

export type TopologyFlowEdgeData = {
  matchKind: string;
  authoritative: boolean;
  originalEdgeIds: string[];
  dimmed: boolean;
};

export type TopologyFlowEdge = Edge<TopologyFlowEdgeData>;

export type TopologyFlowHandlers = {
  onSelectNode?: (node: TopologyVisualNode) => void;
  onExpandCluster?: (node: TopologyVisualNode) => void;
};

const nodePresentations: Record<string, TopologyNodePresentation> = {
  "mcp-transport": {
    label: "MCP transport",
    structuralRole: "transport",
    tone: "neutral",
  },
  "mcp-tool": {
    label: "MCP tool",
    structuralRole: "entity",
    tone: "neutral",
  },
  "erp-api": {
    label: "ERP API",
    structuralRole: "entity",
    tone: "neutral",
  },
  "erp-endpoint": {
    label: "ERP endpoint",
    structuralRole: "entity",
    tone: "neutral",
  },
  "ambiguous-endpoint": {
    label: "Ambiguous endpoint",
    structuralRole: "entity",
    tone: "warning",
  },
  "unresolved-endpoint": {
    label: "Unresolved endpoint",
    structuralRole: "entity",
    tone: "danger",
  },
  "plugin-binding": {
    label: "Plugin binding",
    structuralRole: "entity",
    tone: "neutral",
  },
  "external-plugin": {
    label: "External plugin",
    structuralRole: "entity",
    tone: "neutral",
  },
  cluster: {
    label: "Collapsed group",
    structuralRole: "cluster",
    tone: "info",
  },
  component: {
    label: "Topology component",
    structuralRole: "component",
    tone: "neutral",
  },
  transport: {
    label: "MCP transport",
    structuralRole: "transport",
    tone: "neutral",
  },
  entity: {
    label: "Topology entity",
    structuralRole: "entity",
    tone: "neutral",
  },
};

const defaultNodePresentation: TopologyNodePresentation = {
  label: "Topology component",
  structuralRole: "entity",
  tone: "neutral",
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

function toneForNode(node: TopologyVisualNode): TopologyTone {
  if (node.kind === "cluster") return "info";
  return topologyNodePresentation(node.topologyKind ?? node.kind).tone;
}

function labelForNode(node: TopologyVisualNode) {
  return node.kind === "cluster"
    ? node.label
    : topologyNodePresentation(node.topologyKind ?? node.kind).label;
}

const MULTI_PORT_THRESHOLD = 5;

type PortMaps = {
  incoming: Map<string, string[]>;
  outgoing: Map<string, string[]>;
};

function buildPortMaps(edges: TopologyVisualEdge[]): PortMaps {
  const incoming = new Map<string, string[]>();
  const outgoing = new Map<string, string[]>();
  for (const edge of edges) {
    const inputPorts = incoming.get(edge.target) ?? [];
    inputPorts.push(`target-port-${edge.id}`);
    incoming.set(edge.target, inputPorts);
    const outputPorts = outgoing.get(edge.source) ?? [];
    outputPorts.push(`source-port-${edge.id}`);
    outgoing.set(edge.source, outputPorts);
  }
  return {
    incoming: new Map(
      [...incoming].filter(([, ports]) => ports.length > MULTI_PORT_THRESHOLD),
    ),
    outgoing: new Map(
      [...outgoing].filter(([, ports]) => ports.length > MULTI_PORT_THRESHOLD),
    ),
  };
}

function toFlowNode(
  node: TopologyVisualNode,
  handlers: TopologyFlowHandlers,
  portMaps: PortMaps,
): TopologyFlowNode {
  const topologyKind = node.node?.kind ?? node.topologyKind;
  return {
    id: node.id,
    position: { x: 0, y: 0 },
    type: node.kind,
    data: {
      label: node.label,
      viewKind: node.kind,
      topologyKind,
      presentationLabel: labelForNode(node),
      summary: node.summary,
      count: node.count,
      originalNodeId: node.originalNodeId,
      componentId: node.componentId,
      memberNodeIds: node.memberNodeIds,
      tone: toneForNode(node),
      selected: node.selected,
      dimmed: node.dimmed,
      incomingPorts: portMaps.incoming.get(node.id),
      outgoingPorts: portMaps.outgoing.get(node.id),
      onSelect: () => handlers.onSelectNode?.(node),
      onExpand:
        node.kind === "cluster"
          ? () => handlers.onExpandCluster?.(node)
          : undefined,
    },
  };
}

function edgeColor(matchKind: string) {
  if (matchKind === "unresolved") return "hsl(var(--destructive))";
  if (matchKind === "ambiguous") return "hsl(var(--warning))";
  if (matchKind === "base-prefix") return "hsl(var(--info))";
  return "hsl(var(--muted-foreground))";
}

function toFlowEdge(
  edge: TopologyVisualEdge,
  byID: Map<string, TopologyVisualNode>,
  portMaps: PortMaps,
): TopologyFlowEdge {
  const source = byID.get(edge.source);
  const target = byID.get(edge.target);
  const sourceHandle = portMaps.outgoing.has(edge.source)
    ? `source-port-${edge.id}`
    : handleIDs(source?.kind ?? "entity").source;
  const targetHandle = portMaps.incoming.has(edge.target)
    ? `target-port-${edge.id}`
    : handleIDs(target?.kind ?? "entity").target;
  const sourceLabel = source?.label ?? edge.source;
  const targetLabel = target?.label ?? edge.target;
  const color = edgeColor(edge.matchKind);
  return {
    id: edge.id,
    source: edge.source,
    sourceHandle,
    target: edge.target,
    targetHandle,
    type: "topology",
    ariaLabel: `${sourceLabel} to ${targetLabel}: ${edge.matchKind}${edge.authoritative ? " authoritative" : " inferred or unresolved"}`,
    selected: edge.selected,
    markerEnd:
      edge.matchKind === "summary"
        ? undefined
        : { type: MarkerType.ArrowClosed, color },
    data: {
      matchKind: edge.matchKind,
      authoritative: edge.authoritative,
      originalEdgeIds: edge.originalEdgeIds,
      dimmed: edge.dimmed,
    },
    zIndex: edge.selected ? 10 : 0,
  };
}

export function buildTopologyFlow(
  graph: TopologyVisualGraph,
  handlers: TopologyFlowHandlers = {},
): { nodes: TopologyFlowNode[]; edges: TopologyFlowEdge[] } {
  const byID = new Map(graph.nodes.map((node) => [node.id, node]));
  const portMaps = buildPortMaps(graph.edges);
  return {
    nodes: graph.nodes.map((node) => toFlowNode(node, handlers, portMaps)),
    edges: graph.edges.map((edge) => toFlowEdge(edge, byID, portMaps)),
  };
}
