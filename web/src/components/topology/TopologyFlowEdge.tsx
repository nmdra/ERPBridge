import { memo } from "react";
import {
  BaseEdge,
  EdgeLabelRenderer,
  getSmoothStepPath,
  type EdgeProps,
} from "@xyflow/react";

import { type TopologyFlowEdge as TopologyFlowEdgeModel } from "./topologyFlow";

function edgeColor(matchKind: string) {
  if (matchKind === "unresolved") return "hsl(var(--destructive))";
  if (matchKind === "ambiguous") return "hsl(var(--warning))";
  if (matchKind === "base-prefix") return "hsl(var(--info))";
  if (matchKind === "summary") return "hsl(var(--muted-foreground))";
  return "hsl(var(--muted-foreground))";
}

function edgeLabel(matchKind: string) {
  if (matchKind === "ambiguous") return "Ambiguous";
  if (matchKind === "unresolved") return "Unresolved";
  if (matchKind === "base-prefix") return "Base prefix";
  return null;
}

export const TopologyFlowEdge = memo(function TopologyFlowEdge({
  id,
  sourceX,
  sourceY,
  sourcePosition,
  targetX,
  targetY,
  targetPosition,
  selected,
  markerEnd,
  data,
}: EdgeProps<TopologyFlowEdgeModel>) {
  const matchKind = data?.matchKind ?? "exact";
  const [path, labelX, labelY] = getSmoothStepPath({
    borderRadius: 6,
    sourcePosition,
    sourceX,
    sourceY,
    targetPosition,
    targetX,
    targetY,
  });
  const label = edgeLabel(matchKind);
  const opacity = selected
    ? 1
    : data?.dimmed
      ? 0.06
      : matchKind === "summary"
        ? 0.55
        : 0.72;

  return (
    <>
      <BaseEdge
        id={id}
        markerEnd={markerEnd}
        path={path}
        style={{
          opacity,
          stroke: edgeColor(matchKind),
          strokeDasharray: matchKind === "base-prefix" ? "5 4" : undefined,
          strokeWidth: selected ? 2.2 : 1.25,
        }}
      />
      {label &&
      (selected || matchKind === "ambiguous" || matchKind === "unresolved") ? (
        <EdgeLabelRenderer>
          <div
            className="nodrag nopan pointer-events-none absolute rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] font-medium text-card-foreground shadow-sm"
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            }}
          >
            {label}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
});
