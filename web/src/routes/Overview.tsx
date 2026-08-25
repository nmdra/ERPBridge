import { AlertCircle, RefreshCw } from "lucide-react";

import { useDeployment, useServerInfo } from "../hooks/useConsole";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { Skeleton } from "../components/ui/skeleton";

export function Overview({ contextName }: { contextName: string }) {
  const deployment = useDeployment(contextName);
  const serverInfo = useServerInfo(contextName);
  if (deployment.loading) {
    return <Skeleton className="h-32 w-full" />;
  }
  if (deployment.error || !deployment.data) {
    return (
      <Card>
        <CardContent className="flex items-center gap-3 py-8">
          <AlertCircle
            aria-hidden="true"
            className="text-destructive"
            size={20}
          />
          <div>
            <h2 className="font-medium">Deployment is unavailable</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {deployment.error}
            </p>
          </div>
          <Button
            className="ml-auto"
            variant="secondary"
            onClick={() => window.location.reload()}
          >
            <RefreshCw aria-hidden="true" size={15} />
            Refresh
          </Button>
        </CardContent>
      </Card>
    );
  }
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="grid gap-4 py-6 sm:grid-cols-3">
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Context
            </p>
            <p className="mt-1 font-medium">{deployment.data.context.name}</p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Console
            </p>
            <p className="mt-1 font-medium">{deployment.data.console.state}</p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Server configuration
            </p>
            <p className="mt-1 font-medium">
              {deployment.data.context.serverState}
            </p>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="grid gap-4 py-6 sm:grid-cols-3">
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Version
            </p>
            <p className="mt-1 font-medium">
              {serverInfo.data?.version ?? "Unavailable"}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Cache backend
            </p>
            <p className="mt-1 font-medium">
              {serverInfo.data?.cacheBackend ?? "Unavailable"}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">
              Active tools
            </p>
            <p className="mt-1 font-medium">
              {serverInfo.data?.activeToolCount ?? "—"}
            </p>
          </div>
        </CardContent>
      </Card>
      <EmptyState
        title="Operational data is loading"
        message="Health, tools, logs, and metrics appear here after their read-only adapters are available."
      />
    </div>
  );
}
