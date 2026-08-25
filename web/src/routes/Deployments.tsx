import { EmptyState } from "../components/ui/empty-state";
import { Card, CardContent } from "../components/ui/card";
import type { ContextProjection } from "../hooks/useConsole";

export function Deployments({
  contexts,
  error,
}: {
  contexts: ContextProjection[] | null;
  error: string | null;
}) {
  if (error) {
    return <EmptyState title="Contexts are unavailable" message={error} />;
  }
  if (!contexts?.length) {
    return (
      <EmptyState
        title="No contexts configured"
        message="Add a bridgectl context to inspect a deployment."
      />
    );
  }
  return (
    <Card>
      <CardContent className="overflow-x-auto p-0">
        <table className="w-full text-left text-sm">
          <caption className="sr-only">Configured deployments</caption>
          <thead className="border-b border-border text-xs uppercase tracking-wider text-muted-foreground">
            <tr>
              <th className="px-5 py-3">Context</th>
              <th className="px-5 py-3">Server</th>
              <th className="px-5 py-3">MCP server</th>
            </tr>
          </thead>
          <tbody>
            {contexts.map((context) => (
              <tr
                className="border-b border-border last:border-0"
                key={context.name}
              >
                <th className="px-5 py-4 font-medium">
                  {context.name}
                  {context.current ? " (current)" : ""}
                </th>
                <td className="px-5 py-4">{context.serverState}</td>
                <td className="px-5 py-4">{context.mcpServerState}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}
