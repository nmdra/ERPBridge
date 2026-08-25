import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";

import { Plugins } from "./Plugins";

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      const path = String(input);
      if (path.includes("plugin-bindings")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              state: "available",
              items: [
                {
                  name: "transform-orders",
                  active: true,
                  pluginRef: { name: "transformer", version: "1.2.0" },
                  toolRef: { name: "list_orders", version: "1.0.0" },
                  phase: "after_response",
                  priority: 10,
                  failurePolicy: "continue",
                  configurationPresent: true,
                },
              ],
            }),
            { status: 200 },
          ),
        );
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            items: [
              {
                name: "transformer",
                version: "1.2.0",
                active: true,
                endpointConfigured: true,
                timeoutMilliseconds: 1500,
                configurationPresent: true,
              },
            ],
          }),
          { status: 200 },
        ),
      );
    }),
  );
});

test("renders safe plugin and binding metadata", async () => {
  render(<Plugins contextName="local" />);

  expect(await screen.findByText("transformer")).toBeInTheDocument();
  expect(screen.getByText("transform-orders")).toBeInTheDocument();
  expect(
    screen.queryByText("https://plugin.internal.example"),
  ).not.toBeInTheDocument();
  expect(screen.getAllByText("Present")).toHaveLength(2);
});

test("renders an unavailable state for older deployments", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ state: "unavailable", items: [] }), {
          status: 200,
        }),
      ),
    ),
  );

  render(<Plugins contextName="local" />);

  expect(
    await screen.findByText("Plugin metadata is unavailable"),
  ).toBeInTheDocument();
});
