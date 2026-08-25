import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";

import { Topology } from "./Topology";

test("renders accessible topology relationships", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            nodes: [
              {
                id: "a",
                kind: "erp-api",
                label: "Invoices",
                api: {
                  name: "Invoices",
                  method: "GET",
                  endpointPath: "/invoices",
                },
              },
              {
                id: "t",
                kind: "mcp-tool",
                label: "list-invoices",
                tool: {
                  name: "list-invoices",
                  version: "1.0.0",
                  endpointPath: "/invoices",
                },
              },
              {
                id: "b",
                kind: "plugin-binding",
                label: "transform-invoices",
                binding: {
                  name: "transform-invoices",
                  active: true,
                  pluginRef: { name: "transformer", version: "1.2.0" },
                  toolRef: { name: "list-invoices", version: "1.0.0" },
                  phase: "after_response",
                  priority: 10,
                  failurePolicy: "continue",
                  configurationPresent: true,
                },
              },
              {
                id: "p",
                kind: "external-plugin",
                label: "transformer",
                plugin: {
                  name: "transformer",
                  version: "1.2.0",
                  active: true,
                  endpointConfigured: true,
                  timeoutMilliseconds: 1500,
                  configurationPresent: true,
                },
              },
              {
                id: "u",
                kind: "unresolved-endpoint",
                label: "/unresolved",
              },
            ],
            edges: [
              {
                id: "e",
                source: "t",
                target: "a",
                matchKind: "exact",
                authoritative: true,
              },
              {
                id: "binding-edge",
                source: "t",
                target: "b",
                matchKind: "exact",
                authoritative: true,
              },
              {
                id: "plugin-edge",
                source: "b",
                target: "p",
                matchKind: "exact",
                authoritative: true,
              },
              {
                id: "unresolved-edge",
                source: "t",
                target: "u",
                matchKind: "unresolved",
                authoritative: false,
              },
            ],
          }),
          { status: 200 },
        ),
      ),
    ),
  );
  render(<Topology contextName="local" />);
  expect(
    await screen.findByRole("table", {
      name: "Accessible topology relationships",
    }),
  ).toBeInTheDocument();
  expect(screen.getAllByText(/exact/).length).toBeGreaterThan(0);
  expect(screen.getByText("ERP APIs (1)")).toBeInTheDocument();
  expect(screen.getByText("Plugins (1)")).toBeInTheDocument();
  expect(screen.getByText("Unresolved endpoints (1)")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Invoices" })).toBeInTheDocument();
});

test("filters plugin nodes without placing them in the MCP tools facet", async () => {
  const user = userEvent.setup();
  render(<Topology contextName="local" />);

  await screen.findByRole("table", {
    name: "Accessible topology relationships",
  });
  await user.type(
    screen.getByRole("textbox", { name: "Search topology" }),
    "transformer",
  );

  expect(screen.getByText("Plugins (1)")).toBeInTheDocument();
  expect(screen.queryByText("MCP tools (1)")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Inspect" })).toBeInTheDocument();
});

test("renders a bounded 100-node and 200-edge topology", async () => {
  const nodes = Array.from({ length: 100 }, (_, index) => ({
    id: `node-${index}`,
    kind: "mcp-tool",
    label: `tool-${index}`,
    tool: { name: `tool-${index}`, version: "1.0.0" },
  }));
  const edges = Array.from({ length: 200 }, (_, index) => ({
    id: `edge-${index}`,
    source: `node-${index % 100}`,
    target: `node-${(index + 1) % 100}`,
    matchKind: "exact",
    authoritative: true,
  }));
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            nodes,
            edges,
            truncated: false,
          }),
          { status: 200 },
        ),
      ),
    ),
  );

  render(<Topology contextName="local" />);

  await screen.findByRole("table", {
    name: "Accessible topology relationships",
  });
  const countStatus = screen
    .getAllByRole("status")
    .find((element) => element.textContent?.includes("100 of 100 nodes"));
  expect(countStatus).toHaveTextContent("100 of 100 nodes");
  expect(countStatus).toHaveTextContent("200 of 200 edges");
  expect(screen.getAllByRole("button", { name: "Inspect" })).toHaveLength(200);
});

test("collapses a large graph and drills into one endpoint component", async () => {
  const user = userEvent.setup();
  const nodes = [
    {
      id: "transport",
      kind: "mcp-transport",
      label: "MCP transport",
    },
    {
      id: "api",
      kind: "erp-api",
      label: "Invoices",
      api: {
        name: "Invoices",
        method: "GET",
        endpointPath: "/invoices",
      },
    },
    ...Array.from({ length: 40 }, (_, index) => ({
      id: `tool-${index}`,
      kind: "mcp-tool",
      label: `invoice-tool-${index}`,
      tool: {
        name: `invoice-tool-${index}`,
        version: "1.0.0",
        endpointPath: "/invoices",
      },
    })),
  ];
  const edges = Array.from({ length: 40 }, (_, index) => [
    {
      id: `transport-${index}`,
      source: "transport",
      target: `tool-${index}`,
      matchKind: "exact",
      authoritative: true,
    },
    {
      id: `api-${index}`,
      source: `tool-${index}`,
      target: "api",
      matchKind: "exact",
      authoritative: true,
    },
  ]).flat();
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ state: "available", nodes, edges }), {
          status: 200,
        }),
      ),
    ),
  );

  render(<Topology contextName="local" />);
  await screen.findByRole("table", {
    name: "Accessible topology relationships",
  });

  expect(
    screen.getByText(/Compact overview: showing 1 of 1 endpoint components/),
  ).toBeInTheDocument();
  await screen.findByLabelText("Interactive API to MCP topology");
  await user.click(screen.getAllByRole("button", { name: "Invoices" })[0]);
  expect(
    screen.getByText(/Showing related nodes for Invoices/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Back to compact overview" }),
  ).toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "Back to compact overview" }),
  );
  expect(
    screen.getByText(/Compact overview: showing 1 of 1 endpoint components/),
  ).toBeInTheDocument();
});

test("selects an edge and exposes safe match details", async () => {
  const user = userEvent.setup();
  render(<Topology contextName="local" />);

  await screen.findByRole("table", {
    name: "Accessible topology relationships",
  });
  await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]);

  expect(
    screen.getByRole("heading", { name: "Selected relationship" }),
  ).toBeInTheDocument();
  expect(screen.getByText("Authoritative")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Selected" })).toBeInTheDocument();
  expect(
    screen.queryByText("https://plugin.internal.example"),
  ).not.toBeInTheDocument();
});
