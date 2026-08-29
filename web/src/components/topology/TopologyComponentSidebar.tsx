import { AlertTriangle, CircleCheck, CircleX } from "lucide-react";

import type { TopologyNode } from "../../hooks/useTopology";
import type { EndpointComponent } from "../../routes/topologyPresentation";
import {
  componentForNode,
  componentSummary,
} from "../../routes/topologyPresentation";
import { TopologyIcon } from "./TopologyIcon";

function componentTone(component: EndpointComponent) {
  if (
    component.endpoint.kind === "unresolved-endpoint" ||
    component.matchCounts.unresolved > 0
  ) {
    return "danger" as const;
  }
  if (
    component.endpoint.kind === "ambiguous-endpoint" ||
    component.matchCounts.ambiguous > 0
  ) {
    return "warning" as const;
  }
  return "success" as const;
}

function StatusIcon({ tone }: { tone: ReturnType<typeof componentTone> }) {
  if (tone === "danger") {
    return (
      <CircleX aria-hidden="true" className="text-destructive" size={15} />
    );
  }
  if (tone === "warning") {
    return (
      <AlertTriangle aria-hidden="true" className="text-warning" size={15} />
    );
  }
  return <CircleCheck aria-hidden="true" className="text-success" size={15} />;
}

const SIDEBAR_TOOL_LIMIT = 12;

export function TopologyComponentSidebar({
  components,
  focusedComponentID,
  onFocus,
  tools = [],
}: {
  components: readonly EndpointComponent[];
  focusedComponentID: string | null;
  onFocus: (componentID: string, nodeID: string) => void;
  tools?: readonly TopologyNode[];
}) {
  const attention = components.filter(
    (component) => componentTone(component) !== "success",
  );
  const healthy = components.filter(
    (component) => componentTone(component) === "success",
  );

  const renderComponent = (component: EndpointComponent) => {
    const tone = componentTone(component);
    const selected = focusedComponentID === component.endpoint.id;
    return (
      <button
        aria-label={`Show component for ${component.endpoint.label}`}
        aria-pressed={selected}
        className={`w-full rounded-lg px-3 py-2 text-left transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${selected ? "bg-muted ring-1 ring-border" : ""}`}
        key={component.endpoint.id}
        onClick={() => onFocus(component.endpoint.id, component.endpoint.id)}
        type="button"
      >
        <div className="flex items-center gap-2">
          <StatusIcon tone={tone} />
          <TopologyIcon
            kind={component.endpoint.kind}
            className="shrink-0 text-muted-foreground"
          />
          <span className="min-w-0 truncate font-medium">
            {component.endpoint.label}
          </span>
        </div>
        <div className="mt-1 pl-6 text-xs text-muted-foreground">
          {componentSummary(component)}
        </div>
      </button>
    );
  };

  return (
    <aside
      aria-label="Topology components"
      className="rounded-xl border border-border bg-card p-3"
    >
      <h2 className="px-3 text-sm font-medium">Components</h2>
      {attention.length ? (
        <section className="mt-4" aria-labelledby="topology-attention-heading">
          <h3
            className="px-3 text-[10px] font-medium uppercase tracking-[0.08em] text-muted-foreground"
            id="topology-attention-heading"
          >
            Needs attention
          </h3>
          <div className="mt-1 space-y-1">{attention.map(renderComponent)}</div>
        </section>
      ) : null}
      {healthy.length ? (
        <section className="mt-4" aria-labelledby="topology-healthy-heading">
          <h3
            className="px-3 text-[10px] font-medium uppercase tracking-[0.08em] text-muted-foreground"
            id="topology-healthy-heading"
          >
            Healthy
          </h3>
          <div className="mt-1 space-y-1">{healthy.map(renderComponent)}</div>
        </section>
      ) : null}
      {tools.length ? (
        <section className="mt-4" aria-labelledby="topology-tools-heading">
          <h3
            className="px-3 text-[10px] font-medium uppercase tracking-[0.08em] text-muted-foreground"
            id="topology-tools-heading"
          >
            MCP tools
          </h3>
          <div className="mt-1 space-y-1">
            {tools.slice(0, SIDEBAR_TOOL_LIMIT).map((tool) => {
              const component = componentForNode(components, tool.id);
              if (!component) return null;
              return (
                <button
                  aria-label={`Show component for ${tool.label}`}
                  aria-pressed={focusedComponentID === component.endpoint.id}
                  className="w-full truncate rounded-lg px-3 py-2 pl-9 text-left text-xs text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  key={tool.id}
                  onClick={() => onFocus(component.endpoint.id, tool.id)}
                  type="button"
                >
                  {tool.label}
                </button>
              );
            })}
          </div>
          {tools.length > SIDEBAR_TOOL_LIMIT ? (
            <p className="px-3 pt-2 text-xs text-muted-foreground">
              {tools.length - SIDEBAR_TOOL_LIMIT} more tools are available
              through search.
            </p>
          ) : null}
        </section>
      ) : null}
      {!components.length ? (
        <p className="px-3 py-4 text-sm text-muted-foreground">
          No components match the current filters.
        </p>
      ) : null}
    </aside>
  );
}
