export function Freshness({
  lastUpdated,
  stale = false,
}: {
  lastUpdated?: string;
  stale?: boolean;
}) {
  const date = lastUpdated ? new Date(lastUpdated) : null;
  const timestamp =
    date && !Number.isNaN(date.getTime())
      ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
      : "not observed";
  return (
    <p className="text-xs text-muted-foreground" role="status">
      {stale ? "Stale · " : ""}Last updated {timestamp}
    </p>
  );
}
