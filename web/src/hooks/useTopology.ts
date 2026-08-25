import { useEffect, useState } from "react";

import { apiFetch } from "../lib/api";

export type TopologyNode = {
  id: string;
  kind: string;
  label: string;
  contextState?: string;
  tool?: {
    name: string;
    version: string;
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
  matchKind: string;
  contextState?: string;
  authoritative: boolean;
};

export type TopologyResponse = {
  state: string;
  nodes: TopologyNode[];
  edges: TopologyEdge[];
};

export function useTopology(contextName: string) {
  const [state, setState] = useState<{
    data: TopologyResponse | null;
    error: string | null;
    loading: boolean;
  }>({ data: null, error: null, loading: true });
  useEffect(() => {
    let active = true;
    setState({ data: null, error: null, loading: true });
    apiFetch<TopologyResponse>(
      `/api/console/v1/topology?context=${encodeURIComponent(contextName)}`,
    )
      .then((data) => {
        if (active) setState({ data, error: null, loading: false });
      })
      .catch((error: unknown) => {
        if (active)
          setState({
            data: null,
            error:
              error instanceof Error
                ? error.message
                : "Topology is unavailable",
            loading: false,
          });
      });
    return () => {
      active = false;
    };
  }, [contextName]);
  return state;
}
