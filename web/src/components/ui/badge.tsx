import type { HTMLAttributes } from "react";

import { cn } from "../../lib/cn";

type BadgeProps = HTMLAttributes<HTMLSpanElement> & {
  tone?: "success" | "warning" | "danger" | "info" | "neutral";
};

export function Badge({ className, tone = "neutral", ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium",
        tone === "success" && "border-success/30 bg-success/10 text-success",
        tone === "warning" &&
          "border-warning/30 bg-warning/10 text-warning-foreground",
        tone === "danger" &&
          "border-destructive/30 bg-destructive/10 text-destructive",
        tone === "info" && "border-info/30 bg-info/10 text-info",
        tone === "neutral" && "border-border bg-muted text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}
