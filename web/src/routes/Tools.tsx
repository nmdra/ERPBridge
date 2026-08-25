import { useMemo, useState } from "react";
import { Link } from "wouter";

import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { Skeleton } from "../components/ui/skeleton";
import { StatusBadge } from "../components/status/StatusBadge";
import {
  usePluginBindings,
  useTools,
  type ToolManifest,
  type ToolProjection,
} from "../hooks/useObservability";

function decodeToolName(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function GuidanceList({ title, items }: { title: string; items?: string[] }) {
  if (!items?.length) return null;
  return (
    <div>
      <h3 className="text-sm font-medium">{title}</h3>
      <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-muted-foreground">
        {items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </div>
  );
}

function ManifestDetails({ manifest }: { manifest: ToolManifest }) {
  const fields = manifest.inputFields ?? [];
  const mapping = Object.entries(manifest.execution.mapping ?? {}).sort(
    ([a], [b]) => a.localeCompare(b),
  );
  return (
    <div className="space-y-4">
      <Card>
        <CardContent>
          <h2 className="font-medium">Description</h2>
          <p className="mt-2 text-sm">
            {manifest.description.short ?? "No description provided."}
          </p>
          <div className="mt-4 grid gap-4 md:grid-cols-3">
            <GuidanceList
              title="When to use"
              items={manifest.description.whenToUse}
            />
            <GuidanceList
              title="When not to use"
              items={manifest.description.whenNotToUse}
            />
            <GuidanceList
              title="Examples"
              items={manifest.description.examples}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <h2 className="font-medium">Inputs</h2>
          {fields.length ? (
            <div className="mt-3 overflow-x-auto">
              <table className="w-full text-left text-sm">
                <caption className="sr-only">Tool input fields</caption>
                <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2">Field</th>
                    <th className="px-3 py-2">Type</th>
                    <th className="px-3 py-2">Required</th>
                    <th className="px-3 py-2">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {fields.map((field) => (
                    <tr
                      className="border-b border-border last:border-0"
                      key={field.name}
                    >
                      <th className="px-3 py-2 font-medium">{field.name}</th>
                      <td className="px-3 py-2">
                        {field.type ?? "—"}
                        {field.enum?.length
                          ? ` (${field.enum.join(", ")})`
                          : ""}
                      </td>
                      <td className="px-3 py-2">
                        {field.required ? "Yes" : "No"}
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">
                        {field.description ?? "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="mt-2 text-sm text-muted-foreground">
              This tool does not declare input fields.
            </p>
          )}
          <p className="mt-3 text-xs text-muted-foreground">
            Input schema type: {manifest.inputType ?? "not declared"}
          </p>
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardContent>
            <h2 className="font-medium">Execution</h2>
            <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-muted-foreground">Transport</dt>
                <dd>{manifest.execution.type ?? "—"}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Method</dt>
                <dd>{manifest.execution.method ?? "—"}</dd>
              </div>
              <div className="sm:col-span-2">
                <dt className="text-muted-foreground">Endpoint path</dt>
                <dd className="break-all">
                  {manifest.execution.endpointPath ?? "—"}
                </dd>
              </div>
              <div className="sm:col-span-2">
                <dt className="text-muted-foreground">Response path</dt>
                <dd>{manifest.execution.responsePath ?? "Entire response"}</dd>
              </div>
            </dl>
            {mapping.length ? (
              <div className="mt-4">
                <h3 className="text-sm font-medium">Argument mapping</h3>
                <ul className="mt-2 space-y-1 text-sm text-muted-foreground">
                  {mapping.map(([source, target]) => (
                    <li key={source}>
                      <span className="font-medium text-foreground">
                        {source}
                      </span>{" "}
                      → {target}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            <p className="mt-3 text-xs text-muted-foreground">
              Output type: {manifest.outputType ?? "not declared"}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <h2 className="font-medium">Security and routing</h2>
            <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-muted-foreground">Authentication</dt>
                <dd>{manifest.security.authType ?? "Not declared"}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Allowed roles</dt>
                <dd>
                  {manifest.security.allowedRoles?.join(", ") ||
                    "Any configured role"}
                </dd>
              </div>
              {manifest.routing ? (
                <div>
                  <dt className="text-muted-foreground">Routing priority</dt>
                  <dd>{manifest.routing.priority}</dd>
                </div>
              ) : null}
            </dl>
            {manifest.routing?.signals?.length ? (
              <GuidanceList
                title="Routing signals"
                items={manifest.routing.signals}
              />
            ) : null}
            {manifest.routing?.antiSignals?.length ? (
              <div className="mt-4">
                <GuidanceList
                  title="Routing anti-signals"
                  items={manifest.routing.antiSignals}
                />
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardContent>
          <h2 className="font-medium">Manifest metadata</h2>
          <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">API version</dt>
              <dd>{manifest.apiVersion ?? "—"}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Kind</dt>
              <dd>{manifest.kind ?? "—"}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>
    </div>
  );
}

function toolStatus(tool: ToolProjection) {
  return tool.active ? tool.status || "ready" : "inactive";
}

function OperationalDetails({ tool }: { tool: ToolProjection }) {
  return (
    <Card>
      <CardContent>
        <h2 className="font-medium">Operational settings</h2>
        <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <dt className="text-muted-foreground">Cache</dt>
            <dd>{tool.cache?.enabled ? "Enabled" : "Disabled"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Cache TTL</dt>
            <dd>{tool.cache?.enabled ? `${tool.cache.ttlSeconds}s` : "—"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Cache mode</dt>
            <dd>
              {tool.cache?.enabled
                ? tool.cache.isReadOnly
                  ? "Read-only"
                  : "Read/write"
                : "—"}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Lifecycle</dt>
            <dd>{tool.lifecycle?.status ?? "Not declared"}</dd>
          </div>
        </dl>
        {tool.lifecycle?.replacement ? (
          <p className="mt-3 text-sm text-muted-foreground">
            Replacement:{" "}
            <span className="text-foreground">
              {tool.lifecycle.replacement}
            </span>
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

export function Tools({ contextName }: { contextName: string }) {
  const tools = useTools(contextName);
  const [filter, setFilter] = useState("");
  const visibleTools = useMemo(() => {
    const query = filter.toLowerCase();
    return (tools.data?.items ?? []).filter((tool) =>
      `${tool.name} ${tool.module ?? ""} ${tool.version} ${tool.status ?? ""} ${tool.endpointPath ?? ""}`
        .toLowerCase()
        .includes(query),
    );
  }, [filter, tools.data?.items]);

  if (tools.loading) return <Skeleton className="h-48 w-full" />;
  if (tools.error || !tools.data || tools.data.state !== "available") {
    return (
      <EmptyState
        title="Tools are unavailable"
        message={
          tools.error ?? "The selected context has no readable tool inventory."
        }
      />
    );
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="font-medium">Tools</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {tools.data.items.length} safe MCP tool projections. Select a tool
              to inspect its read-only manifest. Credentials and full upstream
              URLs are hidden.
            </p>
          </div>
          <label className="text-sm" htmlFor="tool-filter">
            Filter
            <input
              className="ml-2 h-9 rounded-md border border-border bg-card px-2"
              id="tool-filter"
              placeholder="name, module, or path"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
            />
          </label>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">Safe MCP tool inventory</caption>
            <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
              <tr>
                <th className="px-5 py-3">Name</th>
                <th className="px-5 py-3">Module</th>
                <th className="px-5 py-3">Version</th>
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3">Method and path</th>
                <th className="px-5 py-3">Cache</th>
              </tr>
            </thead>
            <tbody>
              {visibleTools.map((tool) => (
                <tr
                  className="border-b border-border last:border-0"
                  key={`${tool.name}@${tool.version}`}
                >
                  <th className="px-5 py-3 font-medium">
                    <Link
                      className="text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      href={`/tools/${encodeURIComponent(tool.name)}`}
                    >
                      {tool.name}
                    </Link>
                  </th>
                  <td className="px-5 py-3">{tool.module ?? "—"}</td>
                  <td className="px-5 py-3">{tool.version}</td>
                  <td className="px-5 py-3">
                    <StatusBadge
                      label={toolStatus(tool)}
                      tone={tool.active ? "success" : "neutral"}
                    />
                  </td>
                  <td className="px-5 py-3">
                    {tool.method ?? "—"} {tool.endpointPath ?? "—"}
                  </td>
                  <td className="px-5 py-3">
                    {tool.cache?.enabled ? `${tool.cache.ttlSeconds}s` : "off"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}

function PluginBindingDetails({
  contextName,
  tool,
}: {
  contextName: string;
  tool: ToolProjection;
}) {
  const bindings = usePluginBindings(contextName);
  if (bindings.loading || bindings.data?.state !== "available") return null;
  const matches = bindings.data.items.filter(
    (binding) =>
      binding.toolRef?.name === tool.name &&
      binding.toolRef?.version === tool.version,
  );

  return (
    <Card>
      <CardContent>
        <h2 className="font-medium">Plugin bindings</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Read-only bindings for this exact tool version. Plugin endpoints,
          credentials, and static configuration are not shown.
        </p>
        {!matches.length ? (
          <p className="mt-3 text-sm">No plugin bindings are registered.</p>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <caption className="sr-only">
                Plugin bindings for this tool
              </caption>
              <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
                <tr>
                  <th className="px-3 py-2">Binding</th>
                  <th className="px-3 py-2">Plugin</th>
                  <th className="px-3 py-2">Phase</th>
                  <th className="px-3 py-2">Policy</th>
                  <th className="px-3 py-2">Configuration</th>
                </tr>
              </thead>
              <tbody>
                {matches.map((binding) => (
                  <tr
                    className="border-b border-border last:border-0"
                    key={binding.name}
                  >
                    <th className="px-3 py-2 font-medium">{binding.name}</th>
                    <td className="px-3 py-2">
                      <Link
                        className="text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        href={`/plugins/${encodeURIComponent(binding.pluginRef.name)}/${encodeURIComponent(binding.pluginRef.version)}`}
                      >
                        {binding.pluginRef.name}@{binding.pluginRef.version}
                      </Link>
                    </td>
                    <td className="px-3 py-2">
                      {binding.phase} · priority {binding.priority}
                    </td>
                    <td className="px-3 py-2">{binding.failurePolicy}</td>
                    <td className="px-3 py-2">
                      {binding.configurationPresent ? "Present" : "None"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export function ToolDetails({
  contextName,
  toolName,
}: {
  contextName: string;
  toolName: string;
}) {
  const tools = useTools(contextName);
  const name = decodeToolName(toolName);
  const tool = tools.data?.items.find((item) => item.name === name);

  if (tools.loading) return <Skeleton className="h-48 w-full" />;
  if (tools.error || !tools.data || tools.data.state !== "available") {
    return (
      <EmptyState
        title="Tool manifest is unavailable"
        message={
          tools.error ?? "The selected context has no readable tool inventory."
        }
      />
    );
  }
  if (!tool) {
    return (
      <div className="space-y-4">
        <Link className="text-sm text-primary hover:underline" href="/tools">
          ← Back to tools
        </Link>
        <EmptyState
          title="Tool not found"
          message={`No tool named ${name} exists in the selected context.`}
        />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <Link className="text-sm text-primary hover:underline" href="/tools">
        ← Back to tools
      </Link>
      <Card>
        <CardContent>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-xs uppercase tracking-wider text-muted-foreground">
                MCP tool
              </p>
              <h2 className="mt-1 text-2xl font-semibold tracking-tight">
                {tool.name}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {tool.module ?? "Unassigned module"} · version {tool.version}
              </p>
            </div>
            <StatusBadge
              label={toolStatus(tool)}
              tone={tool.active ? "success" : "neutral"}
            />
          </div>
        </CardContent>
      </Card>
      {tool.manifest ? (
        <>
          <ManifestDetails manifest={tool.manifest} />
          <OperationalDetails tool={tool} />
          <PluginBindingDetails contextName={contextName} tool={tool} />
        </>
      ) : (
        <EmptyState
          title="Manifest details are unavailable"
          message="This server returned only the basic safe tool projection."
        />
      )}
    </div>
  );
}
