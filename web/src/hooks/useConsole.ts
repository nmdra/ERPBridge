import { useEffect, useState } from "react";

import { apiFetch } from "../lib/api";

export type ContextProjection = {
  name: string;
  serverIdentity: string;
  mcpServerIdentity: string;
  serverState: string;
  mcpServerState: string;
  current: boolean;
};

type ContextResponse = { items: ContextProjection[] };

export type AsyncState<T> = {
  data: T | null;
  error: string | null;
  loading: boolean;
};

export function useContexts(): AsyncState<ContextProjection[]> {
  const [state, setState] = useState<AsyncState<ContextProjection[]>>({
    data: null,
    error: null,
    loading: true,
  });
  useEffect(() => {
    let active = true;
    apiFetch<ContextResponse>("/api/console/v1/contexts")
      .then((response) => {
        if (active)
          setState({ data: response.items, error: null, loading: false });
      })
      .catch((error: unknown) => {
        if (active) {
          setState({
            data: null,
            error:
              error instanceof Error
                ? error.message
                : "Contexts are unavailable",
            loading: false,
          });
        }
      });
    return () => {
      active = false;
    };
  }, []);
  return state;
}

export type DeploymentResponse = {
  context: ContextProjection;
  console: { state: string };
};

export type ServerInfoResponse = {
  state: string;
  version?: string;
  commit?: string;
  date?: string;
  cacheBackend?: string;
  activeToolCount?: number;
  observedAt?: string;
};

export function useServerInfo(
  contextName: string,
): AsyncState<ServerInfoResponse> {
  const [state, setState] = useState<AsyncState<ServerInfoResponse>>({
    data: null,
    error: null,
    loading: true,
  });
  useEffect(() => {
    let active = true;
    apiFetch<ServerInfoResponse>(
      `/api/console/v1/server-info?context=${encodeURIComponent(contextName)}`,
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
                : "Server metadata is unavailable",
            loading: false,
          });
      });
    return () => {
      active = false;
    };
  }, [contextName]);
  return state;
}

export function useDeployment(
  contextName: string,
): AsyncState<DeploymentResponse> {
  const [state, setState] = useState<AsyncState<DeploymentResponse>>({
    data: null,
    error: null,
    loading: true,
  });
  useEffect(() => {
    let active = true;
    setState({ data: null, error: null, loading: true });
    apiFetch<DeploymentResponse>(
      `/api/console/v1/deployment?context=${encodeURIComponent(contextName)}`,
    )
      .then((data) => {
        if (active) setState({ data, error: null, loading: false });
      })
      .catch((error: unknown) => {
        if (active) {
          setState({
            data: null,
            error:
              error instanceof Error
                ? error.message
                : "Deployment is unavailable",
            loading: false,
          });
        }
      });
    return () => {
      active = false;
    };
  }, [contextName]);
  return state;
}
