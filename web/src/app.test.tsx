import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";

import { App } from "./App";

afterEach(() => {
  window.sessionStorage.clear();
  vi.unstubAllGlobals();
});

function mockConsoleFetch(contextItems: Array<{ name: string; current?: boolean }>) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      const path = String(input);
      if (path.includes("/contexts")) {
        return Promise.resolve(
          new Response(JSON.stringify({ items: contextItems }), { status: 200 }),
        );
      }
      if (path.includes("/deployment")) {
        const context = new URL(path, window.location.origin).searchParams.get(
          "context",
        );
        return Promise.resolve(
          new Response(
            JSON.stringify({ context: { name: context }, console: { state: "available" } }),
            { status: 200 },
          ),
        );
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({ state: "available", items: [], cumulative: [], rates: [] }),
          { status: 200 },
        ),
      );
    }),
  );
}

test("renders the console shell", () => {
  render(<App />);
  expect(screen.getByRole("heading", { name: "Overview" })).toBeInTheDocument();
  expect(screen.getAllByText("Monitor").length).toBeGreaterThan(0);
  expect(screen.getByRole("link", { name: "Contexts" })).toBeInTheDocument();
  expect(
    screen.getByRole("link", { name: "Integration topology" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Collapse sidebar" }),
  ).toBeInTheDocument();
});

test("starts from the context marked current by the BFF", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      const path = String(input);
      if (path.includes("/contexts")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              items: [
                { name: "local", current: false },
                { name: "production", current: true },
              ],
            }),
            { status: 200 },
          ),
        );
      }
      if (path.includes("/deployment")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              context: { name: "production" },
              console: { state: "available" },
            }),
            { status: 200 },
          ),
        );
      }
      if (path.includes("/metrics")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ state: "available", cumulative: [], rates: [] }),
            { status: 200 },
          ),
        );
      }
      if (path.includes("/tools")) {
        return Promise.resolve(
          new Response(JSON.stringify({ state: "available", items: [] }), {
            status: 200,
          }),
        );
      }
      return Promise.resolve(
        new Response(JSON.stringify({ state: "available", status: "ok" }), {
          status: 200,
        }),
      );
    }),
  );

  render(<App />);
  await waitFor(() =>
    expect(
      screen.getByRole("combobox", { name: "Select context" }),
    ).toHaveValue("production"),
  );
});

test("persists an explicit context selection across app reloads", async () => {
  const user = userEvent.setup();
  mockConsoleFetch([
    { name: "local", current: true },
    { name: "staging" },
  ]);

  const first = render(<App />);
  const selector = await screen.findByRole("combobox", { name: "Select context" });
  await user.selectOptions(selector, "staging");
  expect(selector).toHaveValue("staging");
  first.unmount();

  render(<App />);
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "Select context" })).toHaveValue(
      "staging",
    ),
  );
});

test("falls back when session context is no longer configured", async () => {
  window.sessionStorage.setItem("erpbridge-console-selected-context", "removed");
  mockConsoleFetch([{ name: "local", current: true }]);

  render(<App />);
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "Select context" })).toHaveValue(
      "local",
    ),
  );
});

test("opens an accessible mobile navigation drawer", async () => {
  const user = userEvent.setup();
  render(<App />);

  await user.click(screen.getByRole("button", { name: "Open navigation" }));
  const dialog = screen.getByRole("dialog", {
    name: "ERPBridge Console",
  });
  expect(
    within(dialog).getByRole("link", { name: "Integration topology" }),
  ).toBeInTheDocument();
  await user.click(
    within(dialog).getByRole("button", { name: "Close navigation" }),
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("collapses and expands the sidebar", async () => {
  const user = userEvent.setup();
  render(<App />);

  await user.click(screen.getByRole("button", { name: "Collapse sidebar" }));
  expect(
    screen.getByRole("button", { name: "Expand sidebar" }),
  ).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Expand sidebar" }));
  expect(
    screen.getByRole("button", { name: "Collapse sidebar" }),
  ).toBeInTheDocument();
});
