import type { ReactNode } from "react";

import { cn } from "../../lib/cn";

export function FilterToolbar({
  children,
  summary,
  actions,
}: {
  children: ReactNode;
  summary?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <section
      aria-label="Filters"
      className="rounded-xl border border-border bg-card p-4 shadow-sm"
    >
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div
          className={cn(
            "grid min-w-0 flex-1 gap-3 sm:grid-cols-2 lg:grid-cols-4",
          )}
        >
          {children}
        </div>
        {actions ? (
          <div className="flex shrink-0 items-center gap-2">{actions}</div>
        ) : null}
      </div>
      {summary ? (
        <p className="mt-3 text-xs text-muted-foreground">{summary}</p>
      ) : null}
    </section>
  );
}
