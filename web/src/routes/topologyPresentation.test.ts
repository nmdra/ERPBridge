import { expect, test } from "vitest";

import type { TopologyEdge, TopologyNode } from "../hooks/useTopology";
import {
  buildCompactTopologyOverview,
  buildEndpointComponents,
  componentSummary,
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

test("uses a compact overview whenever the filtered graph has selectable components", () => {
  expect(shouldUseCompactTopology([], [])).toBe(false);
  expect(shouldUseCompactTopology([node("api", "erp-api")], [])).toBe(true);
  expect(shouldUseCompactTopology([node("tool", "mcp-tool")], [])).toBe(true);
});

test("groups an endpoint with its MCP and plugin relationship chain", () => {
  const nodes = [
    node("transport", "mcp-transport"),
    node("tool", "mcp-tool"),
    node("api", "erp-api", "mockerp-items"),
    node("endpoint", "erp-endpoint", "/api/items"),
    node("binding", "plugin-binding"),
    node("plugin", "external-plugin"),
  ];
  const edges = [
    edge("transport-tool", "transport", "tool"),
    edge("tool-api", "tool", "api", "base-prefix"),
    edge("api-endpoint", "api", "endpoint", "base-prefix"),
    edge("tool-binding", "tool", "binding"),
    edge("binding-plugin", "binding", "plugin"),
  ];

  const [component] = buildEndpointComponents(nodes, edges);
  expect(component.memberNodeIds).toEqual([
    "api",
    "endpoint",
    "tool",
    "transport",
    "binding",
    "plugin",
  ]);
  expect(component.toolCount).toBe(1);
  expect(component.matchCounts["base-prefix"]).toBe(1);
  expect(componentSummary(component)).toContain("1 base-prefix");

  const focused = focusedComponentGraph(component, nodes, edges);
  expect(focused.nodes.map((item) => item.id)).toHaveLength(6);
  expect(focused.edges.map((item) => item.id)).toHaveLength(5);
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

test("builds a connected transport-to-component overview", () => {
  const nodes = [
    node("transport", "mcp-transport"),
    node("tool", "mcp-tool"),
    node("api", "erp-api", "Invoices"),
    node("ambiguous", "ambiguous-endpoint", "/invoices"),
  ];
  const edges = [
    edge("transport-tool", "transport", "tool"),
    edge("tool-api", "tool", "api"),
    edge("tool-ambiguous", "tool", "ambiguous", "ambiguous"),
  ];

  const overview = buildCompactTopologyOverview(
    buildEndpointComponents(nodes, edges),
    nodes,
  );

  expect(overview.nodes.map((item) => item.id)).toEqual([
    "transport",
    "api",
    "ambiguous",
  ]);
  expect(overview.edges).toEqual([
    expect.objectContaining({ source: "transport", target: "api" }),
    expect.objectContaining({ source: "transport", target: "ambiguous" }),
  ]);
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
