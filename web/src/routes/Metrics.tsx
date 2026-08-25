import { Activity, Gauge, Layers, Timer } from "lucide-react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent, CardHeader } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { MetricCard } from "../components/ui/metric-card";
import { Skeleton } from "../components/ui/skeleton";
import { StateBanner } from "../components/ui/state-banner";
import {
  metricSeriesKey,
  useMetrics,
  type MetricSample,
  type MetricsHistoryPoint,
} from "../hooks/useObservability";

function formatNumber(value: number) {
  return Number.isInteger(value) ? value.toLocaleString() : value.toFixed(3);
}

function formatRate(value: number | undefined) {
  return value === undefined ? "—" : `${value.toFixed(2)}/s`;
}

function labelsText(labels?: Record<string, string>) {
  if (!labels || !Object.keys(labels).length) return "No labels";
  return Object.entries(labels)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${value}`)
    .join(", ");
}

function sumRate(
  rates: MetricsHistoryPoint["rates"],
  names: string[],
): number | undefined {
  const matching = rates.filter((rate) => names.includes(rate.name));
  if (!matching.length) return undefined;
  return matching.reduce((total, rate) => total + rate.perSecond, 0);
}

function currentValue(
  samples: MetricSample[],
  names: string[],
): number | undefined {
  const matching = samples.filter((sample) => names.includes(sample.name));
  if (!matching.length) return undefined;
  return matching.reduce((total, sample) => total + sample.value, 0);
}

function trendData(history: MetricsHistoryPoint[]) {
  return history.map((point) => ({
    label: new Date(point.observedAt).toLocaleTimeString([], {
      minute: "2-digit",
      second: "2-digit",
    }),
    requests: sumRate(point.rates, ["erp_requests_total"]) ?? 0,
    invocations: sumRate(point.rates, ["mcp_tool_invocations_total"]) ?? 0,
  }));
}

function MetricTrend({ history }: { history: MetricsHistoryPoint[] }) {
  const data = trendData(history);
  const latest = data.at(-1);
  return (
    <Card>
      <CardHeader>
        <h2 className="text-lg font-semibold">Session activity</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Rates observed by this browser. The chart is intentionally not
          historical storage.
        </p>
      </CardHeader>
      <CardContent>
        <figure aria-labelledby="metrics-trend-title" className="min-w-0">
          <figcaption className="sr-only" id="metrics-trend-title">
            Session rates for ERP requests and MCP tool invocations
          </figcaption>
          <div
            aria-hidden="true"
            className="h-64 w-full rounded-lg bg-muted/40 p-2"
          >
            {data.length > 1 ? (
              <ResponsiveContainer height="100%" width="100%">
                <LineChart
                  data={data}
                  margin={{ top: 8, right: 12, left: -20, bottom: 0 }}
                >
                  <CartesianGrid
                    stroke="hsl(var(--border))"
                    strokeDasharray="3 3"
                  />
                  <XAxis
                    dataKey="label"
                    fontSize={11}
                    tick={{ fill: "hsl(var(--muted-foreground))" }}
                  />
                  <YAxis
                    fontSize={11}
                    tick={{ fill: "hsl(var(--muted-foreground))" }}
                  />
                  <Tooltip
                    contentStyle={{
                      background: "hsl(var(--card))",
                      borderColor: "hsl(var(--border))",
                      borderRadius: "0.5rem",
                      color: "hsl(var(--foreground))",
                    }}
                  />
                  <Line
                    dataKey="requests"
                    dot={false}
                    name="ERP requests/s"
                    stroke="hsl(var(--chart-1))"
                    strokeWidth={2}
                    type="monotone"
                  />
                  <Line
                    dataKey="invocations"
                    dot={false}
                    name="Tool invocations/s"
                    stroke="hsl(var(--chart-2))"
                    strokeWidth={2}
                    type="monotone"
                  />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex h-full items-center justify-center text-center text-sm text-muted-foreground">
                Collecting a second sample for the session trend…
              </div>
            )}
          </div>
          <p className="mt-3 text-sm text-muted-foreground">
            {latest
              ? `Latest observation: ${latest.requests.toFixed(2)} ERP requests/s and ${latest.invocations.toFixed(2)} tool invocations/s.`
              : "No rate observations are available yet."}
          </p>
        </figure>
      </CardContent>
    </Card>
  );
}

export function Metrics({ contextName }: { contextName: string }) {
  const metrics = useMetrics(contextName);
  if (metrics.loading && !metrics.data) {
    return (
      <div className="space-y-6" aria-busy="true">
        <PageHeader
          description="Current-session rates and safe cumulative metric projections."
          eyebrow="Monitor"
          title="Metrics"
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
  if (!metrics.data || metrics.data.state !== "available") {
    return (
      <div className="space-y-6">
        <PageHeader
          description="Current-session rates and safe cumulative metric projections."
          eyebrow="Monitor"
          title="Metrics"
        />
        <EmptyState
          title="Metrics are unavailable"
          message={
            metrics.error ?? "The selected context has no live metric snapshot."
          }
        />
      </div>
    );
  }

  const snapshot = metrics.data;
  const requestRate = sumRate(snapshot.rates, ["erp_requests_total"]);
  const invocationRate = sumRate(snapshot.rates, [
    "mcp_tool_invocations_total",
  ]);
  const cacheHits = currentValue(snapshot.cumulative, ["cache_hits_total"]);
  const ratesBySeries = new Map(
    snapshot.rates.map((rate) => [
      metricSeriesKey(rate.name, rate.labels),
      rate,
    ]),
  );

  return (
    <div className="space-y-6">
      <PageHeader
        description="Live metrics are bounded to the selected context. Rates are derived from successive observations in this console session."
        eyebrow="Monitor"
        title="Metrics"
      />
      {metrics.stale ? (
        <StateBanner
          message="The last refresh failed. The values below are retained from the most recent successful observation."
          title="Showing stale metrics"
          tone="warning"
        />
      ) : null}
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="text-sm font-medium">Observation window</p>
            <p className="mt-1 text-sm text-muted-foreground">
              {snapshot.sampleWindowStart
                ? `Started ${new Date(snapshot.sampleWindowStart).toLocaleString()}`
                : "Starts with the first successful sample"}
            </p>
          </div>
          <p className="text-sm text-muted-foreground">
            Last updated{" "}
            {metrics.lastUpdated
              ? new Date(metrics.lastUpdated).toLocaleTimeString()
              : "not yet"}
          </p>
        </CardContent>
      </Card>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          detail="Derived from ERP request counters"
          icon={<Activity size={17} />}
          label="ERP request rate"
          value={formatRate(requestRate)}
        />
        <MetricCard
          detail="Derived from MCP invocation counters"
          icon={<Gauge size={17} />}
          label="Tool invocation rate"
          tone="info"
          value={formatRate(invocationRate)}
        />
        <MetricCard
          detail="Current cumulative cache hits"
          icon={<Layers size={17} />}
          label="Cache hits"
          tone="success"
          value={cacheHits === undefined ? "—" : formatNumber(cacheHits)}
        />
        <MetricCard
          detail="Histogram-derived average"
          icon={<Timer size={17} />}
          label="Average latency"
          tone="warning"
          value={
            snapshot.averageLatencySeconds === undefined
              ? "—"
              : `${snapshot.averageLatencySeconds.toFixed(3)}s`
          }
        />
      </div>

      <MetricTrend history={metrics.history} />

      <Card>
        <CardHeader>
          <h2 className="text-lg font-semibold">Metric table</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Every row keeps its complete label set. Rates are matched by metric
            name and labels.
          </p>
        </CardHeader>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[42rem] text-left text-sm">
            <caption className="sr-only">Live metric samples</caption>
            <thead className="border-b border-border bg-muted/50 text-xs uppercase tracking-wider text-muted-foreground">
              <tr>
                <th className="px-5 py-3">Metric</th>
                <th className="px-5 py-3">Labels</th>
                <th className="px-5 py-3">Value</th>
                <th className="px-5 py-3">Current rate</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.cumulative.map((sample, index) => {
                const rate = ratesBySeries.get(
                  metricSeriesKey(sample.name, sample.labels),
                );
                return (
                  <tr
                    className="border-b border-border last:border-0 hover:bg-muted/30"
                    key={`${metricSeriesKey(sample.name, sample.labels)}-${index}`}
                  >
                    <th className="px-5 py-3 font-medium">{sample.name}</th>
                    <td className="px-5 py-3 text-muted-foreground">
                      {labelsText(sample.labels)}
                    </td>
                    <td className="px-5 py-3">{formatNumber(sample.value)}</td>
                    <td className="px-5 py-3">
                      {rate ? formatRate(rate.perSecond) : "—"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </CardContent>
      </Card>
      <p className="text-xs text-muted-foreground">
        Historical Prometheus queries and percentile latency are not available
        in the local console.
      </p>
    </div>
  );
}
