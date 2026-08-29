import type { ReactNode } from "react";

import { cn } from "../../lib/cn";

export type TopologyTone =
  "neutral" | "success" | "info" | "warning" | "danger";

const toneClasses: Record<TopologyTone, string> = {
  neutral: "border-border bg-card",
  success: "border-success/50 bg-card",
  info: "border-info/50 bg-info/[0.04]",
  warning: "border-warning/60 bg-warning/[0.04]",
  danger: "border-destructive/60 bg-destructive/[0.035]",
};

const railClasses: Record<TopologyTone, string> = {
  neutral: "bg-transparent",
  success: "bg-success",
  info: "bg-info",
  warning: "bg-warning",
  danger: "bg-destructive",
};

export function TopologyNodeShell({
  children,
  className,
  dimmed,
  selected,
  tone = "neutral",
}: {
  children: ReactNode;
  className?: string;
  dimmed?: boolean;
  selected?: boolean;
  tone?: TopologyTone;
}) {
  return (
    <div
      className={cn(
        "relative overflow-hidden rounded-xl border text-card-foreground shadow-sm transition-[border-color,box-shadow,opacity] duration-150",
        toneClasses[tone],
        selected &&
          "z-10 ring-2 ring-primary ring-offset-1 ring-offset-background shadow-md",
        dimmed ? "opacity-[0.12]" : "opacity-100",
        className,
      )}
    >
      <div
        aria-hidden="true"
        className={cn("absolute inset-y-0 left-0 w-[3px]", railClasses[tone])}
      />
      {children}
    </div>
  );
}
