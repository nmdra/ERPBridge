import { render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";

import { ToolDetails, Tools } from "./Tools";

test("renders the safe MCP tool inventory", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            items: [
              {
                name: "list-employees",
                version: "1.0.0",
                module: "hr",
                active: true,
                method: "GET",
                endpointPath: "/api/resource/Employee",
                description: "List employees",
                manifest: {
                  apiVersion: "v1",
                  kind: "MCPTool",
                  description: {
                    short: "List employees",
                    whenToUse: ["Find employees"],
                    whenNotToUse: ["Do not create employees"],
                    examples: ["Find all employees"],
                  },
                  inputType: "object",
                  inputFields: [
                    {
                      name: "company",
                      type: "string",
                      description: "Company name",
                      required: true,
                    },
                  ],
                  outputType: "array",
                  execution: {
                    type: "http",
                    method: "GET",
                    endpointPath: "/api/resource/Employee",
                    responsePath: "data",
                  },
                  security: { authType: "api-key", allowedRoles: ["reader"] },
                },
              },
            ],
          }),
          { status: 200 },
        ),
      ),
    ),
  );
  render(<Tools contextName="local" />);
  expect(
    await screen.findByRole("table", { name: "Safe MCP tool inventory" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "list-employees" })).toHaveAttribute(
    "href",
    "/tools/list-employees",
  );
  expect(screen.getByText(/\/api\/resource\/Employee/)).toBeInTheDocument();
});

test("renders a user-friendly tool manifest", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            items: [
              {
                name: "list-employees",
                version: "1.0.0",
                module: "hr",
                active: true,
                manifest: {
                  apiVersion: "v1",
                  kind: "MCPTool",
                  description: {
                    short: "List employees",
                    whenToUse: ["Find employees"],
                    whenNotToUse: ["Do not create employees"],
                    examples: ["Find all employees"],
                  },
                  inputType: "object",
                  inputFields: [
                    {
                      name: "company",
                      type: "string",
                      description: "Company name",
                      required: true,
                    },
                  ],
                  outputType: "array",
                  execution: {
                    type: "http",
                    method: "GET",
                    endpointPath: "/api/resource/Employee",
                    responsePath: "data",
                  },
                  security: { authType: "api-key", allowedRoles: ["reader"] },
                },
              },
            ],
          }),
          { status: 200 },
        ),
      ),
    ),
  );
  render(<ToolDetails contextName="local" toolName="list-employees" />);
  expect(
    await screen.findByRole("heading", { name: "list-employees" }),
  ).toBeInTheDocument();
  expect(screen.getByText("Find employees")).toBeInTheDocument();
  expect(screen.getByText("company")).toBeInTheDocument();
  expect(screen.getByText("GET")).toBeInTheDocument();
  expect(screen.getAllByText("/api/resource/Employee").length).toBeGreaterThan(
    0,
  );
  expect(
    screen.queryByRole("heading", { name: "Plugin bindings" }),
  ).not.toBeInTheDocument();
});
