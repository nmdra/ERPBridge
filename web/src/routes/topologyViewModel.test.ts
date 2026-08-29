import { expect, test } from "vitest";

import type { TopologyEdge, TopologyNode } from "../hooks/useTopology";
import {
  compactTopologyComponentLimit,
  type EndpointComponent,
} from "./topologyPresentation";
import { buildTopologyView, INDIVIDUAL_NODE_LIMIT } from "./topologyViewModel";

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

function component(
  endpoint: TopologyNode,
  memberNodeIds: string[],
): EndpointComponent {
  return {
    endpoint,
    memberNodeIds,
    memberEdges: [],
    toolCount: memberNodeIds.length,
    endpointCount: 0,
    matchCounts: {},
  };
}

test("overview keeps transports and one component node per endpoint", () => {
  const nodes = [
    node("transport", "mcp-transport"),
    node("tool", "mcp-tool"),
    node("api", "erp-api"),
  ];
  const edges = [
    edge("transport-tool", "transport", "tool"),
    edge("tool-api", "tool", "api"),
  ];
  const view = buildTopologyView({ mode: "overview", nodes, edges });

  expect(view.nodes.map((item) => [item.id, item.kind])).toEqual([
    ["transport", "transport"],
    ["api", "component"],
  ]);
  expect(view.edges).toEqual([
    expect.objectContaining({
      source: "transport",
      target: "api",
      matchKind: "summary",
      originalEdgeIds: ["transport-tool"],
    }),
  ]);
});

test("focused views preserve the MCP tool to API to ERP endpoint chain", () => {
  const api = node("api", "erp-api", "mockerp");
  const tool = node("tool", "mcp-tool", "list-items");
  const endpoint: TopologyNode = {
    ...node("endpoint", "erp-endpoint", "/api/resource/Item"),
    endpoint: { method: "GET", path: "/api/resource/Item" },
  };
  const nodes = [api, tool, endpoint];
  const edges = [
    edge("tool-api", "tool", "api"),
    edge("api-endpoint", "api", "endpoint"),
  ];
  const view = buildTopologyView({
    mode: "focused",
    nodes,
    edges,
    components: [
      component(
        api,
        nodes.map((item) => item.id),
      ),
    ],
    focusedComponentID: api.id,
  });

  expect(view.nodes.map((item) => item.id).sort()).toEqual(
    ["api", "tool", "endpoint"].sort(),
  );
  expect(view.edges).toEqual([
    expect.objectContaining({ source: "tool", target: "api" }),
    expect.objectContaining({ source: "api", target: "endpoint" }),
  ]);
  expect(view.nodes.find((item) => item.id === "endpoint")).toEqual(
    expect.objectContaining({
      topologyKind: "erp-endpoint",
      summary: "GET",
    }),
  );
});

test("selecting an MCP tool shows only its execution and plugin paths", () => {
  const transport = node("transport", "mcp-transport");
  const api = node("api", "erp-api", "users");
  const selectedTool: TopologyNode = {
    ...node("user-list", "mcp-tool", "user_list"),
    tool: {
      name: "user_list",
      version: "1.0.0",
      method: "GET",
      endpointPath: "/api/users",
    },
  };
  const otherTool = node("other-tool", "mcp-tool", "other_list");
  const selectedEndpoint: TopologyNode = {
    ...node("users-endpoint", "erp-endpoint", "/api/users"),
    endpoint: { method: "GET", path: "/api/users" },
  };
  const otherEndpoint: TopologyNode = {
    ...node("other-endpoint", "erp-endpoint", "/api/other"),
    endpoint: { method: "GET", path: "/api/other" },
  };
  const binding = node("binding", "plugin-binding", "format-users");
  const plugin = node("plugin", "external-plugin", "formatter");
  const nodes = [
    transport,
    api,
    selectedTool,
    otherTool,
    selectedEndpoint,
    otherEndpoint,
    binding,
    plugin,
  ];
  const edges = [
    edge("transport-selected", transport.id, selectedTool.id),
    edge("selected-api", selectedTool.id, api.id),
    edge("api-selected-endpoint", api.id, selectedEndpoint.id),
    edge("api-other-endpoint", api.id, otherEndpoint.id),
    edge("transport-other", transport.id, otherTool.id),
    edge("other-api", otherTool.id, api.id),
    edge("selected-binding", selectedTool.id, binding.id),
    edge("binding-plugin", binding.id, plugin.id),
  ];
  const view = buildTopologyView({
    mode: "focused",
    nodes,
    edges,
    components: [
      component(
        api,
        nodes.map((item) => item.id),
      ),
    ],
    focusedComponentID: api.id,
    selection: { kind: "node", id: selectedTool.id },
  });

  expect(view.nodes.map((item) => item.id).sort()).toEqual(
    [
      transport.id,
      api.id,
      selectedTool.id,
      selectedEndpoint.id,
      binding.id,
      plugin.id,
    ].sort(),
  );
  expect(view.edges.map((item) => [item.source, item.target])).toEqual(
    expect.arrayContaining([
      [transport.id, selectedTool.id],
      [selectedTool.id, api.id],
      [api.id, selectedEndpoint.id],
      [selectedTool.id, binding.id],
      [binding.id, plugin.id],
    ]),
  );
  expect(view.nodes.some((item) => item.id === otherTool.id)).toBe(false);
  expect(view.nodes.some((item) => item.id === otherEndpoint.id)).toBe(false);
});

test("focused high-cardinality relationships collapse into a cluster", () => {
  const api = node("api", "erp-api");
  const tools = Array.from({ length: INDIVIDUAL_NODE_LIMIT + 1 }, (_, i) =>
    node(`tool-${i}`, "mcp-tool"),
  );
  const nodes = [api, ...tools];
  const edges = tools.flatMap((tool) => [
    edge(`${tool.id}-api`, tool.id, api.id),
  ]);
  const view = buildTopologyView({
    mode: "focused",
    nodes,
    edges,
    components: [
      component(
        api,
        nodes.map((item) => item.id),
      ),
    ],
    focusedComponentID: api.id,
  });

  expect(view.nodes.filter((item) => item.kind === "cluster")).toEqual([
    expect.objectContaining({
      id: "cluster:api:mcp-tool",
      memberNodeIds: tools.map((item) => item.id),
      componentId: "api",
    }),
  ]);
  expect(view.nodes.some((item) => item.id === "tool-0")).toBe(false);
  expect(view.edges).toEqual([
    expect.objectContaining({ source: "cluster:api:mcp-tool", target: "api" }),
  ]);

  const selectedView = buildTopologyView({
    mode: "focused",
    nodes,
    edges,
    components: [
      component(
        api,
        nodes.map((item) => item.id),
      ),
    ],
    focusedComponentID: api.id,
    selection: { kind: "node", id: api.id },
  });
  expect(
    selectedView.nodes.find((item) => item.id === "cluster:api:mcp-tool"),
  ).toEqual(expect.objectContaining({ selected: true, dimmed: false }));
});

test("expanded mode and explicit cluster expansion restore raw node IDs", () => {
  const api = node("api", "erp-api");
  const tools = Array.from({ length: INDIVIDUAL_NODE_LIMIT + 1 }, (_, i) =>
    node(`tool-${i}`, "mcp-tool"),
  );
  const nodes = [api, ...tools];
  const edges = tools.map((tool) => edge(`${tool.id}-api`, tool.id, api.id));
  const args = {
    nodes,
    edges,
    components: [
      component(
        api,
        nodes.map((item) => item.id),
      ),
    ],
    focusedComponentID: "api",
  } as const;

  expect(
    buildTopologyView({ ...args, mode: "expanded" }).nodes.filter(
      (item) => item.kind === "entity",
    ),
  ).toHaveLength(tools.length);
  expect(
    buildTopologyView({
      ...args,
      mode: "focused",
      expandedClusters: ["cluster:api:mcp-tool"],
    }).nodes.find((item) => item.id === "tool-0"),
  ).toEqual(
    expect.objectContaining({ originalNodeId: "tool-0", componentId: "api" }),
  );
});

test("bounds overview rendering to the component limit", () => {
  const nodes = Array.from({ length: 30 }, (_, index) =>
    node(`api-${index}`, "erp-api"),
  );
  const view = buildTopologyView({
    mode: "overview",
    nodes,
    edges: [],
    components: nodes.map((item) => component(item, [item.id])),
  });

  expect(view.nodes.filter((item) => item.kind === "component")).toHaveLength(
    compactTopologyComponentLimit,
  );
});

test("selection marks the selected raw node and dims disconnected visual nodes", () => {
  const nodes = [
    node("transport-a", "mcp-transport"),
    node("transport-b", "mcp-transport"),
    node("api", "erp-api"),
  ];
  const view = buildTopologyView({
    mode: "overview",
    nodes,
    edges: [],
    components: [component(nodes[2], ["api", "transport-a"])],
    selection: { kind: "node", id: "api" },
  });

  expect(view.nodes.find((item) => item.id === "api")).toEqual(
    expect.objectContaining({ selected: true, dimmed: false }),
  );
  expect(view.nodes.find((item) => item.id === "transport-b")).toEqual(
    expect.objectContaining({ selected: false, dimmed: true }),
  );
});
