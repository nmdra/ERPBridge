import { useMemo, useState } from "react";
import { Link } from "wouter";

import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { FilterToolbar } from "../components/ui/filter-toolbar";
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

function decodePluginPart(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function pluginHref(plugin: Pick<PluginProjection, "name" | "version">) {
  return `/plugins/${encodeURIComponent(plugin.name)}/${encodeURIComponent(plugin.version)}`;
}

const linkClassName =
  "text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

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
                  <div>
                    <Link className={linkClassName} href={pluginHref(plugin)}>
                      {plugin.name}
                    </Link>
                  </div>
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
                  <Link
                    className={linkClassName}
                    href={pluginHref(binding.pluginRef)}
                  >
                    {binding.pluginRef.name}@{binding.pluginRef.version}
                  </Link>
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

export function PluginDetails({
  contextName,
  pluginName,
  pluginVersion,
}: {
  contextName: string;
  pluginName: string;
  pluginVersion: string;
}) {
  const plugins = usePlugins(contextName);
  const bindings = usePluginBindings(contextName);
  const name = decodePluginPart(pluginName);
  const version = decodePluginPart(pluginVersion);

  if (plugins.loading) return <Skeleton className="h-64 w-full" />;
  if (plugins.error || !plugins.data || plugins.data.state !== "available") {
    return (
      <div className="space-y-4">
        <Link className="text-sm text-primary hover:underline" href="/plugins">
          ← Back to plugins
        </Link>
        <EmptyState
          title="Plugin details are unavailable"
          message={
            plugins.error ??
            "The selected context has no readable plugin inventory."
          }
        />
      </div>
    );
  }

  const plugin = plugins.data.items.find(
    (item) => item.name === name && item.version === version,
  );
  if (!plugin) {
    return (
      <div className="space-y-4">
        <Link className="text-sm text-primary hover:underline" href="/plugins">
          ← Back to plugins
        </Link>
        <EmptyState
          title="Plugin not found"
          message={`No plugin named ${name}@${version} exists in the selected context.`}
        />
      </div>
    );
  }

  const pluginBindings =
    bindings.data?.state === "available"
      ? bindings.data.items.filter(
          (binding) =>
            binding.pluginRef.name === plugin.name &&
            binding.pluginRef.version === plugin.version,
        )
      : [];

  return (
    <div className="space-y-6">
      <Link
        className="text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        href="/plugins"
      >
        ← Back to plugins
      </Link>
      <PageHeader
        actions={
          <StatusBadge
            label={activeStatus(plugin.active)}
            tone={plugin.active ? "success" : "neutral"}
          />
        }
        description={`Version ${plugin.version}${plugin.type ? ` · ${plugin.type}` : ""}`}
        eyebrow="External plugin"
        title={plugin.name}
      />
      <Card>
        <CardContent>
          <h2 className="font-medium">Plugin details</h2>
          <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <div>
              <dt className="text-muted-foreground">Deployment type</dt>
              <dd>{plugin.type ?? "api"}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Endpoint</dt>
              <dd>
                {plugin.endpointConfigured ? "Configured" : "Not configured"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Timeout</dt>
              <dd>
                {plugin.timeoutMilliseconds > 0
                  ? `${plugin.timeoutMilliseconds} ms`
                  : "Default"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Configuration</dt>
              <dd>{plugin.configurationPresent ? "Present" : "None"}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Health</dt>
              <dd>Unknown</dd>
            </div>
          </dl>
          <p className="mt-4 text-xs text-muted-foreground">
            Plugin endpoints, credentials, and raw configuration are never shown
            in the console.
          </p>
        </CardContent>
      </Card>
      <div>
        <h2 className="text-lg font-semibold">Bindings</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Exact tool versions that reference this plugin.
        </p>
      </div>
      {bindings.loading ? (
        <Skeleton className="h-40 w-full" />
      ) : bindings.data?.state === "available" && pluginBindings.length ? (
        <BindingTable bindings={pluginBindings} />
      ) : (
        <EmptyState
          title="No plugin bindings registered"
          message={
            bindings.error ??
            "No read-only plugin bindings reference this plugin version."
          }
        />
      )}
    </div>
  );
}

export function Plugins({ contextName }: { contextName: string }) {
  const plugins = usePlugins(contextName);
  const bindings = usePluginBindings(contextName);
  const [filter, setFilter] = useState("");

  const visiblePlugins = useMemo(() => {
    const query = filter.toLowerCase().trim();
    return (plugins.data?.items ?? []).filter((plugin) =>
      `${plugin.name} ${plugin.version} ${plugin.type ?? ""}`
        .toLowerCase()
        .includes(query),
    );
  }, [filter, plugins.data?.items]);
  const visibleBindings = useMemo(() => {
    const query = filter.toLowerCase().trim();
    return (bindings.data?.items ?? []).filter((binding) =>
      `${binding.name} ${binding.pluginRef.name} ${binding.pluginRef.version} ${binding.toolRef.name} ${binding.phase}`
        .toLowerCase()
        .includes(query),
    );
  }, [bindings.data?.items, filter]);

  if (plugins.loading || bindings.loading) {
    return (
      <div className="space-y-6" aria-busy="true">
        <PageHeader
          description="Read-only plugin and binding metadata."
          eyebrow="Inventory"
          title="Plugins"
        />
        <Skeleton className="h-64 w-full" />
      </div>
    );
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
      <PageHeader
        description="Read-only plugin and binding metadata. ERPBridge does not deploy plugin processes, expose their endpoints, or show their credentials."
        eyebrow="Inventory"
        title="Plugins"
      />
      <FilterToolbar
        summary={`${visiblePlugins.length} plugins and ${visibleBindings.length} bindings match the current search.`}
      >
        <label className="text-sm sm:col-span-2 lg:col-span-4">
          <span className="font-medium">Search plugins and bindings</span>
          <input
            className="mt-1 h-10 w-full rounded-lg border border-border bg-card px-3"
            onChange={(event) => setFilter(event.target.value)}
            placeholder="Name, version, tool, phase, or type"
            value={filter}
          />
        </label>
      </FilterToolbar>
      {plugins.data?.state === "available" && visiblePlugins.length ? (
        <PluginTable plugins={visiblePlugins} />
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
      {bindings.data?.state === "available" && visibleBindings.length ? (
        <BindingTable bindings={visibleBindings} />
      ) : (
        <EmptyState
          title="No plugin bindings registered"
          message="No read-only plugin bindings are available for this deployment."
        />
      )}
    </div>
  );
}
