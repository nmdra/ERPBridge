import { AlertCircle, CheckCircle2, Info, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "../../lib/cn";

const icons = {
  success: CheckCircle2,
  warning: TriangleAlert,
  danger: AlertCircle,
  info: Info,
};

export function StateBanner({
  title,
  message,
  tone = "info",
  action,
}: {
  title: string;
  message?: ReactNode;
  tone?: keyof typeof icons;
  action?: ReactNode;
}) {
  const Icon = icons[tone];
  return (
    <div
      aria-live={tone === "danger" ? "assertive" : "polite"}
      className={cn(
        "flex flex-wrap items-start gap-3 rounded-xl border p-4",
        tone === "success" && "border-success/30 bg-success/5",
        tone === "warning" && "border-warning/30 bg-warning/5",
        tone === "danger" && "border-destructive/30 bg-destructive/5",
        tone === "info" && "border-info/30 bg-info/5",
      )}
      role={tone === "danger" ? "alert" : "status"}
    >
      <Icon
        aria-hidden="true"
        className={cn(
          "mt-0.5 shrink-0",
          tone === "success" && "text-success",
          tone === "warning" && "text-warning-foreground",
          tone === "danger" && "text-destructive",
          tone === "info" && "text-info",
        )}
        size={18}
      />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{title}</p>
        {message ? (
          <p className="mt-1 text-sm text-muted-foreground">{message}</p>
        ) : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
