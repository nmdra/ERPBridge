import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";

import { App } from "./App";

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
