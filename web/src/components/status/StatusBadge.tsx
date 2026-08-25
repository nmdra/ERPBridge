import { CircleCheck, CircleHelp, CircleX, TriangleAlert } from "lucide-react";

import { Badge } from "../ui/badge";

type StatusBadgeProps = {
  label: string;
  tone?: "success" | "warning" | "danger" | "info" | "neutral";
};

const icons = {
  success: CircleCheck,
  warning: TriangleAlert,
  danger: CircleX,
  info: CircleHelp,
  neutral: CircleHelp,
};

export function StatusBadge({ label, tone = "neutral" }: StatusBadgeProps) {
  const Icon = icons[tone];
  return (
    <Badge tone={tone}>
      <Icon aria-hidden="true" size={13} />
      <span>{label}</span>
    </Badge>
  );
}
