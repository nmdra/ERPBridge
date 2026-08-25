import { useMemo, useState } from "react";

import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { Skeleton } from "../components/ui/skeleton";
import { useFilteredLogs, useLogs } from "../hooks/useObservability";

export function Logs({ contextName }: { contextName: string }) {
  const logs = useLogs(contextName);
  const [filters, setFilters] = useState({
    level: "",
    component: "",
    tool: "",
    requestId: "",
  });
  const filtered = useFilteredLogs(logs.data, filters);
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

  if (logs.loading) return <Skeleton className="h-48 w-full" />;
  if (!logs.data && logs.error)
    return <EmptyState title="Logs are unavailable" message={logs.error} />;
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <label className="text-sm">
            Level
            <select
              className="mt-1 h-9 w-full rounded-md border border-border bg-card px-2"
              value={filters.level}
              onChange={(event) =>
                setFilters({ ...filters, level: event.target.value })
              }
            >
              <option value="">All levels</option>
              {options.levels.map((level) => (
                <option key={level}>{level}</option>
              ))}
            </select>
          </label>
          <label className="text-sm">
            Component
            <select
              className="mt-1 h-9 w-full rounded-md border border-border bg-card px-2"
              value={filters.component}
              onChange={(event) =>
                setFilters({ ...filters, component: event.target.value })
              }
            >
              <option value="">All components</option>
              {options.components.map((component) => (
                <option key={component}>{component}</option>
              ))}
            </select>
          </label>
          <label className="text-sm">
            Tool
            <input
              className="mt-1 h-9 w-full rounded-md border border-border bg-card px-2"
              value={filters.tool}
              onChange={(event) =>
                setFilters({ ...filters, tool: event.target.value })
              }
            />
          </label>
          <label className="text-sm">
            Request ID
            <input
              className="mt-1 h-9 w-full rounded-md border border-border bg-card px-2"
              value={filters.requestId}
              onChange={(event) =>
                setFilters({ ...filters, requestId: event.target.value })
              }
            />
          </label>
        </CardContent>
      </Card>
      {!filtered.length ? (
        <EmptyState
          title="No matching log events"
          message="The selected filters return no projected events."
        />
      ) : (
        <Card>
          <CardContent className="overflow-x-auto p-0">
            <table className="w-full text-left text-sm">
              <caption className="sr-only">Projected log events</caption>
              <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
                <tr>
                  <th className="px-5 py-3">Time</th>
                  <th className="px-5 py-3">Level</th>
                  <th className="px-5 py-3">Component</th>
                  <th className="px-5 py-3">Tool</th>
                  <th className="px-5 py-3">Summary</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((event, index) => (
                  <tr
                    className="border-b border-border last:border-0"
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
          </CardContent>
        </Card>
      )}
      <p className="text-xs text-muted-foreground">
        {logs.streaming ? "Live stream connected" : "Live stream disconnected"}.
        Events remain in bounded browser memory only.
      </p>
    </div>
  );
}
