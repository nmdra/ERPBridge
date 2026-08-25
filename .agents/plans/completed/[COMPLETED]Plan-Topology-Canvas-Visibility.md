# Plan: Topology Canvas Visibility

> **Status: COMPLETED — verified 2026-08-26**
>
> The topology now registers individual read-only MockERP API projections,
> keeps the canvas wider at laptop widths, and refits the graph after filters
> change so related MCP and ERP API nodes remain inspectable.

## Goal
Keep the topology canvas readable when the inspector and a larger MockERP graph are visible at tablet and laptop widths.

## Tasks

- [x] **Task 1: Give the topology canvas sufficient width.** Move the side-by-side inspector breakpoint from `lg` to `xl`, keeping the inspector stacked below the canvas at 1024px-class viewports.
  - **Verify:** frontend tests and a 1024px Playwright check show the ERP API nodes and MCP tool nodes in the canvas region.

- [x] **Task 2: Validate the responsive topology presentation.** Run focused frontend checks, asset verification, and manual topology inspection with the current MockERP fixture.
  - **Verify:** frontend format, typecheck, lint, tests, and production build pass; topology shows the MockERP API nodes without changing matching or relationship data.
