import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
} from "react";

import {
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  Panel,
  ReactFlow,
  useReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import type {
  TopologyVisualGraph,
  TopologyVisualNode,
} from "../../routes/topologyViewModel";
import { graphLayoutKey, layoutTopology } from "../../routes/topologyLayout";
import {
  buildTopologyFlow,
  type TopologyFlowEdgeData,
  type TopologyFlowNode,
} from "./topologyFlow";
import { TopologyClusterNode } from "./TopologyClusterNode";
import { TopologyComponentNode } from "./TopologyComponentNode";
import { TopologyEntityNode } from "./TopologyEntityNode";
import { TopologyFlowEdge as TopologyFlowEdgeRenderer } from "./TopologyFlowEdge";
import { TopologyTransportNode } from "./TopologyTransportNode";
import { TopologyLegend } from "./TopologyLegend";

const nodeTypes = {
  transport: TopologyTransportNode,
  component: TopologyComponentNode,
  entity: TopologyEntityNode,
  cluster: TopologyClusterNode,
};
const edgeTypes = { topology: TopologyFlowEdgeRenderer };
const fitViewOptions = { duration: 180, maxZoom: 1.1, padding: 0.2 };

function miniMapNodeColor(node: Node) {
  const kind = node.data?.topologyKind ?? node.data?.viewKind;
  if (kind === "ambiguous-endpoint") return "hsl(var(--warning))";
  if (kind === "unresolved-endpoint") return "hsl(var(--destructive))";
  if (node.data?.viewKind === "cluster") return "hsl(var(--info))";
  if (node.data?.viewKind === "transport")
    return "hsl(var(--muted-foreground))";
  return "hsl(var(--primary))";
}

function FitViewAfterLayout({
  layoutKey,
  ready,
}: {
  layoutKey: string;
  ready: boolean;
}) {
  const { fitView } = useReactFlow();
  useEffect(() => {
    if (ready) void fitView(fitViewOptions);
  }, [fitView, layoutKey, ready]);
  return null;
}

export type TopologyCanvasProps = {
  graph: TopologyVisualGraph;
  mode: "overview" | "focused" | "expanded";
  onSelectNode: (nodeID: string) => void;
  onSelectEdge: (edgeID: string) => void;
  onExpandCluster: (node: TopologyVisualNode) => void;
  onClearSelection: () => void;
};

export function TopologyCanvas({
  graph,
  mode,
  onSelectNode,
  onSelectEdge,
  onExpandCluster,
  onClearSelection,
}: TopologyCanvasProps) {
  const currentFlow = useMemo(
    () =>
      buildTopologyFlow(graph, {
        onExpandCluster,
        onSelectNode: (node) =>
          onSelectNode(node.originalNodeId ?? node.componentId ?? node.id),
      }),
    [graph, onExpandCluster, onSelectNode],
  );
  const currentLayoutKey = useMemo(
    () => graphLayoutKey(currentFlow.nodes, currentFlow.edges),
    [currentFlow.edges, currentFlow.nodes],
  );
  const latestFlow = useRef(currentFlow);
  latestFlow.current = currentFlow;
  const [layout, setLayout] = useState<{
    key: string;
    nodes: Node[];
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLayout(null);
    void layoutTopology(latestFlow.current.nodes, latestFlow.current.edges)
      .then((next) => {
        if (!cancelled) {
          setLayout({ key: currentLayoutKey, nodes: next.nodes });
        }
      })
      .catch(() => {
        if (!cancelled) {
          setLayout({ key: currentLayoutKey, nodes: latestFlow.current.nodes });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [currentLayoutKey]);

  const positionedNodes = useMemo(() => {
    const positions = new Map(
      (layout?.nodes ?? []).map((node) => [node.id, node.position]),
    );
    return currentFlow.nodes.map((node) => ({
      ...node,
      position: positions.get(node.id) ?? node.position,
    }));
  }, [currentFlow.nodes, layout?.nodes]);
  const layoutReady = layout?.key === currentLayoutKey;
  const showMiniMap = mode !== "overview" && positionedNodes.length >= 30;

  const handleNodeClick = useCallback(
    (_event: MouseEvent, node: Node<TopologyFlowNode["data"]>) => {
      onSelectNode(
        node.data.originalNodeId ?? node.data.componentId ?? node.id,
      );
    },
    [onSelectNode],
  );
  const handleEdgeClick = useCallback(
    (_event: MouseEvent, edge: Edge<TopologyFlowEdgeData>) => {
      onSelectEdge(edge.data?.originalEdgeIds?.[0] ?? edge.id);
    },
    [onSelectEdge],
  );

  return (
    <div className="space-y-3">
      <div
        aria-busy={!layoutReady}
        aria-label="Interactive API to MCP topology"
        className="relative h-[calc(100vh-14rem)] min-h-[32rem] overflow-hidden rounded-xl border border-border bg-background"
        role="application"
      >
        <ReactFlow
          edges={currentFlow.edges}
          edgeTypes={edgeTypes}
          elementsSelectable
          fitView
          fitViewOptions={fitViewOptions}
          maxZoom={1.5}
          minZoom={0.1}
          nodeTypes={nodeTypes}
          nodes={positionedNodes}
          nodesConnectable={false}
          nodesDraggable={false}
          nodesFocusable
          edgesFocusable
          onEdgeClick={handleEdgeClick}
          onNodeClick={handleNodeClick}
          onPaneClick={onClearSelection}
        >
          <FitViewAfterLayout
            layoutKey={currentLayoutKey}
            ready={layoutReady}
          />
          <Background
            color="hsl(var(--border))"
            gap={24}
            size={1}
            variant={BackgroundVariant.Dots}
          />
          <Controls />
          {showMiniMap ? (
            <MiniMap nodeColor={miniMapNodeColor} pannable zoomable />
          ) : null}
          <Panel
            className="rounded-lg border border-border bg-card/95 px-3 py-2 text-xs text-muted-foreground shadow-sm"
            position="top-left"
          >
            {mode === "overview"
              ? "Select a component to investigate its relationships."
              : "MCP tool → ERP API → ERP endpoint. Select a node or relationship to inspect it. Press Escape to clear."}
          </Panel>
        </ReactFlow>
      </div>
      <TopologyLegend />
    </div>
  );
}
