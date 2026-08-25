import type { TopologyEdge, TopologyNode } from "../../hooks/useTopology";
import { Card, CardContent } from "../ui/card";

export function TopologyList({
  nodes,
  edges,
  filter,
}: {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  filter: string;
}) {
  const visible = nodes.filter((node) =>
    `${node.label} ${node.kind} ${node.contextState ?? ""}`
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  const byID = new Map(nodes.map((node) => [node.id, node]));
  return (
    <Card>
      <CardContent className="max-h-[32rem] overflow-auto p-0">
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
            </tr>
          </thead>
          <tbody>
            {edges
              .filter((edge) =>
                visible.some(
                  (node) => node.id === edge.source || node.id === edge.target,
                ),
              )
              .map((edge) => {
                const source = byID.get(edge.source);
                const target = byID.get(edge.target);
                return (
                  <tr
                    className="border-b border-border last:border-0"
                    key={edge.id}
                  >
                    <th className="px-5 py-3 font-medium">
                      {source?.label ?? edge.source}
                    </th>
                    <td className="px-5 py-3">
                      {target?.label ?? edge.target}
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
                        "—"}
                    </td>
                  </tr>
                );
              })}
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}
