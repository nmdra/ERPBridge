import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { Route, Switch } from "wouter";

import { AppShell } from "./components/layout/AppShell";
import { useContexts } from "./hooks/useConsole";
import { PageHeader } from "./components/layout/PageHeader";
import { StateBanner } from "./components/ui/state-banner";
import { Deployments } from "./routes/Deployments";
import { Logs } from "./routes/Logs";
import { Skeleton } from "./components/ui/skeleton";
import { Overview } from "./routes/Overview";
import { Placeholder } from "./routes/Placeholder";
import { PluginDetails, Plugins } from "./routes/Plugins";
import { Settings } from "./routes/Settings";
import { Topology } from "./routes/Topology";
import { ThemeProvider } from "./theme/ThemeProvider";
import { ToolDetails, Tools } from "./routes/Tools";

const selectedContextStorageKey = "erpbridge-console-selected-context";

function readStoredContext() {
  try {
    return window.sessionStorage.getItem(selectedContextStorageKey) ?? "";
  } catch {
    return "";
  }
}

function storeSelectedContext(name: string) {
  try {
    window.sessionStorage.setItem(selectedContextStorageKey, name);
  } catch {
    // Storage is optional; the in-memory selection remains usable.
  }
}

const Metrics = lazy(() =>
  import("./routes/Metrics").then((module) => ({ default: module.Metrics })),
);

function ConsoleApp() {
  const contexts = useContexts();
  const [selectedContext, setSelectedContext] = useState(readStoredContext);

  useEffect(() => {
    if (!contexts.data?.length) return;
    const selectedStillExists = contexts.data.some(
      (context) => context.name === selectedContext,
    );
    if (!selectedContext || !selectedStillExists) {
      const current = contexts.data.find((context) => context.current);
      const nextContext = current?.name ?? contexts.data[0].name;
      setSelectedContext(nextContext);
      storeSelectedContext(nextContext);
    }
  }, [contexts.data, selectedContext]);

  const handleContextChange = useCallback((name: string) => {
    setSelectedContext(name);
    storeSelectedContext(name);
  }, []);

  return (
    <AppShell
      contexts={contexts.data}
      onContextChange={handleContextChange}
      onRefresh={contexts.refresh}
      selectedContext={selectedContext}
    >
      <div className="mx-auto max-w-7xl space-y-6">
        {!selectedContext ? (
          <div className="space-y-6">
            <PageHeader
              description="Select a configured bridgectl context to begin read-only monitoring."
              eyebrow="Monitor"
              title="Overview"
            />
            <StateBanner
              message={
                contexts.loading
                  ? "Loading configured contexts. No upstream data is requested until a context is selected."
                  : (contexts.error ??
                    "Add a bridgectl context to begin read-only monitoring.")
              }
              title={
                contexts.loading
                  ? "Loading contexts"
                  : contexts.error
                    ? "Contexts are unavailable"
                    : "No contexts configured"
              }
              tone={contexts.error ? "danger" : "info"}
            />
          </div>
        ) : (
          <Switch>
            <Route path="/">
              <Overview contextName={selectedContext} />
            </Route>
            <Route path="/contexts">
              <Deployments
                contexts={contexts.data}
                error={contexts.error}
                lastUpdated={contexts.lastUpdated}
                onRefresh={contexts.refresh}
                stale={contexts.stale}
              />
            </Route>
            <Route path="/deployments">
              <Deployments
                contexts={contexts.data}
                error={contexts.error}
                lastUpdated={contexts.lastUpdated}
                onRefresh={contexts.refresh}
                stale={contexts.stale}
              />
            </Route>
            <Route path="/logs">
              <Logs contextName={selectedContext} />
            </Route>
            <Route path="/metrics">
              <Suspense fallback={<Skeleton className="h-48 w-full" />}>
                <Metrics contextName={selectedContext} />
              </Suspense>
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
            <Route path="/plugins/:pluginName/:pluginVersion">
              {(params) => (
                <PluginDetails
                  contextName={selectedContext}
                  pluginName={params.pluginName ?? ""}
                  pluginVersion={params.pluginVersion ?? ""}
                />
              )}
            </Route>
            <Route path="/plugins">
              <Plugins contextName={selectedContext} />
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
        )}
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
