import { Radio, RotateCcw } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { FilterToolbar } from "../components/ui/filter-toolbar";
import { Button } from "../components/ui/button";
import { Skeleton } from "../components/ui/skeleton";
import { Pagination } from "../components/ui/pagination";
import { StateBanner } from "../components/ui/state-banner";
import {
  useFilteredLogs,
  useLogs,
  type LogEvent,
} from "../hooks/useObservability";
import { usePagination } from "../hooks/usePagination";

const emptyFilters = {
  level: "",
  component: "",
  tool: "",
  requestId: "",
};

type LogFilters = typeof emptyFilters;

function useStreamingLogPagination(items: LogEvent[], filters: LogFilters) {
  const basePagination = usePagination(items, 50);
  const [selectedPage, setSelectedPage] = useState(1);
  const previousItems = useRef(items);
  const previousFilters = useRef(filters);
  const page = Math.min(
    Math.max(1, selectedPage),
    Math.max(1, basePagination.pageCount),
  );

  useEffect(() => {
    const collectionChanged = previousItems.current !== items;
    const filtersChanged = previousFilters.current !== filters;
    previousItems.current = items;
    previousFilters.current = filters;

    if (filtersChanged) {
      setSelectedPage(1);
    } else if (collectionChanged) {
      setSelectedPage((currentPage) =>
        Math.min(
          Math.max(1, currentPage),
          Math.max(1, basePagination.pageCount),
        ),
      );
    }
  }, [basePagination.pageCount, filters, items]);

  const setPage = useCallback(
    (nextPage: number) => {
      setSelectedPage(
        Math.min(
          Math.max(1, Math.floor(nextPage)),
          Math.max(1, basePagination.pageCount),
        ),
      );
    },
    [basePagination.pageCount],
  );
  const previous = useCallback(() => setPage(page - 1), [page, setPage]);
  const next = useCallback(() => setPage(page + 1), [page, setPage]);
  const start = (page - 1) * 50;
  const pageItems = items.slice(start, start + 50);

  return {
    ...basePagination,
    page,
    firstItem: pageItems.length === 0 ? 0 : start + 1,
    lastItem: pageItems.length === 0 ? 0 : start + pageItems.length,
    pageItems,
    next,
    previous,
    setPage,
  };
}

export function Logs({ contextName }: { contextName: string }) {
  const logs = useLogs(contextName);
  const [filters, setFilters] = useState(emptyFilters);
  const filtered = useFilteredLogs(logs.data, filters);
  const pagination = useStreamingLogPagination(filtered, filters);
  const options = useMemo(
    () => ({
      levels: [
        ...new Set(
          (logs.data ?? []).map((event) => event.level).filter(Boolean),
        ),
      ] as string[],
      components: [
        ...new Set(
          (logs.data ?? []).map((event) => event.component).filter(Boolean),
        ),
      ] as string[],
    }),
    [logs.data],
  );
  const filterCount = Object.values(filters).filter(Boolean).length;

  if (logs.loading && !logs.data) {
    return (
      <div className="space-y-6" aria-busy="true">
        <PageHeader
          description="Bounded, redacted log events for the selected context."
          eyebrow="Monitor"
          title="Logs"
        />
        <Skeleton className="h-24" />
        <Skeleton className="h-80" />
      </div>
    );
  }
  if (!logs.data && logs.error) {
    return (
      <div className="space-y-6">
        <PageHeader
          description="Bounded, redacted log events for the selected context."
          eyebrow="Monitor"
          title="Logs"
        />
        <EmptyState title="Logs are unavailable" message={logs.error} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        description="Search projected events by level, component, tool, or request ID. Raw payloads and unknown fields are not shown."
        eyebrow="Monitor"
        title="Logs"
      />
      {logs.stale || logs.error ? (
        <StateBanner
          message={
            logs.error ??
            "The live stream is reconnecting. Previously received events remain visible."
          }
          title="Showing retained log data"
          tone="warning"
        />
      ) : null}
      <FilterToolbar
        actions={
          filterCount ? (
            <Button
              onClick={() => setFilters(emptyFilters)}
              type="button"
              variant="secondary"
            >
              <RotateCcw aria-hidden="true" size={15} />
              Clear filters
            </Button>
          ) : null
        }
        summary={`${filtered.length} of ${logs.data?.length ?? 0} projected events match${filterCount ? " the current filters" : ""}.`}
      >
        <label className="text-sm">
          <span className="font-medium">Level</span>
          <select
            className="mt-1 h-10 w-full rounded-lg border border-border bg-card px-3"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                level: event.target.value,
              }))
            }
            value={filters.level}
          >
            <option value="">All levels</option>
            {options.levels.map((level) => (
              <option key={level}>{level}</option>
            ))}
          </select>
        </label>
        <label className="text-sm">
          <span className="font-medium">Component</span>
          <select
            className="mt-1 h-10 w-full rounded-lg border border-border bg-card px-3"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                component: event.target.value,
              }))
            }
            value={filters.component}
          >
            <option value="">All components</option>
            {options.components.map((component) => (
              <option key={component}>{component}</option>
            ))}
          </select>
        </label>
        <label className="text-sm">
          <span className="font-medium">Tool</span>
          <input
            className="mt-1 h-10 w-full rounded-lg border border-border bg-card px-3"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                tool: event.target.value,
              }))
            }
            placeholder="Filter tool name"
            value={filters.tool}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Request ID</span>
          <input
            className="mt-1 h-10 w-full rounded-lg border border-border bg-card px-3"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                requestId: event.target.value,
              }))
            }
            placeholder="Filter request ID"
            value={filters.requestId}
          />
        </label>
      </FilterToolbar>
      {!filtered.length ? (
        <EmptyState
          title={
            logs.data?.length ? "No matching log events" : "No log events yet"
          }
          message={
            logs.data?.length
              ? "Clear or adjust the filters to see retained projected events."
              : "The selected context has not produced a projected event in the current window."
          }
        />
      ) : (
        <Card>
          <CardContent className="p-0">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
              <div>
                <h2 className="font-semibold">Recent events</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Most recent valid timestamps appear first.
                </p>
              </div>
              <span className="inline-flex items-center gap-2 text-sm text-muted-foreground">
                <Radio
                  aria-hidden="true"
                  className={
                    logs.streaming ? "text-success" : "text-muted-foreground"
                  }
                  size={15}
                />
                {logs.streaming
                  ? "Live stream connected"
                  : "Stream disconnected"}
              </span>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[52rem] text-left text-sm">
                <caption className="sr-only">Projected log events</caption>
                <thead className="border-b border-border bg-muted/50 text-xs uppercase tracking-wider text-muted-foreground">
                  <tr>
                    <th className="px-5 py-3">Time</th>
                    <th className="px-5 py-3">Level</th>
                    <th className="px-5 py-3">Component</th>
                    <th className="px-5 py-3">Tool</th>
                    <th className="px-5 py-3">Summary</th>
                  </tr>
                </thead>
                <tbody>
                  {pagination.pageItems.map((event, index) => (
                    <tr
                      className="border-b border-border last:border-0 hover:bg-muted/30"
                      key={`${event.requestId ?? "event"}-${index}`}
                    >
                      <td className="whitespace-nowrap px-5 py-3 text-muted-foreground">
                        {event.timestamp ?? "—"}
                      </td>
                      <td className="px-5 py-3 font-medium">
                        {event.level ?? "—"}
                      </td>
                      <td className="px-5 py-3">{event.component ?? "—"}</td>
                      <td className="px-5 py-3">{event.toolName ?? "—"}</td>
                      <td className="max-w-xl px-5 py-3">
                        {event.summary ?? "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="px-5 pb-4">
              <Pagination
                label="Logs pagination"
                firstItem={pagination.firstItem}
                lastItem={pagination.lastItem}
                onNext={pagination.next}
                onPrevious={pagination.previous}
                page={pagination.page}
                pageCount={pagination.pageCount}
                totalItems={pagination.totalItems}
              />
            </div>
          </CardContent>
        </Card>
      )}
      <p className="text-xs text-muted-foreground">
        Events remain in bounded browser memory only. Last update:{" "}
        {logs.lastUpdated
          ? new Date(logs.lastUpdated).toLocaleTimeString()
          : "not yet"}
        .
      </p>
    </div>
  );
}
