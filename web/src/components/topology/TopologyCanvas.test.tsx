import { expect, test } from "vitest";

import type {
  TopologyVisualEdge,
  TopologyVisualGraph,
  TopologyVisualNode,
} from "../../routes/topologyViewModel";
import {
  buildTopologyFlow,
  handleIDs,
  topologyNodePresentation,
} from "./topologyFlow";

const componentKinds = [
  "mcp-transport",
  "mcp-tool",
  "erp-api",
  "erp-endpoint",
  "ambiguous-endpoint",
  "unresolved-endpoint",
  "plugin-binding",
  "external-plugin",
] as const;

function visualNode(
  id: string,
  kind: TopologyVisualNode["kind"],
  label = id,
  topologyKind?: string,
): TopologyVisualNode {
  return {
    id,
    kind,
    label,
    topologyKind,
    selected: false,
    dimmed: false,
  };
}

function visualEdge(
  id: string,
  source: string,
  target: string,
  matchKind = "exact",
): TopologyVisualEdge {
  return {
    id,
    source,
    target,
    matchKind,
    authoritative: matchKind === "exact",
    originalEdgeIds: [id],
    selected: false,
    dimmed: false,
  };
}

test("gives topology entities accessible roles without relying on unique shapes", () => {
  const presentations = componentKinds.map((kind) =>
    topologyNodePresentation(kind),
  );

  expect(
    presentations.map((presentation) => presentation.structuralRole),
  ).toEqual([
    "transport",
    "entity",
    "entity",
    "entity",
    "entity",
    "entity",
    "entity",
    "entity",
  ]);
  expect(presentations.map((presentation) => presentation.label)).toEqual([
    "MCP transport",
    "MCP tool",
    "ERP API",
    "ERP endpoint",
    "Ambiguous endpoint",
    "Unresolved endpoint",
    "Plugin binding",
    "External plugin",
  ]);
  expect(handleIDs("erp-api")).toEqual({
    source: "source-erp-api",
    target: "target-erp-api",
  });
});

test("converts visual graph edges to stable handles without permanent labels", () => {
  const graph: TopologyVisualGraph = {
    nodes: [
      visualNode("transport", "transport", "MCP transport", "mcp-transport"),
      visualNode("tool", "entity", "list-invoices", "mcp-tool"),
      visualNode("api", "component", "Invoices", "erp-api"),
    ],
    edges: [
      visualEdge("transport-tool", "transport", "tool"),
      visualEdge("tool-api", "tool", "api", "base-prefix"),
    ],
  };

  const flow = buildTopologyFlow(graph);

  expect(flow.edges).toEqual([
    expect.objectContaining({
      sourceHandle: "source-transport",
      targetHandle: "target-entity",
      data: expect.objectContaining({ matchKind: "exact" }),
    }),
    expect.objectContaining({
      sourceHandle: "source-entity",
      targetHandle: "target-component",
      data: expect.objectContaining({ matchKind: "base-prefix" }),
    }),
  ]);
  expect(flow.edges.every((item) => item.label === undefined)).toBe(true);
});

test("adds per-edge ports only for high-degree nodes", () => {
  const nodes = [visualNode("api", "component", "Invoices", "erp-api")];
  const edges = Array.from({ length: 6 }, (_, index) => {
    const toolID = `tool-${index}`;
    nodes.push(visualNode(toolID, "entity", toolID, "mcp-tool"));
    return visualEdge(`edge-${index}`, toolID, "api");
  });

  const flow = buildTopologyFlow({ nodes, edges });
  const api = flow.nodes.find((item) => item.id === "api");

  expect(api?.data.incomingPorts).toHaveLength(6);
  expect(flow.edges.map((item) => item.targetHandle)).toEqual(
    edges.map((item) => `target-port-${item.id}`),
  );
});
