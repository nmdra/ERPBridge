import { memo } from "react";
import { NodeToolbar, type NodeProps } from "@xyflow/react";

import { type TopologyFlowNode } from "./topologyFlow";
import { TopologyIcon } from "./TopologyIcon";
import { TopologyNodeHandles } from "./TopologyNodeHandles";
import { TopologyNodeShell } from "./TopologyNodeShell";

export const TopologyComponentNode = memo(function TopologyComponentNode({
  data,
  selected,
}: NodeProps<TopologyFlowNode>) {
  const selectedState = selected || data.selected;
  return (
    <>
      {selectedState ? (
        <NodeToolbar isVisible>
          <div className="flex items-center gap-1 rounded-lg border border-border bg-popover p-1 text-xs text-popover-foreground shadow-md">
            <button
              className="rounded-md px-2 py-1 hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={data.onSelect}
              type="button"
            >
              Inspect
            </button>
          </div>
        </NodeToolbar>
      ) : null}
      <TopologyNodeShell
        className="w-[240px] p-3.5 pl-4"
        dimmed={data.dimmed}
        selected={selectedState}
        tone={data.tone}
      >
        <div
          aria-label={`${data.label}, ${data.presentationLabel}${data.summary ? `, ${data.summary}` : ""}`}
          aria-pressed={selectedState}
          className="cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          onClick={data.onSelect}
          onKeyDown={(event) => {
            if (event.key === "Enter" || event.key === " ") {
              event.preventDefault();
              data.onSelect();
            }
          }}
          role="button"
          tabIndex={0}
        >
          <div className="flex items-start gap-2.5">
            <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <TopologyIcon kind={data.topologyKind} />
            </span>
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium" title={data.label}>
                {data.label}
              </div>
              <div className="mt-0.5 text-[10px] font-medium uppercase tracking-[0.08em] text-muted-foreground">
                {data.presentationLabel}
              </div>
            </div>
          </div>
          {data.summary ? (
            <div className="mt-3 text-xs text-muted-foreground">
              {data.summary}
            </div>
          ) : null}
        </div>
        <TopologyNodeHandles data={data} />
      </TopologyNodeShell>
    </>
  );
});
