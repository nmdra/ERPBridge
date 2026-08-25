import { useEffect, useState } from "react";
import { Route, Switch } from "wouter";

import { AppShell } from "./components/layout/AppShell";
import { Card, CardContent } from "./components/ui/card";
import { StatusBadge } from "./components/status/StatusBadge";
import { useContexts } from "./hooks/useConsole";
import { Deployments } from "./routes/Deployments";
import { Logs } from "./routes/Logs";
import { Metrics } from "./routes/Metrics";
import { Overview } from "./routes/Overview";
import { Placeholder } from "./routes/Placeholder";
import { Settings } from "./routes/Settings";
import { Topology } from "./routes/Topology";
import { ThemeProvider } from "./theme/ThemeProvider";
import { ToolDetails, Tools } from "./routes/Tools";

function ConsoleApp() {
  const contexts = useContexts();
  const [selectedContext, setSelectedContext] = useState("local");

  useEffect(() => {
    const current = contexts.data?.find((context) => context.current);
    if (
      current &&
      !contexts.data?.some((context) => context.name === selectedContext)
    ) {
      setSelectedContext(current.name);
    }
  }, [contexts.data, selectedContext]);

  return (
    <AppShell
      contexts={contexts.data}
      onContextChange={setSelectedContext}
      selectedContext={selectedContext}
    >
      <div className="mx-auto max-w-7xl space-y-6">
        <Switch>
          <Route path="/">
            <div>
              <p className="text-sm text-muted-foreground">
                Deployment overview
              </p>
              <h2 className="mt-1 text-2xl font-semibold tracking-tight">
                ERPBridge Console
              </h2>
              <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
                Monitor configured deployments without changing their persistent
                CLI context.
              </p>
              <div className="mt-3">
                <StatusBadge label="Read-only monitoring" tone="info" />
              </div>
              <Card className="mt-5 border-primary/30 bg-primary/5" role="note">
                <CardContent className="py-4">
                  <p className="text-sm font-medium">ERPBridge Console</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    ERPBridge Console is the read-only solution for monitoring
                    the ERPBridge server. Use{" "}
                    <code className="rounded bg-muted px-1 py-0.5 text-xs">
                      bridgectl
                    </code>{" "}
                    to configure and modify ERPBridge.
                  </p>
                </CardContent>
              </Card>
            </div>
            <Overview contextName={selectedContext} />
          </Route>
          <Route path="/deployments">
            <Deployments contexts={contexts.data} error={contexts.error} />
          </Route>
          <Route path="/logs">
            <Logs contextName={selectedContext} />
          </Route>
          <Route path="/metrics">
            <Metrics contextName={selectedContext} />
          </Route>
          <Route path="/tools/:toolName">
            {(params) => (
              <ToolDetails
                contextName={selectedContext}
                toolName={params.toolName ?? ""}
              />
            )}
          </Route>
          <Route path="/tools">
            <Tools contextName={selectedContext} />
          </Route>
          <Route path="/topology">
            <Topology contextName={selectedContext} />
          </Route>
          <Route path="/settings">
            <Settings />
          </Route>
          <Route>
            <Placeholder
              title="Page not found"
              message="The selected console page does not exist."
            />
          </Route>
        </Switch>
      </div>
    </AppShell>
  );
}

export function App() {
  return (
    <ThemeProvider>
      <ConsoleApp />
    </ThemeProvider>
  );
}
