import type { TopologyEdge, TopologyNode } from "../hooks/useTopology";

export const compactTopologyNodeThreshold = 20;
export const compactTopologyEdgeThreshold = 30;
export const compactTopologyComponentLimit = 24;

export const topologyMatchKinds = [
  "exact",
  "base-prefix",
  "ambiguous",
  "unresolved",
] as const;

export type TopologyMatchHistogram = Record<string, number>;

export type EndpointComponent = {
  endpoint: TopologyNode;
  memberNodeIds: string[];
  memberEdges: TopologyEdge[];
  toolCount: number;
  matchCounts: TopologyMatchHistogram;
};

const endpointKinds = new Set(["erp-api", "unresolved-endpoint"]);
const componentNodeKinds = new Set([
  "mcp-transport",
  "mcp-tool",
  "plugin-binding",
  "external-plugin",
]);

export function shouldUseCompactTopology(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
) {
  return (
    nodes.length >= compactTopologyNodeThreshold ||
    edges.length >= compactTopologyEdgeThreshold
  );
}

function matchPriority(matchKind: string) {
  switch (matchKind) {
    case "unresolved":
      return 0;
    case "ambiguous":
      return 1;
    case "base-prefix":
      return 2;
    case "exact":
      return 3;
    default:
      return 4;
  }
}

function isEndpointNode(node: TopologyNode | undefined) {
  return Boolean(node && endpointKinds.has(node.kind));
}

function relatedComponentNode(
  node: TopologyNode | undefined,
  endpointID: string,
) {
  return Boolean(
    node &&
      node.id !== endpointID &&
      (componentNodeKinds.has(node.kind) ||
        node.kind === "unresolved-endpoint"),
  );
}

export function buildEndpointComponents(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
): EndpointComponent[] {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const endpointNodes = nodes.filter(isEndpointNode);

  return endpointNodes
    .map((endpoint) => {
      const memberNodeIds = new Set([endpoint.id]);
      const queue = edges
        .filter(
          (edge) =>
            edge.target === endpoint.id &&
            byID.get(edge.source)?.kind === "mcp-tool",
        )
        .map((edge) => edge.source);

      while (queue.length) {
        const nodeID = queue.shift();
        if (!nodeID || memberNodeIds.has(nodeID)) continue;
        const node = byID.get(nodeID);
        if (!relatedComponentNode(node, endpoint.id)) continue;
        memberNodeIds.add(nodeID);

        for (const edge of edges) {
          if (edge.source !== nodeID && edge.target !== nodeID) continue;
          const otherID = edge.source === nodeID ? edge.target : edge.source;
          const other = byID.get(otherID);
          if (relatedComponentNode(other, endpoint.id)) queue.push(otherID);
        }
      }

      const memberEdges = edges.filter(
        (edge) =>
          memberNodeIds.has(edge.source) && memberNodeIds.has(edge.target),
      );
      const matchCounts: TopologyMatchHistogram = {};
      for (const kind of topologyMatchKinds) matchCounts[kind] = 0;
      for (const edge of memberEdges) {
        if (edge.target !== endpoint.id) continue;
        matchCounts[edge.matchKind] = (matchCounts[edge.matchKind] ?? 0) + 1;
      }

      return {
        endpoint,
        memberNodeIds: [...memberNodeIds],
        memberEdges,
        toolCount: [...memberNodeIds].filter(
          (id) => byID.get(id)?.kind === "mcp-tool",
        ).length,
        matchCounts,
      } satisfies EndpointComponent;
    })
    .sort((left, right) => {
      const leftPriority =
        left.endpoint.kind === "unresolved-endpoint"
          ? -1
          : Math.min(
              ...Object.entries(left.matchCounts)
                .filter(([, count]) => count > 0)
                .map(([kind]) => matchPriority(kind)),
              4,
            );
      const rightPriority =
        right.endpoint.kind === "unresolved-endpoint"
          ? -1
          : Math.min(
              ...Object.entries(right.matchCounts)
                .filter(([, count]) => count > 0)
                .map(([kind]) => matchPriority(kind)),
              4,
            );
      return (
        leftPriority - rightPriority ||
        left.endpoint.label.localeCompare(right.endpoint.label)
      );
    });
}

export function componentSummary(component: EndpointComponent) {
  const matches = topologyMatchKinds
    .filter((kind) => component.matchCounts[kind] > 0)
    .map((kind) => `${component.matchCounts[kind]} ${kind}`)
    .join(" · ");
  return `${component.toolCount} MCP tool${component.toolCount === 1 ? "" : "s"}${matches ? ` · ${matches}` : ""}`;
}

export function focusedComponentGraph(
  component: EndpointComponent,
  nodes: TopologyNode[],
  edges: TopologyEdge[],
) {
  const memberIDs = new Set(component.memberNodeIds);
  return {
    nodes: nodes.filter((node) => memberIDs.has(node.id)),
    edges: edges.filter(
      (edge) => memberIDs.has(edge.source) && memberIDs.has(edge.target),
    ),
  };
}
