import { render, screen, within } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";

import { Logs } from "./Logs";
import { Metrics } from "./Metrics";

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      const path = String(input);
      if (path.includes("metrics")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              state: "available",
              cumulative: [{ name: "requests", value: 2 }],
              rates: [],
            }),
            { status: 200 },
          ),
        );
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            items: [{ level: "INFO", summary: "safe" }],
          }),
          { status: 200 },
        ),
      );
    }),
  );
});

test("renders a text log table", async () => {
  render(<Logs contextName="local" />);
  expect(await screen.findByText("safe")).toBeInTheDocument();
  expect(
    screen.getByRole("table", { name: "Projected log events" }),
  ).toBeInTheDocument();
});

test("sorts recent logs first", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes("logs/stream")) {
        return Promise.resolve(new Response(null, { status: 200 }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            items: [
              {
                timestamp: "2026-08-25T10:00:00Z",
                level: "INFO",
                summary: "older",
              },
              {
                timestamp: "2026-08-25T12:00:00.000000002Z",
                level: "INFO",
                summary: "newest",
              },
              {
                timestamp: "2026-08-25T12:00:00.000000001Z",
                level: "INFO",
                summary: "middle",
              },
            ],
          }),
          { status: 200 },
        ),
      );
    }),
  );

  render(<Logs contextName="local" />);
  const table = await screen.findByRole("table", {
    name: "Projected log events",
  });
  const rows = within(table).getAllByRole("row");

  expect(rows[1]).toHaveTextContent("newest");
  expect(rows[2]).toHaveTextContent("middle");
  expect(rows[3]).toHaveTextContent("older");
});

test("renders a metrics table", async () => {
  render(<Metrics contextName="local" />);
  expect(await screen.findByText("requests")).toBeInTheDocument();
  expect(
    screen.getByRole("table", { name: "Live metric samples" }),
  ).toBeInTheDocument();
});

test("keeps labeled metric rates aligned with their series", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            state: "available",
            cumulative: [
              { name: "requests", labels: { method: "GET" }, value: 10 },
              { name: "requests", labels: { method: "POST" }, value: 20 },
            ],
            rates: [
              { name: "requests", labels: { method: "GET" }, perSecond: 1 },
              { name: "requests", labels: { method: "POST" }, perSecond: 2 },
            ],
            observedAt: "2026-08-25T12:00:00Z",
          }),
          { status: 200 },
        ),
      ),
    ),
  );

  render(<Metrics contextName="local" />);
  const table = await screen.findByRole("table", {
    name: "Live metric samples",
  });
  const rows = within(table).getAllByRole("row");
  expect(rows[1]).toHaveTextContent("method=GET");
  expect(rows[1]).toHaveTextContent("1.00/s");
  expect(rows[2]).toHaveTextContent("method=POST");
  expect(rows[2]).toHaveTextContent("2.00/s");
});
