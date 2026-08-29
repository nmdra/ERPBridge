import ELK from "elkjs/lib/elk.bundled.js";
import type { Edge, Node } from "@xyflow/react";

export type TopologyViewKind = "transport" | "entity" | "component" | "cluster";

export type TopologyNodeDimensions = Readonly<{
  width: number;
  height: number;
}>;

/** Stable sizes keep ELK output predictable before custom nodes are measured. */
export const topologyNodeDimensions: Readonly<
  Record<TopologyViewKind, TopologyNodeDimensions>
> = {
  transport: { width: 220, height: 72 },
  entity: { width: 220, height: 88 },
  component: { width: 240, height: 96 },
  cluster: { width: 280, height: 120 },
};

// Upper-case alias is useful to consumers that treat dimensions as constants.
export const TOPOLOGY_NODE_DIMENSIONS = topologyNodeDimensions;

const layoutCache = new Map<string, { nodes: Node[]; edges: Edge[] }>();
const MAX_LAYOUT_CACHE_ENTRIES = 32;
const elk = new ELK();

type LayoutData = {
  viewKind?: unknown;
  incomingPorts?: string[];
  outgoingPorts?: string[];
};

function viewKind(node: Node): TopologyViewKind {
  const kind = (node.data as LayoutData | undefined)?.viewKind;
  return kind === "transport" ||
    kind === "entity" ||
    kind === "component" ||
    kind === "cluster"
    ? kind
    : "component";
}

/** Returns a selection-independent key for the graph's layout structure. */
export function graphLayoutKey(nodes: Node[], edges: Edge[]): string {
  const nodeParts = nodes
    .map((node) => [node.id, viewKind(node)] as const)
    .sort(([left], [right]) => left.localeCompare(right));
  const edgeParts = edges
    .map((edge) => [edge.id, edge.source, edge.target] as const)
    .sort(([left], [right]) => left.localeCompare(right));
  return JSON.stringify({ nodes: nodeParts, edges: edgeParts });
}

function cloneResult(result: { nodes: Node[]; edges: Edge[] }) {
  return {
    nodes: result.nodes.map((node) => ({
      ...node,
      position: { ...node.position },
    })),
    edges: result.edges.map((edge) => ({ ...edge })),
  };
}

function finiteCoordinate(value: number | undefined, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

export async function layoutTopology(
  nodes: Node[],
  edges: Edge[],
): Promise<{ nodes: Node[]; edges: Edge[] }> {
  const key = graphLayoutKey(nodes, edges);
  const cached = layoutCache.get(key);
  if (cached) return cloneResult(cached);

  if (nodes.length === 0) {
    const empty = { nodes: [], edges: edges.map((edge) => ({ ...edge })) };
    layoutCache.set(key, empty);
    return cloneResult(empty);
  }

  const dimensions = new Map(
    nodes.map((node) => [node.id, topologyNodeDimensions[viewKind(node)]]),
  );
  const graph = await elk.layout({
    id: "topology",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.spacing.nodeNode": "32",
      "elk.layered.spacing.nodeNodeBetweenLayers": "96",
    },
    children: nodes.map((node) => {
      const size = dimensions.get(node.id) ?? topologyNodeDimensions.component;
      const data = (node.data as LayoutData | undefined) ?? {};
      const ports = [
        ...(data.incomingPorts ?? []).map((id) => ({
          id: `${node.id}.${id}`,
          layoutOptions: { "org.eclipse.elk.port.side": "WEST" },
        })),
        ...(data.outgoingPorts ?? []).map((id) => ({
          id: `${node.id}.${id}`,
          layoutOptions: { "org.eclipse.elk.port.side": "EAST" },
        })),
      ];
      return {
        id: node.id,
        width: size.width,
        height: size.height,
        ports,
        ...(ports.length
          ? {
              layoutOptions: {
                "org.eclipse.elk.portConstraints": "FIXED_ORDER",
              },
            }
          : {}),
      };
    }),
    edges: edges.map((edge) => ({
      id: edge.id,
      sources: [
        edge.sourceHandle?.startsWith("source-port-")
          ? `${edge.source}.${edge.sourceHandle}`
          : edge.source,
      ],
      targets: [
        edge.targetHandle?.startsWith("target-port-")
          ? `${edge.target}.${edge.targetHandle}`
          : edge.target,
      ],
    })),
  });

  const positions = new Map(
    (graph.children ?? []).map((node, index) => [
      node.id,
      {
        x: finiteCoordinate(node.x, index * 320),
        y: finiteCoordinate(node.y, 0),
      },
    ]),
  );
  const result = {
    nodes: nodes.map((node, index) => ({
      ...node,
      position: positions.get(node.id) ?? { x: index * 320, y: 0 },
    })),
    edges: edges.map((edge) => ({ ...edge })),
  };

  layoutCache.set(key, result);
  if (layoutCache.size > MAX_LAYOUT_CACHE_ENTRIES) {
    const oldest = layoutCache.keys().next().value;
    if (oldest !== undefined) layoutCache.delete(oldest);
  }
  return cloneResult(result);
}
