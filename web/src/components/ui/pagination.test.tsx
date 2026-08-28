import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";

import { Pagination } from "./pagination";

test("announces the current page and exposes its visible range", () => {
  render(
    <Pagination
      label="Test pagination"
      firstItem={3}
      lastItem={4}
      onNext={vi.fn()}
      onPrevious={vi.fn()}
      page={2}
      pageCount={3}
      totalItems={5}
    />,
  );

  expect(screen.getByText("Showing 3–4 of 5")).toBeInTheDocument();
  expect(screen.getByText("Page 2 of 3")).toHaveAttribute(
    "aria-live",
    "polite",
  );
  expect(screen.getByRole("button", { name: "Previous page" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Next page" })).toBeEnabled();
});

test("disables unavailable navigation and delegates available actions", async () => {
  const user = userEvent.setup();
  const onPrevious = vi.fn();
  const onNext = vi.fn();
  const { rerender } = render(
    <Pagination
      label="Test pagination"
      firstItem={1}
      lastItem={2}
      onNext={onNext}
      onPrevious={onPrevious}
      page={1}
      pageCount={3}
      totalItems={5}
    />,
  );

  await user.click(screen.getByRole("button", { name: "Previous page" }));
  await user.click(screen.getByRole("button", { name: "Next page" }));
  expect(onPrevious).not.toHaveBeenCalled();
  expect(onNext).toHaveBeenCalledOnce();

  rerender(
    <Pagination
      label="Test pagination"
      firstItem={0}
      lastItem={0}
      onNext={onNext}
      onPrevious={onPrevious}
      page={0}
      pageCount={0}
      totalItems={0}
    />,
  );
  expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Next page" })).toBeDisabled();
});
