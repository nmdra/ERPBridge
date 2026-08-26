import { Link } from "wouter";

import { PageHeader } from "../components/layout/PageHeader";
import { StatusBadge } from "../components/status/StatusBadge";
import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import type { ContextProjection } from "../hooks/useConsole";

function stateTone(
  state: string,
): "success" | "warning" | "danger" | "neutral" {
  const normalized = state.toLowerCase();
  if (
    normalized === "configured" ||
    normalized === "available" ||
    normalized === "ok"
  ) {
    return "success";
  }
  if (normalized.includes("unavailable") || normalized.includes("error")) {
    return "danger";
  }
  if (normalized.includes("degraded") || normalized.includes("unknown")) {
    return "warning";
  }
  return "neutral";
}

function formatUpdated(value?: string) {
  if (!value) return "Not refreshed yet";
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "Refresh time unknown"
    : `Updated ${date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
}

export function Deployments({
  contexts,
  error,
  lastUpdated,
  onRefresh,
  stale = false,
}: {
  contexts: ContextProjection[] | null;
  error: string | null;
  lastUpdated?: string;
  onRefresh?: () => void;
  stale?: boolean;
}) {
  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          onRefresh ? (
            <button
              className="rounded-md border border-border px-3 py-2 text-sm font-medium hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={onRefresh}
              type="button"
            >
              Refresh
            </button>
          ) : null
        }
        description="Choose a configured bridgectl context to scope the read-only console. Context changes stay in this browser session."
        eyebrow="Inventory"
        title="Contexts"
      />
      <p className="text-xs text-muted-foreground" role="status">
        {stale ? "Showing the last valid context snapshot · " : ""}
        {formatUpdated(lastUpdated)}
      </p>
      {error ? (
        <div className="space-y-3">
          <EmptyState title="Contexts are unavailable" message={error} />
          {onRefresh ? (
            <button
              className="rounded-md border border-border px-3 py-2 text-sm font-medium hover:bg-muted"
              onClick={onRefresh}
              type="button"
            >
              Retry
            </button>
          ) : null}
        </div>
      ) : null}
      {!error && !contexts?.length ? (
        <EmptyState
          title="No contexts configured"
          message="Add a bridgectl context to inspect a deployment."
        />
      ) : null}
      {!error && contexts?.length ? (
        <Card>
          <CardContent className="p-0">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
              <div>
                <h2 className="font-semibold">Configured contexts</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  {contexts.length} context{contexts.length === 1 ? "" : "s"}{" "}
                  available to inspect.
                </p>
              </div>
              <Link
                className="text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                href="/"
              >
                Open overview
              </Link>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[42rem] text-left text-sm">
                <caption className="sr-only">
                  Configured ERPBridge contexts
                </caption>
                <thead className="border-b border-border bg-muted/50 text-xs uppercase tracking-wider text-muted-foreground">
                  <tr>
                    <th className="px-5 py-3">Context</th>
                    <th className="px-5 py-3">Server</th>
                    <th className="px-5 py-3">MCP server</th>
                    <th className="px-5 py-3">Selection</th>
                  </tr>
                </thead>
                <tbody>
                  {contexts.map((context) => (
                    <tr
                      className="border-b border-border last:border-0 hover:bg-muted/30"
                      key={context.name}
                    >
                      <th className="px-5 py-4 font-medium">{context.name}</th>
                      <td className="px-5 py-4">
                        <StatusBadge
                          label={context.serverState}
                          tone={stateTone(context.serverState)}
                        />
                      </td>
                      <td className="px-5 py-4">
                        <StatusBadge
                          label={context.mcpServerState}
                          tone={stateTone(context.mcpServerState)}
                        />
                      </td>
                      <td className="px-5 py-4">
                        {context.current ? (
                          <StatusBadge label="Current" tone="info" />
                        ) : (
                          <span className="text-muted-foreground">
                            Available
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
