import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";

import { FilterToolbar } from "./filter-toolbar";
import { MetricCard } from "./metric-card";
import { StateBanner } from "./state-banner";

test("exposes shared operational UI semantics", () => {
  render(
    <>
      <StateBanner
        message="The last observation is retained."
        title="Data is stale"
        tone="warning"
      />
      <MetricCard
        detail="Current browser session"
        label="Request rate"
        value="2/s"
      />
      <FilterToolbar summary="2 matching items">
        <label>
          Search
          <input aria-label="Search items" />
        </label>
      </FilterToolbar>
    </>,
  );

  expect(screen.getByRole("status")).toHaveTextContent("Data is stale");
  expect(
    screen.getByRole("region", { name: "Request rate" }),
  ).toHaveTextContent("2/s");
  expect(screen.getByRole("region", { name: "Filters" })).toHaveTextContent(
    "2 matching items",
  );
});
