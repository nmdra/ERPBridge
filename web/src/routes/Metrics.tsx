import { Card, CardContent, CardHeader } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { Skeleton } from "../components/ui/skeleton";
import { useMetrics } from "../hooks/useObservability";

export function Metrics({ contextName }: { contextName: string }) {
  const metrics = useMetrics(contextName);
  if (metrics.loading) return <Skeleton className="h-48 w-full" />;
  if (!metrics.data || metrics.error) {
    return (
      <EmptyState
        title="Metrics are unavailable"
        message={
          metrics.error ?? "The selected context has no live metric snapshot."
        }
      />
    );
  }
  const snapshot = metrics.data;
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <h2 className="font-medium">Live metrics</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Cumulative totals and current-session rates. Window starts{" "}
            {snapshot.sampleWindowStart ?? "now"}.
          </p>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-3">
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Cumulative samples
            </p>
            <p className="mt-1 text-2xl font-semibold">
              {snapshot.cumulative.length}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Session rates
            </p>
            <p className="mt-1 text-2xl font-semibold">
              {snapshot.rates.length}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Average latency
            </p>
            <p className="mt-1 text-2xl font-semibold">
              {snapshot.averageLatencySeconds === undefined
                ? "—"
                : `${snapshot.averageLatencySeconds.toFixed(3)}s`}
            </p>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <h2 className="font-medium">Metric table</h2>
        </CardHeader>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">Live metric samples</caption>
            <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
              <tr>
                <th className="px-5 py-3">Metric</th>
                <th className="px-5 py-3">Value</th>
                <th className="px-5 py-3">Current rate</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.cumulative.map((sample, index) => {
                const rate = snapshot.rates.find(
                  (item) => item.name === sample.name,
                );
                return (
                  <tr
                    className="border-b border-border last:border-0"
                    key={`${sample.name}-${index}`}
                  >
                    <th className="px-5 py-3 font-medium">{sample.name}</th>
                    <td className="px-5 py-3">{sample.value}</td>
                    <td className="px-5 py-3">
                      {rate ? `${rate.perSecond.toFixed(2)}/s` : "—"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </CardContent>
      </Card>
      <p className="text-xs text-muted-foreground">
        Historical Prometheus queries are not available in the local console.
      </p>
    </div>
  );
}
