import { useCallback, useEffect, useRef, useState } from "react";

export type Pagination<T> = {
  page: number;
  pageCount: number;
  totalItems: number;
  firstItem: number;
  lastItem: number;
  pageItems: readonly T[];
  previous: () => void;
  next: () => void;
  setPage: (page: number) => void;
};

function sameItemIdentities<T>(
  previous: readonly T[],
  current: readonly T[],
): boolean {
  return (
    previous.length === current.length &&
    previous.every((item, index) => item === current[index])
  );
}

function normalisePageSize(pageSize: number): number {
  return Number.isFinite(pageSize) && pageSize > 0 ? Math.floor(pageSize) : 1;
}

function clampPage(page: number, pageCount: number): number {
  if (pageCount === 0) {
    return 0;
  }

  return Math.min(Math.max(1, page), pageCount);
}

export function usePagination<T>(
  items: readonly T[],
  pageSize: number,
): Pagination<T> {
  const size = normalisePageSize(pageSize);
  const pageCount = Math.ceil(items.length / size);
  const [selectedPage, setSelectedPage] = useState(1);
  const previousItems = useRef(items);
  const collectionChanged = !sameItemIdentities(previousItems.current, items);
  const page = collectionChanged
    ? clampPage(1, pageCount)
    : clampPage(selectedPage, pageCount);

  useEffect(() => {
    if (sameItemIdentities(previousItems.current, items)) {
      return;
    }

    previousItems.current = items;
    setSelectedPage(1);
  }, [items]);

  useEffect(() => {
    setSelectedPage((currentPage) => clampPage(currentPage, pageCount));
  }, [pageCount]);

  const setPage = useCallback(
    (nextPage: number) => {
      setSelectedPage(clampPage(Math.floor(nextPage), pageCount));
    },
    [pageCount],
  );
  const previous = useCallback(() => setPage(page - 1), [page, setPage]);
  const next = useCallback(() => setPage(page + 1), [page, setPage]);
  const start = page === 0 ? 0 : (page - 1) * size;
  const pageItems = items.slice(start, start + size);

  return {
    page,
    pageCount,
    totalItems: items.length,
    firstItem: pageItems.length === 0 ? 0 : start + 1,
    lastItem: pageItems.length === 0 ? 0 : start + pageItems.length,
    pageItems,
    previous,
    next,
    setPage,
  };
}
