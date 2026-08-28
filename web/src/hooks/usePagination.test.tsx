import { act, renderHook } from "@testing-library/react";
import { expect, test } from "vitest";

import { usePagination } from "./usePagination";

test("paginates items and keeps navigation within the available pages", () => {
  const { result } = renderHook(() =>
    usePagination(["one", "two", "three", "four", "five"], 2),
  );

  expect(result.current).toMatchObject({
    page: 1,
    pageCount: 3,
    firstItem: 1,
    lastItem: 2,
    pageItems: ["one", "two"],
  });

  act(() => result.current.next());
  expect(result.current.pageItems).toEqual(["three", "four"]);

  act(() => result.current.setPage(99));
  expect(result.current).toMatchObject({
    page: 3,
    firstItem: 5,
    lastItem: 5,
    pageItems: ["five"],
  });

  act(() => result.current.next());
  expect(result.current.page).toBe(3);

  act(() => result.current.setPage(-1));
  act(() => result.current.previous());
  expect(result.current.page).toBe(1);
});

test("resets for a new item collection and clamps when its count shrinks", () => {
  const items = ["one", "two", "three", "four", "five"];
  const { result, rerender } = renderHook(
    ({ source }) => usePagination(source, 2),
    { initialProps: { source: items } },
  );

  act(() => result.current.setPage(3));
  const changedItems = ["zero", ...items.slice(1)];
  rerender({ source: changedItems });
  expect(result.current).toMatchObject({ page: 1, pageItems: ["zero", "two"] });

  act(() => result.current.setPage(3));
  changedItems.splice(3);
  rerender({ source: changedItems });
  expect(result.current).toMatchObject({
    page: 2,
    pageCount: 2,
    firstItem: 3,
    lastItem: 3,
    pageItems: ["three"],
  });
});
