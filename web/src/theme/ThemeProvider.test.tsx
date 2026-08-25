import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test } from "vitest";

import { App } from "../App";
import { ThemeProvider } from "./ThemeProvider";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-reduced-motion");
});

test("persists an explicit theme choice", async () => {
  localStorage.setItem("erpbridge-console-theme", "light");
  const user = userEvent.setup();
  render(
    <ThemeProvider>
      <App />
    </ThemeProvider>,
  );

  await user.selectOptions(
    screen.getByRole("combobox", { name: "Color theme" }),
    "dark",
  );

  expect(localStorage.getItem("erpbridge-console-theme")).toBe("dark");
  expect(document.documentElement.dataset.theme).toBe("dark");
});

test("defaults to light mode", () => {
  render(
    <ThemeProvider>
      <App />
    </ThemeProvider>,
  );

  expect(screen.getByRole("combobox", { name: "Color theme" })).toHaveValue(
    "light",
  );
});

test("exposes text labels for status", () => {
  render(
    <ThemeProvider>
      <App />
    </ThemeProvider>,
  );

  expect(screen.getByRole("heading", { name: "Overview" })).toBeInTheDocument();
  expect(screen.getByRole("navigation")).toBeInTheDocument();
});
