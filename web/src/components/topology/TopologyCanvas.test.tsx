import { expect, test } from "vitest";

import type { TopologyEdge, TopologyNode } from "../../hooks/useTopology";
import {
  buildTopologyFlow,
  handleIDs,
  topologyNodePresentation,
} from "./topologyFlow";

const componentKinds = [
  "mcp-transport",
  "mcp-tool",
  "erp-api",
  "ambiguous-endpoint",
  "unresolved-endpoint",
  "plugin-binding",
  "external-plugin",
] as const;

function node(id: string, kind: string): TopologyNode {
  return { id, kind, label: `${kind} label` };
}

function edge(source: string, target: string, matchKind: string): TopologyEdge {
  return {
    id: `${source}:${target}`,
    source,
    target,
    matchKind,
    authoritative: matchKind === "exact",
  };
}

test("gives every topology component an accessible, distinct visual shape", () => {
  const presentations = componentKinds.map((kind) =>
    topologyNodePresentation(kind),
  );

  expect(
    new Set(presentations.map((presentation) => presentation.shape)).size,
  ).toBe(componentKinds.length);
  expect(presentations.map((presentation) => presentation.label)).toEqual([
    "MCP transport",
    "MCP tool",
    "ERP API",
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

test("assigns labelled directional edges to their matching stable handles", () => {
  const nodes = [
    node("transport", "mcp-transport"),
    node("tool", "mcp-tool"),
    node("api", "erp-api"),
  ];
  const edges = [
    edge("transport", "tool", "exact"),
    edge("tool", "api", "base-prefix"),
  ];

  const flow = buildTopologyFlow(nodes, edges, null, false, {});

  expect(flow.nodes.map((item) => item.data.shape)).toEqual([
    "pill",
    "rectangle",
    "database",
  ]);
  expect(flow.edges).toEqual([
    expect.objectContaining({
      label: "exact",
      sourceHandle: "source-mcp-transport",
      targetHandle: "target-mcp-tool",
    }),
    expect.objectContaining({
      label: "base-prefix",
      sourceHandle: "source-mcp-tool",
      targetHandle: "target-erp-api",
    }),
  ]);
  expect(flow.edges[0].ariaLabel).toContain("authoritative");
  expect(flow.edges[1].ariaLabel).toContain("base-prefix");
});
