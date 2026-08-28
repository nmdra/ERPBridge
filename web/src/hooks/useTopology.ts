import { useAsyncResource, type RefreshableState } from "./useConsole";

export const topologyNodeKinds = [
  "mcp-transport",
  "mcp-tool",
  "erp-api",
  "plugin-binding",
  "external-plugin",
  "unresolved-endpoint",
  "ambiguous-endpoint",
] as const;

export type TopologyNodeKind = (typeof topologyNodeKinds)[number];
export type TopologyMatchKind =
  "exact" | "base-prefix" | "ambiguous" | "unresolved";
export type TopologySelection =
  { kind: "node"; id: string } | { kind: "edge"; id: string } | null;

export type TopologyNode = {
  id: string;
  kind: TopologyNodeKind | string;
  label: string;
  diagnosticReason?: string;
  contextState?: string;
  tool?: {
    name: string;
    version: string;
    active?: boolean;
    status?: string;
    method?: string;
    endpointPath?: string;
    responsePath?: string;
  };
  api?: {
    name: string;
    module?: string;
    method: string;
    endpointPath: string;
    status?: string;
  };
  plugin?: {
    name: string;
    version: string;
    type?: string;
    active: boolean;
    endpointConfigured: boolean;
    timeoutMilliseconds: number;
    configurationPresent: boolean;
  };
  binding?: {
    name: string;
    active: boolean;
    pluginRef: { name: string; version: string };
    toolRef: { name: string; version: string };
    phase: string;
    priority: number;
    failurePolicy: string;
    configurationPresent: boolean;
  };
};

export type TopologyEdge = {
  id: string;
  source: string;
  target: string;
  matchKind: TopologyMatchKind | string;
  diagnosticReason?: string;
  contextState?: string;
  authoritative: boolean;
};

export type TopologyResponse = {
  state: string;
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  truncated?: boolean;
  omitted?: { nodes: number; edges: number };
  observedAt?: string;
};

export function useTopology(
  contextName: string,
): RefreshableState<TopologyResponse> {
  return useAsyncResource(
    `/api/console/v1/topology?context=${encodeURIComponent(contextName)}`,
    "Topology is unavailable",
  );
}
