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
                id: "endpoint",
                kind: "erp-endpoint",
                label: "/invoices",
                endpoint: { method: "GET", path: "/invoices" },
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
                id: "api-endpoint-edge",
                source: "a",
                target: "endpoint",
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
  expect(screen.getByText("ERP endpoints (1)")).toBeInTheDocument();
  expect(screen.getByText("Plugins (1)")).toBeInTheDocument();
  expect(screen.getByText("Unresolved endpoints (1)")).toBeInTheDocument();
  expect(
    screen.getAllByRole("button", { name: "Invoices" }).length,
  ).toBeGreaterThan(0);
});

test("filters plugin nodes without placing them in the MCP tools facet", async () => {
  const user = userEvent.setup();
  render(<Topology contextName="local" />);

  await screen.findByRole("table", {
    name: "Accessible topology relationships",
  });
  const filters = screen.getByRole("button", { name: "Show filters" });
  expect(filters).toHaveAttribute("aria-expanded", "false");
  expect(
    screen.queryByRole("textbox", { name: "Search topology" }),
  ).not.toBeInTheDocument();
  await user.click(filters);
  expect(filters).toHaveAttribute("aria-expanded", "true");
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
  expect(screen.getByText("Showing 1–25 of 200")).toBeInTheDocument();
  expect(screen.getByText("Page 1 of 8")).toBeInTheDocument();
  expect(screen.getAllByRole("button", { name: "Inspect" })).toHaveLength(25);

  const user = userEvent.setup();
  for (let page = 1; page < 8; page += 1) {
    await user.click(screen.getByRole("button", { name: "Next page" }));
  }
  expect(screen.getByText("Showing 176–200 of 200")).toBeInTheDocument();
  await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]);
  expect(
    screen.getByRole("heading", { name: "Selected relationship" }),
  ).toBeInTheDocument();
});

test("uses the compact overview by default and drills into one endpoint component", async () => {
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
    screen.getByText(/Compact overview: showing 1 of 1 components/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("application", {
      name: "Interactive API to MCP topology",
    }),
  ).toBeInTheDocument();
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
    screen.getByText(/Compact overview: showing 1 of 1 components/),
  ).toBeInTheDocument();
});

test("component menu drills into ERP API, ambiguous, unresolved, and MCP tool components", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            nodes: [
              {
                id: "transport",
                kind: "mcp-transport",
                label: "MCP transport",
              },
              { id: "api", kind: "erp-api", label: "Invoices" },
              { id: "ambiguous", kind: "ambiguous-endpoint", label: "/orders" },
              {
                id: "unresolved",
                kind: "unresolved-endpoint",
                label: "/reports",
              },
              {
                id: "invoices-tool",
                kind: "mcp-tool",
                label: "list-invoices",
                tool: { name: "list-invoices", version: "1.0.0" },
              },
              {
                id: "orders-tool",
                kind: "mcp-tool",
                label: "list-orders",
                tool: { name: "list-orders", version: "1.0.0" },
              },
              {
                id: "reports-tool",
                kind: "mcp-tool",
                label: "list-reports",
                tool: { name: "list-reports", version: "1.0.0" },
              },
            ],
            edges: [
              {
                id: "transport-invoices",
                source: "transport",
                target: "invoices-tool",
                matchKind: "exact",
                authoritative: true,
              },
              {
                id: "invoices-api",
                source: "invoices-tool",
                target: "api",
                matchKind: "exact",
                authoritative: true,
              },
              {
                id: "transport-orders",
                source: "transport",
                target: "orders-tool",
                matchKind: "exact",
                authoritative: true,
              },
              {
                id: "orders-ambiguous",
                source: "orders-tool",
                target: "ambiguous",
                matchKind: "ambiguous",
                authoritative: false,
              },
              {
                id: "transport-reports",
                source: "transport",
                target: "reports-tool",
                matchKind: "exact",
                authoritative: true,
              },
              {
                id: "reports-unresolved",
                source: "reports-tool",
                target: "unresolved",
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
  await screen.findByRole("table", {
    name: "Accessible topology relationships",
  });

  expect(
    screen.getByText(/Compact overview: showing 3 of 3 components/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Show component for Invoices" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Show component for /orders" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Show component for /reports" }),
  ).toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "Show component for list-invoices" }),
  );

  expect(
    screen.getByText(/Showing related nodes for Invoices/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("heading", { name: "list-invoices" }),
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

test("renders safe diagnostics when unresolved and ambiguous nodes are selected", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            nodes: [
              {
                id: "transport",
                kind: "mcp-transport",
                label: "MCP transport",
              },
              {
                id: "ambiguous",
                kind: "ambiguous-endpoint",
                label: "/ambiguous",
                diagnosticReason:
                  "More than one registered ERP API matches this endpoint.",
              },
              {
                id: "unresolved",
                kind: "unresolved-endpoint",
                label: "/unresolved",
                diagnosticReason:
                  "No registered ERP API matches this endpoint.",
              },
              { id: "tool-a", kind: "mcp-tool", label: "ambiguous-tool" },
              { id: "tool-u", kind: "mcp-tool", label: "unresolved-tool" },
            ],
            edges: [
              {
                id: "ambiguous-edge",
                source: "tool-a",
                target: "ambiguous",
                matchKind: "ambiguous",
                diagnosticReason:
                  "More than one registered ERP API matches this endpoint.",
                authoritative: false,
              },
              {
                id: "unresolved-edge",
                source: "tool-u",
                target: "unresolved",
                matchKind: "unresolved",
                diagnosticReason:
                  "No registered ERP API matches this endpoint.",
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
  await screen.findByRole("table", {
    name: "Accessible topology relationships",
  });

  await user.click(screen.getByRole("button", { name: "/ambiguous" }));
  expect(
    screen.getByText("More than one registered ERP API matches this endpoint."),
  ).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "/unresolved" }));
  expect(
    screen.getByText("No registered ERP API matches this endpoint."),
  ).toBeInTheDocument();
});

test("switches to the relationship table and returns to topology on inspection", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            nodes: [
              {
                id: "transport",
                kind: "mcp-transport",
                label: "MCP transport",
              },
              { id: "api", kind: "erp-api", label: "Invoices" },
            ],
            edges: [
              {
                id: "edge",
                source: "transport",
                target: "api",
                matchKind: "exact",
                authoritative: true,
              },
            ],
          }),
          { status: 200 },
        ),
      ),
    ),
  );

  render(<Topology contextName="local" />);
  await screen.findByRole("tabpanel", { name: "Topology canvas" });
  await user.click(screen.getByRole("tab", { name: "Relationships" }));

  expect(screen.queryByRole("application")).not.toBeInTheDocument();
  expect(
    screen.getByRole("tabpanel", { name: "Topology relationships" }),
  ).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Inspect" }));
  expect(
    screen.getByRole("tabpanel", { name: "Topology canvas" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("heading", { name: "Selected relationship" }),
  ).toBeInTheDocument();
});
