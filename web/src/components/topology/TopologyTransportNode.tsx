import { memo } from "react";
import { type NodeProps } from "@xyflow/react";

import { type TopologyFlowNode } from "./topologyFlow";
import { TopologyIcon } from "./TopologyIcon";
import { TopologyNodeHandles } from "./TopologyNodeHandles";
import { TopologyNodeShell } from "./TopologyNodeShell";

export const TopologyTransportNode = memo(function TopologyTransportNode({
  data,
  selected,
}: NodeProps<TopologyFlowNode>) {
  return (
    <TopologyNodeShell
      className="flex h-[72px] w-[220px] items-center rounded-full p-3 pl-4"
      dimmed={data.dimmed}
      selected={selected || data.selected}
      tone={data.tone}
    >
      <div
        aria-label={`${data.label}, ${data.presentationLabel}`}
        className="flex min-w-0 cursor-pointer items-center gap-2.5 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
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
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <TopologyIcon kind="mcp-transport" />
        </span>
        <span className="truncate text-sm font-medium" title={data.label}>
          {data.label}
        </span>
      </div>
      <TopologyNodeHandles data={data} />
    </TopologyNodeShell>
  );
});
