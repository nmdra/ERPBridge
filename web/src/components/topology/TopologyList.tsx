import type {
  TopologyEdge,
  TopologyNode,
  TopologySelection,
} from "../../hooks/useTopology";
import { Card, CardContent } from "../ui/card";

const actionClassName =
  "text-left text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export function TopologyList({
  nodes,
  edges,
  selection,
  onSelectNode,
  onSelectEdge,
}: {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  selection: TopologySelection;
  onSelectNode: (id: string) => void;
  onSelectEdge: (id: string) => void;
}) {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  return (
    <Card>
      <CardContent className="max-h-[32rem] overflow-auto p-0">
        <div className="min-w-[68rem]">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">
              Accessible topology relationships
            </caption>
            <thead className="sticky top-0 z-10 border-b border-border bg-card text-xs uppercase tracking-wider text-muted-foreground">
              <tr>
                <th className="px-5 py-3">Source</th>
                <th className="px-5 py-3">Target</th>
                <th className="px-5 py-3">Match</th>
                <th className="px-5 py-3">Details</th>
                <th className="px-5 py-3">Select</th>
              </tr>
            </thead>
            <tbody>
              {edges.length ? (
                edges.map((edge) => {
                  const source = byID.get(edge.source);
                  const target = byID.get(edge.target);
                  const selected =
                    selection?.kind === "edge" && selection.id === edge.id;
                  return (
                    <tr
                      aria-selected={selected}
                      className={
                        selected
                          ? "border-b border-primary/50 bg-primary/10 last:border-0"
                          : "border-b border-border last:border-0"
                      }
                      key={edge.id}
                    >
                      <th className="px-5 py-3 font-medium">
                        <button
                          className={`${actionClassName} break-words [overflow-wrap:anywhere]`}
                          onClick={() => source && onSelectNode(source.id)}
                          type="button"
                        >
                          {source?.label ?? edge.source}
                        </button>
                      </th>
                      <td className="px-5 py-3">
                        <button
                          className={`${actionClassName} break-words [overflow-wrap:anywhere]`}
                          onClick={() => target && onSelectNode(target.id)}
                          type="button"
                        >
                          {target?.label ?? edge.target}
                        </button>
                      </td>
                      <td className="px-5 py-3">
                        {edge.matchKind}
                        {edge.authoritative
                          ? " (authoritative)"
                          : " (inferred or unresolved)"}
                      </td>
                      <td className="px-5 py-3">
                        {target?.tool?.endpointPath ??
                          target?.api?.endpointPath ??
                          target?.binding?.pluginRef.name ??
                          target?.plugin?.version ??
                          "—"}
                      </td>
                      <td className="px-5 py-3">
                        <button
                          aria-pressed={selected}
                          className="rounded-md border border-border px-2 py-1 text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={() => onSelectEdge(edge.id)}
                          type="button"
                        >
                          {selected ? "Selected" : "Inspect"}
                        </button>
                      </td>
                    </tr>
                  );
                })
              ) : (
                <tr>
                  <td
                    className="px-5 py-8 text-center text-muted-foreground"
                    colSpan={5}
                  >
                    No relationships match the current filters.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  );
}
