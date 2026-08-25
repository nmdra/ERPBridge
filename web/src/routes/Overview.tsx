import { Activity, Database, Gauge, Server, Wrench } from "lucide-react";
import { Link } from "wouter";

import { PageHeader } from "../components/layout/PageHeader";
import { StatusBadge } from "../components/status/StatusBadge";
import { Card, CardContent } from "../components/ui/card";
import { MetricCard } from "../components/ui/metric-card";
import { Skeleton } from "../components/ui/skeleton";
import { StateBanner } from "../components/ui/state-banner";
import {
  useCache,
  useDeployment,
  useHealth,
  useServerInfo,
} from "../hooks/useConsole";
import { useMetrics, useTools } from "../hooks/useObservability";

function healthTone(state: string): "success" | "warning" | "danger" | "info" {
  if (state === "healthy") return "success";
  if (state === "degraded") return "warning";
  if (state === "unavailable") return "danger";
  return "info";
}

function formatObservedAt(value?: string) {
  if (!value) return "Not observed yet";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Unknown time";
  return `Updated ${date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
}

const linkClassName =
  "font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export function Overview({ contextName }: { contextName: string }) {
  const deployment = useDeployment(contextName);
  const health = useHealth(contextName);
  const serverInfo = useServerInfo(contextName);
  const cache = useCache(contextName);
  const tools = useTools(contextName);
  const metrics = useMetrics(contextName);

  if (deployment.loading) {
    return (
      <div className="space-y-6" aria-busy="true">
        <PageHeader
          description="A read-only operational summary for the selected context."
          eyebrow="Monitor"
          title="Overview"
        />
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
        </div>
      </div>
    );
  }

  if (deployment.error || !deployment.data) {
    return (
      <div className="space-y-6">
        <PageHeader
          description="A read-only operational summary for the selected context."
          eyebrow="Monitor"
          title="Overview"
        />
        <StateBanner
          message={deployment.error ?? "The selected context is unavailable."}
          title="Context data is unavailable"
          tone="danger"
        />
      </div>
    );
  }

  const healthState = health.data?.state ?? "unavailable";
  const activeTools =
    serverInfo.data?.activeToolCount ??
    tools.data?.items.filter((tool) => tool.active).length;
  const requestRate = (metrics.data?.rates ?? [])
    .filter((sample) => sample.name === "erp_requests_total")
    .reduce((total, sample) => total + sample.perSecond, 0);
  const toolRate = (metrics.data?.rates ?? [])
    .filter((sample) => sample.name === "mcp_tool_invocations_total")
    .reduce((total, sample) => total + sample.perSecond, 0);
  const hasStaleData = Boolean(health.stale || cache.stale || metrics.stale);

  return (
    <div className="space-y-6">
      <PageHeader
        actions={<StatusBadge label="Read-only monitoring" tone="info" />}
        description={`Monitor ${deployment.data.context.name} without changing its persistent bridgectl configuration.`}
        eyebrow="Monitor"
        title="Overview"
      />

      <StateBanner
        message={
          health.data?.status === "ok"
            ? `${formatObservedAt(health.lastUpdated)} · The selected context is responding normally.`
            : (health.error ?? "Health data is not available for this context.")
        }
        title={`Context health: ${health.data?.status ?? healthState}`}
        tone={healthTone(healthState)}
      />
      {hasStaleData ? (
        <StateBanner
          message="One or more live sources could not be refreshed. Values shown below are the last safe observations."
          title="Some dashboard data is stale"
          tone="warning"
        />
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          detail={formatObservedAt(health.lastUpdated)}
          icon={<Activity size={17} />}
          label="Health"
          tone={healthTone(healthState)}
          value={health.data?.status ?? healthState}
        />
        <MetricCard
          detail={`${tools.data?.items.length ?? 0} projected in inventory`}
          icon={<Wrench size={17} />}
          label="Active tools"
          value={activeTools ?? "—"}
        />
        <MetricCard
          detail={
            cache.data?.stats?.redisMemory ?? "Read-only cache projection"
          }
          icon={<Database size={17} />}
          label="Cache keys"
          tone="info"
          value={cache.data?.stats?.exactKeys ?? "—"}
        />
        <MetricCard
          detail={serverInfo.data?.cacheBackend ?? "Backend not reported"}
          icon={<Server size={17} />}
          label="Server version"
          value={serverInfo.data?.version ?? "Unavailable"}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.3fr_0.7fr]">
        <Card>
          <CardContent>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold">Operational signals</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Current-session rates from the safe metrics projection.
                </p>
              </div>
              <Link className={linkClassName} href="/metrics">
                View metrics
              </Link>
            </div>
            <div className="mt-5 grid gap-4 sm:grid-cols-3">
              <div className="rounded-lg bg-muted/60 p-4">
                <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  ERP requests
                </p>
                <p className="mt-2 text-xl font-semibold">
                  {requestRate ? `${requestRate.toFixed(2)}/s` : "—"}
                </p>
              </div>
              <div className="rounded-lg bg-muted/60 p-4">
                <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Tool invocations
                </p>
                <p className="mt-2 text-xl font-semibold">
                  {toolRate ? `${toolRate.toFixed(2)}/s` : "—"}
                </p>
              </div>
              <div className="rounded-lg bg-muted/60 p-4">
                <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Average latency
                </p>
                <p className="mt-2 text-xl font-semibold">
                  {metrics.data?.averageLatencySeconds === undefined
                    ? "—"
                    : `${metrics.data.averageLatencySeconds.toFixed(3)}s`}
                </p>
              </div>
            </div>
            <p className="mt-4 text-xs text-muted-foreground">
              Metrics are scoped to this browser session and do not represent
              historical Prometheus data.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <div className="flex items-center gap-3">
              <Gauge aria-hidden="true" className="text-primary" size={20} />
              <div>
                <h2 className="text-lg font-semibold">Investigate</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Start with a safe read-only surface.
                </p>
              </div>
            </div>
            <div className="mt-5 grid gap-2 text-sm">
              <Link className={linkClassName} href="/logs">
                Review recent logs →
              </Link>
              <Link className={linkClassName} href="/tools">
                Browse MCP tools →
              </Link>
              <Link className={linkClassName} href="/topology">
                Inspect integration topology →
              </Link>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
