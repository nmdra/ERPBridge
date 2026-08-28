import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";

import { PluginDetails, Plugins } from "./Plugins";

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
  expect(screen.getByRole("link", { name: "transformer" })).toHaveAttribute(
    "href",
    "/plugins/transformer/1.2.0",
  );
  expect(screen.getByText("transform-orders")).toBeInTheDocument();
  expect(
    screen.queryByText("https://plugin.internal.example"),
  ).not.toBeInTheDocument();
  expect(screen.getAllByText("Present")).toHaveLength(2);
});

test("paginates plugins and bindings independently", async () => {
  const user = userEvent.setup();
  const plugins = Array.from({ length: 26 }, (_, index) => ({
    name: `plugin-${String(index).padStart(2, "0")}`,
    version: "1.0.0",
    active: true,
    endpointConfigured: true,
    timeoutMilliseconds: 1000,
    configurationPresent: true,
  }));
  const bindings = Array.from({ length: 26 }, (_, index) => ({
    name: `binding-${String(index).padStart(2, "0")}`,
    active: true,
    pluginRef: { name: "plugin-00", version: "1.0.0" },
    toolRef: { name: "tool", version: "1.0.0" },
    phase: "after_response",
    priority: index,
    failurePolicy: "continue",
    configurationPresent: true,
  }));
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            items: String(input).includes("plugin-bindings")
              ? bindings
              : plugins,
          }),
          { status: 200 },
        ),
      ),
    ),
  );

  render(<Plugins contextName="local" />);

  expect(await screen.findAllByText("Showing 1–25 of 26")).toHaveLength(2);
  await user.click(screen.getAllByRole("button", { name: "Next page" })[0]);
  expect(screen.getByText("plugin-25")).toBeInTheDocument();
  expect(screen.queryByText("plugin-00")).not.toBeInTheDocument();
  expect(screen.getByText("binding-00")).toBeInTheDocument();
  expect(screen.getAllByText("Page 1 of 2")).toHaveLength(1);
});

test("renders safe details for an exact plugin version", async () => {
  render(
    <PluginDetails
      contextName="local"
      pluginName="transformer"
      pluginVersion="1.2.0"
    />,
  );

  expect(
    await screen.findByRole("heading", { name: "transformer" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("heading", { name: "Plugin details" }),
  ).toBeInTheDocument();
  expect(screen.getByText("transform-orders")).toBeInTheDocument();
  expect(screen.getByText("Unknown")).toBeInTheDocument();
  expect(
    screen.queryByText("https://plugin.internal.example"),
  ).not.toBeInTheDocument();
  expect(screen.queryByText("PLUGIN_SECRET")).not.toBeInTheDocument();
  expect(screen.getByRole("link", { name: /Back to plugins/ })).toHaveAttribute(
    "href",
    "/plugins",
  );
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
