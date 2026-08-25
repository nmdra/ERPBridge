import type { HTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export function Skeleton({
  className,
  "aria-label": ariaLabel = "Loading",
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      aria-label={ariaLabel}
      aria-live="polite"
      aria-busy="true"
      className={cn("animate-pulse rounded-md bg-muted", className)}
      role="status"
      {...props}
    />
  );
}
