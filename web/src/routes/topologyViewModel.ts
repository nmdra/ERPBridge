import type {
  TopologyEdge,
  TopologyNode,
  TopologySelection,
} from "../hooks/useTopology";
import {
  type EndpointComponent,
  buildEndpointComponents,
  componentSummary,
  compactTopologyComponentLimit,
} from "./topologyPresentation";

export type TopologyViewMode = "overview" | "focused" | "expanded";
export type TopologyVisualNodeKind =
  "transport" | "component" | "entity" | "cluster";

export const INDIVIDUAL_NODE_LIMIT = 10;

export type TopologyVisualNode = {
  id: string;
  kind: TopologyVisualNodeKind;
  label: string;
  topologyKind?: string;
  count?: number;
  originalNodeId?: string;
  componentId?: string;
  memberNodeIds?: string[];
  summary?: string;
  selected: boolean;
  dimmed: boolean;
  node?: TopologyNode;
};

export type TopologyVisualEdge = {
  id: string;
  source: string;
  target: string;
  matchKind: string;
  authoritative: boolean;
  originalEdgeIds: string[];
  selected: boolean;
  dimmed: boolean;
};

export type TopologyVisualGraph = {
  nodes: TopologyVisualNode[];
  edges: TopologyVisualEdge[];
};

export type BuildTopologyViewInput = {
  mode: TopologyViewMode;
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  components?: readonly EndpointComponent[];
  focusedComponentID?: string | null;
  expandedClusters?: ReadonlySet<string> | readonly string[];
  selection?: TopologySelection;
};

const collapsedKinds = new Set([
  "mcp-tool",
  "plugin-binding",
  "external-plugin",
]);

function expanded(
  expandedClusters: BuildTopologyViewInput["expandedClusters"],
  id: string,
) {
  if (!expandedClusters) return false;
  return "has" in expandedClusters
    ? expandedClusters.has(id)
    : expandedClusters.includes(id);
}

function componentNode(
  component: EndpointComponent,
  selection: TopologySelection,
): TopologyVisualNode {
  const id = component.endpoint.id;
  return {
    id,
    kind: "component",
    label: component.endpoint.label,
    topologyKind: component.endpoint.kind,
    count: component.toolCount,
    originalNodeId: id,
    componentId: id,
    memberNodeIds: [...component.memberNodeIds],
    summary: componentSummary(component),
    selected: selection?.kind === "node" && selection.id === id,
    dimmed: false,
    node: component.endpoint,
  };
}

function connectedIDs(
  graph: TopologyVisualGraph,
  selection: TopologySelection,
) {
  if (!selection) return new Set(graph.nodes.map((node) => node.id));
  if (selection.kind === "node") {
    const selectedVisualIDs = graph.nodes
      .filter(
        (node) =>
          node.id === selection.id ||
          node.originalNodeId === selection.id ||
          node.memberNodeIds?.includes(selection.id) ||
          (node.kind === "cluster" && node.componentId === selection.id),
      )
      .map((node) => node.id);
    const connected = new Set(selectedVisualIDs);
    for (const edge of graph.edges) {
      if (selectedVisualIDs.includes(edge.source)) connected.add(edge.target);
      if (selectedVisualIDs.includes(edge.target)) connected.add(edge.source);
    }
    return connected;
  }
  const edge = graph.edges.find(
    (candidate) =>
      candidate.id === selection.id ||
      candidate.originalEdgeIds.includes(selection.id),
  );
  return edge
    ? new Set([edge.source, edge.target])
    : new Set(graph.nodes.map((node) => node.id));
}

function applySelection(
  graph: TopologyVisualGraph,
  selection: TopologySelection,
  selectionPath = false,
) {
  const connected = selectionPath
    ? new Set(graph.nodes.map((node) => node.id))
    : connectedIDs(graph, selection);
  for (const node of graph.nodes) {
    const selected =
      selection?.kind === "node" &&
      (node.id === selection.id ||
        node.originalNodeId === selection.id ||
        node.memberNodeIds?.includes(selection.id) ||
        (node.kind === "cluster" && node.componentId === selection.id));
    node.selected = Boolean(selected);
    node.dimmed = Boolean(selection) && !connected.has(node.id);
  }
  for (const edge of graph.edges) {
    edge.selected =
      selection?.kind === "edge" &&
      (edge.id === selection.id || edge.originalEdgeIds.includes(selection.id));
    edge.dimmed =
      Boolean(selection) &&
      !edge.selected &&
      !connected.has(edge.source) &&
      !connected.has(edge.target);
  }
}

function overview(
  nodes: TopologyNode[],
  edges: TopologyEdge[],
  components: readonly EndpointComponent[],
  selection: TopologySelection,
): TopologyVisualGraph {
  const transports = nodes.filter((node) => node.kind === "mcp-transport");
  const visualNodes = [
    ...transports.map((node): TopologyVisualNode => ({
      id: node.id,
      kind: "transport",
      label: node.label,
      topologyKind: node.kind,
      originalNodeId: node.id,
      selected: false,
      dimmed: false,
      node,
    })),
    ...components.map((component) => componentNode(component, selection)),
  ];
  const transportIDs = new Set(transports.map((node) => node.id));
  const seen = new Set<string>();
  const visualEdges: TopologyVisualEdge[] = [];
  for (const component of components) {
    for (const memberID of component.memberNodeIds) {
      if (!transportIDs.has(memberID)) continue;
      const key = `${memberID}->${component.endpoint.id}`;
      if (seen.has(key)) continue;
      seen.add(key);
      const originalEdgeIds = edges
        .filter(
          (edge) =>
            (edge.source === memberID &&
              component.memberNodeIds.includes(edge.target)) ||
            (edge.target === memberID &&
              component.memberNodeIds.includes(edge.source)),
        )
        .map((edge) => edge.id);
      visualEdges.push({
        id: `summary:${memberID}:${component.endpoint.id}`,
        source: memberID,
        target: component.endpoint.id,
        matchKind: "summary",
        authoritative: false,
        originalEdgeIds,

        selected: false,
        dimmed: false,
      });
    }
  }
  const graph = { nodes: visualNodes, edges: visualEdges };
  applySelection(graph, selection);
  return graph;
}

const matchPriority: Record<string, number> = {
  unresolved: 0,
  ambiguous: 1,
  "base-prefix": 2,
  exact: 3,
  summary: 4,
};

function selectedToolPathMemberIDs(
  memberIDs: ReadonlySet<string>,
  nodes: TopologyNode[],
  edges: TopologyEdge[],
  selection: TopologySelection,
) {
  if (selection?.kind !== "node") return null;
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const tool = byID.get(selection.id);
  if (tool?.kind !== "mcp-tool" || !memberIDs.has(tool.id)) return null;

  const pathIDs = new Set([tool.id]);
  const apiIDs = new Set<string>();
  const bindingIDs = new Set<string>();
  for (const edge of edges) {
    if (edge.source !== tool.id && edge.target !== tool.id) continue;
    const otherID = edge.source === tool.id ? edge.target : edge.source;
    if (!memberIDs.has(otherID)) continue;
    const other = byID.get(otherID);
    if (
      other?.kind === "mcp-transport" ||
      other?.kind === "erp-api" ||
      other?.kind === "unresolved-endpoint" ||
      other?.kind === "ambiguous-endpoint" ||
      other?.kind === "plugin-binding" ||
      other?.kind === "external-plugin"
    ) {
      pathIDs.add(otherID);
    }
    if (other?.kind === "erp-api") apiIDs.add(otherID);
    if (other?.kind === "plugin-binding") bindingIDs.add(otherID);
  }

  for (const edge of edges) {
    const apiID = apiIDs.has(edge.source)
      ? edge.source
      : apiIDs.has(edge.target)
        ? edge.target
        : null;
    if (apiID) {
      const endpointID = edge.source === apiID ? edge.target : edge.source;
      const endpoint = byID.get(endpointID);
      const matchesToolEndpoint =
        endpoint?.kind === "erp-endpoint" &&
        (!tool.tool?.endpointPath ||
          endpoint.endpoint?.path === tool.tool.endpointPath) &&
        (!tool.tool?.method || endpoint.endpoint?.method === tool.tool.method);
      if (matchesToolEndpoint) pathIDs.add(endpointID);
    }

    const bindingID = bindingIDs.has(edge.source)
      ? edge.source
      : bindingIDs.has(edge.target)
        ? edge.target
        : null;
    if (bindingID) {
      const pluginID = edge.source === bindingID ? edge.target : edge.source;
      if (byID.get(pluginID)?.kind === "external-plugin") pathIDs.add(pluginID);
    }
  }
  return pathIDs;
}

function focused(
  component: EndpointComponent,
  nodes: TopologyNode[],
  edges: TopologyEdge[],
  mode: TopologyViewMode,
  expandedClusters: BuildTopologyViewInput["expandedClusters"],
  selection: TopologySelection,
): TopologyVisualGraph {
  const memberIDs = new Set(component.memberNodeIds);
  const selectedToolPath = selectedToolPathMemberIDs(
    memberIDs,
    nodes,
    edges,
    selection,
  );
  const rawNodes = nodes.filter(
    (node) =>
      memberIDs.has(node.id) &&
      (!selectedToolPath || selectedToolPath.has(node.id)),
  );
  const visualID = new Map<string, string>();
  const visualNodes: TopologyVisualNode[] = [];
  const endpointID = component.endpoint.id;

  for (const node of rawNodes) {
    const kind: TopologyVisualNodeKind =
      node.id === endpointID
        ? "component"
        : node.kind === "mcp-transport"
          ? "transport"
          : "entity";
    if (kind === "entity" && collapsedKinds.has(node.kind)) {
      const clusterID = `cluster:${endpointID}:${node.kind}`;
      visualID.set(node.id, clusterID);
      continue;
    }
    visualID.set(node.id, node.id);
    visualNodes.push({
      id: node.id,
      kind,
      label: node.label,
      topologyKind: node.kind,
      originalNodeId: node.id,
      componentId: endpointID,
      selected: false,
      dimmed: false,
      summary: node.endpoint?.method,
      node,
    });
  }

  for (const kind of collapsedKinds) {
    const members = rawNodes.filter((node) => node.kind === kind);
    const clusterID = `cluster:${endpointID}:${kind}`;
    const shouldCollapse =
      members.length > INDIVIDUAL_NODE_LIMIT &&
      mode !== "expanded" &&
      !expanded(expandedClusters, clusterID);
    if (shouldCollapse) {
      visualNodes.push({
        id: clusterID,
        kind: "cluster",
        label: `${members.length} ${kind.replaceAll("-", " ")}${members.length === 1 ? "" : "s"}`,
        topologyKind: kind,
        count: members.length,
        componentId: endpointID,
        memberNodeIds: members.map((node) => node.id),
        selected: false,
        dimmed: false,
      });
    } else {
      for (const node of members) {
        visualID.set(node.id, node.id);
        visualNodes.push({
          id: node.id,
          kind: "entity",
          label: node.label,
          topologyKind: node.kind,
          originalNodeId: node.id,
          componentId: endpointID,
          selected: false,
          dimmed: false,
          summary: node.endpoint?.method,
          node,
        });
      }
    }
  }

  const aggregate = new Map<string, TopologyVisualEdge>();
  for (const edge of edges) {
    if (!memberIDs.has(edge.source) || !memberIDs.has(edge.target)) continue;
    const source = visualID.get(edge.source);
    const target = visualID.get(edge.target);
    if (!source || !target || source === target) continue;
    const key = `${source}->${target}`;
    const existing = aggregate.get(key);
    if (existing) {
      existing.originalEdgeIds.push(edge.id);
      existing.authoritative ||= edge.authoritative;
      if (
        (matchPriority[edge.matchKind] ?? 5) <
        (matchPriority[existing.matchKind] ?? 5)
      ) {
        existing.matchKind = edge.matchKind;
      }
      continue;
    }
    aggregate.set(key, {
      id: `aggregate:${source}:${target}`,
      source,
      target,
      matchKind: edge.matchKind,
      authoritative: edge.authoritative,
      originalEdgeIds: [edge.id],
      selected: false,
      dimmed: false,
    });
  }
  const graph = { nodes: visualNodes, edges: [...aggregate.values()] };
  applySelection(graph, selection, Boolean(selectedToolPath));
  return graph;
}

export function buildTopologyView({
  mode,
  nodes,
  edges,
  components = buildEndpointComponents(nodes, edges),
  focusedComponentID,
  expandedClusters = new Set<string>(),
  selection = null,
}: BuildTopologyViewInput): TopologyVisualGraph {
  if (mode === "overview" || !focusedComponentID)
    return overview(
      nodes,
      edges,
      components.slice(0, compactTopologyComponentLimit),
      selection,
    );
  const component = components.find(
    (item) => item.endpoint.id === focusedComponentID,
  );
  return component
    ? focused(component, nodes, edges, mode, expandedClusters, selection)
    : overview(nodes, edges, components, selection);
}
