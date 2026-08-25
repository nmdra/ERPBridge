import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";

import { App } from "./App";

test("renders the console shell", () => {
  render(<App />);
  expect(
    screen.getByRole("heading", { name: "ERPBridge Console" }),
  ).toBeInTheDocument();
  expect(screen.getAllByText("Console")).toHaveLength(2);
  expect(screen.getByRole("note")).toHaveTextContent(
    "read-only solution for monitoring the ERPBridge server",
  );
  expect(
    screen.getByRole("button", { name: "Collapse sidebar" }),
  ).toBeInTheDocument();
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
