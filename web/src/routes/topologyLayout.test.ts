import type { Edge, Node } from "@xyflow/react";
import { expect, test } from "vitest";

import {
  graphLayoutKey,
  layoutTopology,
  topologyNodeDimensions,
} from "./topologyLayout";

function node(
  id: string,
  viewKind: "transport" | "entity" | "component",
): Node {
  return {
    id,
    type: "topology",
    data: { viewKind },
    position: { x: 0, y: 0 },
  };
}

function edge(id: string, source: string, target: string): Edge {
  return { id, source, target };
}

test("lays out a layered graph without changing node or edge identity", async () => {
  const nodes = [
    node("transport-1", "transport"),
    node("entity-1", "entity"),
    node("component-1", "component"),
  ];
  const edges = [
    edge("transport-entity", "transport-1", "entity-1"),
    edge("entity-component", "entity-1", "component-1"),
  ];

  const result = await layoutTopology(nodes, edges);

  expect(result.nodes.map(({ id }) => id)).toEqual(nodes.map(({ id }) => id));
  expect(result.edges.map(({ id }) => id)).toEqual(edges.map(({ id }) => id));
  expect(
    result.nodes.every(
      ({ position }) =>
        Number.isFinite(position.x) && Number.isFinite(position.y),
    ),
  ).toBe(true);

  const positions = new Map(
    result.nodes.map((item) => [item.id, item.position]),
  );
  expect(positions.get("transport-1")!.x).toBeLessThan(
    positions.get("entity-1")!.x,
  );
  expect(positions.get("entity-1")!.x).toBeLessThan(
    positions.get("component-1")!.x,
  );
});

test("lays out high-degree edges with their declared ports", async () => {
  const apiPorts = Array.from(
    { length: 6 },
    (_, index) => `target-port-e${index}`,
  );
  const nodes: Node[] = [
    {
      id: "api",
      type: "component",
      data: { viewKind: "component", incomingPorts: apiPorts },
      position: { x: 0, y: 0 },
    },
    ...apiPorts.map((_, index) => ({
      id: `tool-${index}`,
      type: "entity",
      data: { viewKind: "entity" },
      position: { x: 0, y: 0 },
    })),
  ];
  const edges: Edge[] = apiPorts.map((targetHandle, index) => ({
    id: `e${index}`,
    source: `tool-${index}`,
    sourceHandle: "source-entity",
    target: "api",
    targetHandle,
  }));

  const result = await layoutTopology(nodes, edges);

  expect(result.nodes).toHaveLength(nodes.length);
  expect(result.edges.map((item) => item.targetHandle)).toEqual(apiPorts);
});

test("uses stable dimensions and a structural key", () => {
  expect(topologyNodeDimensions.transport.width).toBeGreaterThan(0);
  expect(topologyNodeDimensions.entity.height).toBeGreaterThan(0);

  const nodes = [node("transport-1", "transport")];
  const first = graphLayoutKey(nodes, [edge("e", "transport-1", "target")]);
  const second = graphLayoutKey(
    [{ ...nodes[0], selected: true }],
    [edge("e", "transport-1", "target")],
  );
  expect(second).toBe(first);
});
