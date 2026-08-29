export function TopologyLegend() {
  return (
    <div
      aria-label="Topology legend"
      className="flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground"
    >
      <span className="font-medium text-foreground">Relationships</span>
      <span className="inline-flex items-center gap-1.5">
        <span aria-hidden="true" className="h-px w-5 bg-muted-foreground" />
        Exact
      </span>
      <span className="inline-flex items-center gap-1.5">
        <span
          aria-hidden="true"
          className="w-5 border-t border-dashed border-info"
        />
        Base prefix
      </span>
      <span className="inline-flex items-center gap-1.5">
        <span aria-hidden="true" className="font-semibold text-warning">
          ⚠
        </span>
        Ambiguous
      </span>
      <span className="inline-flex items-center gap-1.5">
        <span aria-hidden="true" className="font-semibold text-destructive">
          ×
        </span>
        Unresolved
      </span>
      <span className="inline-flex items-center gap-1.5">
        <span aria-hidden="true" className="font-semibold text-info">
          ▦
        </span>
        Collapsed group
      </span>
    </div>
  );
}
