import { render, screen } from "@testing-library/react";
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
            ],
            edges: [
              {
                id: "e",
                source: "t",
                target: "a",
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
  expect(
    await screen.findByRole("table", {
      name: "Accessible topology relationships",
    }),
  ).toBeInTheDocument();
  expect(screen.getByText(/exact/)).toBeInTheDocument();
  expect(screen.getByText("ERP APIs (1)")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Invoices" })).toBeInTheDocument();
});
