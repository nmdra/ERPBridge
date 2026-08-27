import { expect, test } from "vitest";

import type { TopologyEdge, TopologyNode } from "../hooks/useTopology";
import {
  buildEndpointComponents,
  componentSummary,
  compactTopologyNodeThreshold,
  compactTopologyComponentLimit,
  focusedComponentGraph,
  shouldUseCompactTopology,
} from "./topologyPresentation";

function node(id: string, kind: string, label = id): TopologyNode {
  return { id, kind, label };
}

function edge(
  id: string,
  source: string,
  target: string,
  matchKind = "exact",
): TopologyEdge {
  return {
    id,
    source,
    target,
    matchKind,
    authoritative: matchKind === "exact",
  };
}

test("switches to compact mode only for large filtered graphs", () => {
  expect(shouldUseCompactTopology([], [])).toBe(false);
  expect(
    shouldUseCompactTopology(
      Array.from({ length: compactTopologyNodeThreshold - 1 }, (_, index) =>
        node(String(index), "mcp-tool"),
      ),
      [],
    ),
  ).toBe(false);
  expect(
    shouldUseCompactTopology(
      Array.from({ length: compactTopologyNodeThreshold }, (_, index) =>
        node(String(index), "mcp-tool"),
      ),
      [],
    ),
  ).toBe(true);
  expect(
    shouldUseCompactTopology(
      [],
      Array.from({ length: 30 }, (_, index) => edge(String(index), "a", "b")),
    ),
  ).toBe(true);
});

test("groups an endpoint with its MCP and plugin relationship chain", () => {
  const nodes = [
    node("transport", "mcp-transport"),
    node("tool", "mcp-tool"),
    node("api", "erp-api", "mockerp-items"),
    node("binding", "plugin-binding"),
    node("plugin", "external-plugin"),
  ];
  const edges = [
    edge("transport-tool", "transport", "tool"),
    edge("tool-api", "tool", "api", "base-prefix"),
    edge("tool-binding", "tool", "binding"),
    edge("binding-plugin", "binding", "plugin"),
  ];

  const [component] = buildEndpointComponents(nodes, edges);
  expect(component.memberNodeIds).toEqual([
    "api",
    "tool",
    "transport",
    "binding",
    "plugin",
  ]);
  expect(component.toolCount).toBe(1);
  expect(component.matchCounts["base-prefix"]).toBe(1);
  expect(componentSummary(component)).toContain("1 base-prefix");

  const focused = focusedComponentGraph(component, nodes, edges);
  expect(focused.nodes.map((item) => item.id)).toHaveLength(5);
  expect(focused.edges.map((item) => item.id)).toHaveLength(4);
});

test("ranks unresolved and ambiguous endpoint components before exact ones", () => {
  const nodes = [
    node("exact-api", "erp-api", "Exact"),
    node("ambiguous-api", "erp-api", "Ambiguous"),
    node("unresolved", "unresolved-endpoint", "Unresolved"),
    node("exact-tool", "mcp-tool"),
    node("ambiguous-tool", "mcp-tool"),
  ];
  const edges = [
    edge("exact-edge", "exact-tool", "exact-api", "exact"),
    edge("ambiguous-edge", "ambiguous-tool", "ambiguous-api", "ambiguous"),
  ];

  expect(
    buildEndpointComponents(nodes, edges).map((item) => item.endpoint.id),
  ).toEqual(["unresolved", "ambiguous-api", "exact-api"]);
});

test("keeps compact display bounded while preserving the full model", () => {
  const nodes = Array.from(
    { length: compactTopologyComponentLimit + 4 },
    (_, index) => node(`api-${index}`, "erp-api"),
  );
  const components = buildEndpointComponents(nodes, []);
  expect(components).toHaveLength(compactTopologyComponentLimit + 4);
  expect(components.slice(0, compactTopologyComponentLimit)).toHaveLength(
    compactTopologyComponentLimit,
  );
});
