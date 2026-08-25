import type { ReactNode } from "react";

import { cn } from "../../lib/cn";

export function MetricCard({
  label,
  value,
  detail,
  icon,
  tone = "primary",
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  icon?: ReactNode;
  tone?: "primary" | "success" | "warning" | "danger" | "info";
}) {
  return (
    <section
      aria-label={label}
      className="rounded-xl border border-border bg-card p-5 shadow-sm"
    >
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
          {label}
        </p>
        {icon ? (
          <span
            aria-hidden="true"
            className={cn(
              "inline-flex h-8 w-8 items-center justify-center rounded-lg",
              tone === "primary" && "bg-primary/10 text-primary",
              tone === "success" && "bg-success/10 text-success",
              tone === "warning" && "bg-warning/10 text-warning-foreground",
              tone === "danger" && "bg-destructive/10 text-destructive",
              tone === "info" && "bg-info/10 text-info",
            )}
          >
            {icon}
          </span>
        ) : null}
      </div>
      <p className="mt-3 text-2xl font-semibold tracking-tight">{value}</p>
      {detail ? (
        <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
      ) : null}
    </section>
  );
}
