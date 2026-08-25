import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { Skeleton } from "../components/ui/skeleton";
import { StatusBadge } from "../components/status/StatusBadge";
import {
  usePluginBindings,
  usePlugins,
  type PluginBindingProjection,
  type PluginProjection,
} from "../hooks/useObservability";

function activeStatus(active: boolean) {
  return active ? "Active" : "Inactive";
}

function PluginTable({ plugins }: { plugins: PluginProjection[] }) {
  return (
    <Card>
      <CardContent className="overflow-x-auto p-0">
        <table className="w-full text-left text-sm">
          <caption className="sr-only">External plugins</caption>
          <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
            <tr>
              <th className="px-5 py-3">Plugin</th>
              <th className="px-5 py-3">Status</th>
              <th className="px-5 py-3">Endpoint</th>
              <th className="px-5 py-3">Timeout</th>
              <th className="px-5 py-3">Configuration</th>
            </tr>
          </thead>
          <tbody>
            {plugins.map((plugin) => (
              <tr
                className="border-b border-border last:border-0"
                key={`${plugin.name}@${plugin.version}`}
              >
                <th className="px-5 py-3 font-medium">
                  <div>{plugin.name}</div>
                  <div className="text-xs font-normal text-muted-foreground">
                    version {plugin.version}
                    {plugin.type ? ` · ${plugin.type}` : ""}
                  </div>
                </th>
                <td className="px-5 py-3">
                  <StatusBadge
                    label={activeStatus(plugin.active)}
                    tone={plugin.active ? "success" : "neutral"}
                  />
                </td>
                <td className="px-5 py-3">
                  {plugin.endpointConfigured ? "Configured" : "Not configured"}
                </td>
                <td className="px-5 py-3">
                  {plugin.timeoutMilliseconds > 0
                    ? `${plugin.timeoutMilliseconds} ms`
                    : "Default"}
                </td>
                <td className="px-5 py-3">
                  {plugin.configurationPresent ? "Present" : "None"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}

function BindingTable({ bindings }: { bindings: PluginBindingProjection[] }) {
  return (
    <Card>
      <CardContent className="overflow-x-auto p-0">
        <table className="w-full text-left text-sm">
          <caption className="sr-only">External plugin bindings</caption>
          <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
            <tr>
              <th className="px-5 py-3">Binding</th>
              <th className="px-5 py-3">Plugin</th>
              <th className="px-5 py-3">Tool</th>
              <th className="px-5 py-3">Execution</th>
              <th className="px-5 py-3">Configuration</th>
            </tr>
          </thead>
          <tbody>
            {bindings.map((binding) => (
              <tr
                className="border-b border-border last:border-0"
                key={binding.name}
              >
                <th className="px-5 py-3 font-medium">
                  <div>{binding.name}</div>
                  <div className="mt-1">
                    <StatusBadge
                      label={activeStatus(binding.active)}
                      tone={binding.active ? "success" : "neutral"}
                    />
                  </div>
                </th>
                <td className="px-5 py-3">
                  {binding.pluginRef.name}@{binding.pluginRef.version}
                </td>
                <td className="px-5 py-3">
                  {binding.toolRef.name}@{binding.toolRef.version}
                </td>
                <td className="px-5 py-3">
                  <div>{binding.phase}</div>
                  <div className="text-xs text-muted-foreground">
                    Priority {binding.priority} · {binding.failurePolicy}
                  </div>
                </td>
                <td className="px-5 py-3">
                  {binding.configurationPresent ? "Present" : "None"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}

export function Plugins({ contextName }: { contextName: string }) {
  const plugins = usePlugins(contextName);
  const bindings = usePluginBindings(contextName);

  if (plugins.loading || bindings.loading) {
    return <Skeleton className="h-64 w-full" />;
  }

  if (
    (plugins.data?.state !== "available" && !plugins.data?.items.length) ||
    (bindings.data?.state !== "available" && !bindings.data?.items.length)
  ) {
    return (
      <EmptyState
        title="Plugin metadata is unavailable"
        message="This deployment does not expose the read-only plugin control-plane API. Plugin UI requires the generic external-plugin contract."
      />
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm text-muted-foreground">External extensions</p>
        <h2 className="mt-1 text-2xl font-semibold tracking-tight">Plugins</h2>
        <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
          Read-only plugin and binding metadata. ERPBridge does not deploy
          plugin processes, expose their endpoints, or show their credentials.
        </p>
      </div>
      {plugins.data?.state === "available" && plugins.data.items.length ? (
        <PluginTable plugins={plugins.data.items} />
      ) : (
        <EmptyState
          title="No plugins registered"
          message="No plugin resources are active or stored for this deployment."
        />
      )}
      <div>
        <h3 className="text-lg font-semibold">Bindings</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Exact plugin and tool versions run in the documented after-response
          phase.
        </p>
      </div>
      {bindings.data?.state === "available" && bindings.data.items.length ? (
        <BindingTable bindings={bindings.data.items} />
      ) : (
        <EmptyState
          title="No plugin bindings registered"
          message="No read-only plugin bindings are available for this deployment."
        />
      )}
    </div>
  );
}
