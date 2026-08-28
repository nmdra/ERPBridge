import type { TopologyEdge, TopologyNode } from "../hooks/useTopology";

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

const endpointKinds = new Set([
  "erp-api",
  "ambiguous-endpoint",
  "unresolved-endpoint",
]);

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

export function canDrillIntoNode(node: TopologyNode | undefined) {
  return Boolean(node && (node.kind === "mcp-tool" || isEndpointNode(node)));
}

function componentForEndpoint(
  endpoint: TopologyNode,
  edges: TopologyEdge[],
  byID: Map<string, TopologyNode>,
) {
  const memberNodeIds = new Set([endpoint.id]);
  const toolIDs = new Set(
    edges.flatMap((edge) => {
      if (
        edge.source === endpoint.id &&
        byID.get(edge.target)?.kind === "mcp-tool"
      ) {
        return [edge.target];
      }
      if (
        edge.target === endpoint.id &&
        byID.get(edge.source)?.kind === "mcp-tool"
      ) {
        return [edge.source];
      }
      return [];
    }),
  );
  const visitedBindings = new Set<string>();

  const includeBinding = (bindingID: string) => {
    if (visitedBindings.has(bindingID)) return;
    visitedBindings.add(bindingID);
    memberNodeIds.add(bindingID);
    for (const edge of edges) {
      if (edge.source !== bindingID && edge.target !== bindingID) continue;
      const otherID = edge.source === bindingID ? edge.target : edge.source;
      if (byID.get(otherID)?.kind === "external-plugin") {
        memberNodeIds.add(otherID);
      }
    }
  };

  for (const toolID of toolIDs) {
    memberNodeIds.add(toolID);
    for (const edge of edges) {
      if (edge.source !== toolID && edge.target !== toolID) continue;
      const otherID = edge.source === toolID ? edge.target : edge.source;
      const other = byID.get(otherID);
      if (other?.kind === "mcp-transport") memberNodeIds.add(otherID);
      if (other?.kind === "plugin-binding") includeBinding(otherID);
      if (other?.kind === "external-plugin") memberNodeIds.add(otherID);
    }
  }

  return memberNodeIds;
}

function toComponent(
  endpoint: TopologyNode,
  memberNodeIds: Set<string>,
  edges: TopologyEdge[],
  byID: Map<string, TopologyNode>,
): EndpointComponent {
  const memberEdges = edges.filter(
    (edge) => memberNodeIds.has(edge.source) && memberNodeIds.has(edge.target),
  );
  const matchCounts: TopologyMatchHistogram = {};
  for (const kind of topologyMatchKinds) matchCounts[kind] = 0;
  for (const edge of memberEdges) {
    if (edge.target !== endpoint.id && edge.source !== endpoint.id) continue;
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
  };
}

export function shouldUseCompactTopology(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
) {
  return buildEndpointComponents(nodes, edges).length > 0;
}

export function buildEndpointComponents(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
): EndpointComponent[] {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const endpointNodes = nodes.filter(isEndpointNode);
  const endpointComponents = endpointNodes.map((endpoint) =>
    toComponent(
      endpoint,
      componentForEndpoint(endpoint, edges, byID),
      edges,
      byID,
    ),
  );
  const assignedToolIDs = new Set(
    endpointComponents.flatMap((component) =>
      component.memberNodeIds.filter((id) => byID.get(id)?.kind === "mcp-tool"),
    ),
  );
  const standaloneToolComponents = nodes
    .filter((node) => node.kind === "mcp-tool" && !assignedToolIDs.has(node.id))
    .map((tool) =>
      toComponent(tool, componentForEndpoint(tool, edges, byID), edges, byID),
    );

  return [...endpointComponents, ...standaloneToolComponents].sort(
    (left, right) => {
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
    },
  );
}

export function componentForNode(
  components: EndpointComponent[],
  nodeID: string,
) {
  return components.find((component) =>
    component.memberNodeIds.includes(nodeID),
  );
}

export function componentSummary(component: EndpointComponent) {
  const matches = topologyMatchKinds
    .filter((kind) => component.matchCounts[kind] > 0)
    .map((kind) => `${component.matchCounts[kind]} ${kind}`)
    .join(" · ");
  return `${component.toolCount} MCP tool${component.toolCount === 1 ? "" : "s"}${matches ? ` · ${matches}` : ""}`;
}

export function buildCompactTopologyOverview(
  components: EndpointComponent[],
  nodes: TopologyNode[],
) {
  const transports = nodes.filter((node) => node.kind === "mcp-transport");
  const componentIDs = new Set(
    components.map((component) => component.endpoint.id),
  );
  const componentNodes = nodes.filter((node) => componentIDs.has(node.id));
  const componentOrder = new Map(
    componentNodes.map((node, index) => [node.id, index]),
  );
  const transportIDs = new Set(transports.map((node) => node.id));
  const edges: TopologyEdge[] = [];

  for (const component of [...components].sort(
    (left, right) =>
      (componentOrder.get(left.endpoint.id) ?? 0) -
      (componentOrder.get(right.endpoint.id) ?? 0),
  )) {
    for (const transportID of component.memberNodeIds) {
      if (!transportIDs.has(transportID)) continue;
      edges.push({
        id: `summary:${transportID}:${component.endpoint.id}`,
        source: transportID,
        target: component.endpoint.id,
        matchKind: "summary",
        authoritative: false,
      });
    }
  }

  return { nodes: [...transports, ...componentNodes], edges };
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
