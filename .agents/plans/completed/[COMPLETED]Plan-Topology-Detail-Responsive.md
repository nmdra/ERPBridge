# Plan: Responsive Topology Details

> **Status: COMPLETED — verified 2026-08-26**
>
> Long topology identities now wrap inside the inspector, and the relationship
> table uses an internal horizontal scroll region without widening the page.

## Goal
Keep long tool names and relationship tables contained within the topology page while preserving readable responsive access.

## Tasks

- [x] **Task 1: Prevent inspector content overflow.** Allow long tool names, relationship identities, and endpoint paths to wrap at safe boundaries.
  - **Verify:** topology tests pass and a selected long-name node remains inside its inspector card.

- [x] **Task 2: Make the relationship table horizontally scrollable.** Keep the table aligned to its card with a bounded minimum width and an explicit horizontal scroll region.
  - **Verify:** frontend format, typecheck, lint, tests, build, and a mobile Playwright check pass without page-level horizontal overflow.
