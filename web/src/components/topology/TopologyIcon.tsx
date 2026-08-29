import {
  AlertTriangle,
  Boxes,
  CircleX,
  Database,
  Layers3,
  Link2,
  Network,
  Plug,
  Route,
  Wrench,
} from "lucide-react";

export function TopologyIcon({
  kind,
  className,
}: {
  kind?: string;
  className?: string;
}) {
  const Icon =
    kind === "mcp-transport"
      ? Network
      : kind === "mcp-tool"
        ? Wrench
        : kind === "erp-api"
          ? Database
          : kind === "erp-endpoint"
            ? Route
            : kind === "plugin-binding"
              ? Link2
              : kind === "external-plugin"
                ? Plug
                : kind === "ambiguous-endpoint"
                  ? AlertTriangle
                  : kind === "unresolved-endpoint"
                    ? CircleX
                    : kind === "cluster"
                      ? Layers3
                      : Boxes;

  return (
    <Icon
      aria-hidden="true"
      className={className}
      size={16}
      strokeWidth={1.75}
    />
  );
}
