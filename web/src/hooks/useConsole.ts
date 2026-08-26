import { useCallback, useEffect, useRef, useState } from "react";

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
  lastUpdated?: string;
  stale?: boolean;
};

export type RefreshableState<T> = AsyncState<T> & { refresh: () => void };

function observedAt(value: unknown) {
  if (!value || typeof value !== "object") return new Date().toISOString();
  const timestamp = (value as { observedAt?: unknown }).observedAt;
  return typeof timestamp === "string" && timestamp
    ? timestamp
    : new Date().toISOString();
}

function isUnavailable(value: unknown) {
  return (
    value !== null &&
    typeof value === "object" &&
    (value as { state?: unknown }).state === "unavailable"
  );
}

function responseIsStale(value: unknown) {
  return (
    value !== null &&
    typeof value === "object" &&
    (value as { stale?: unknown }).stale === true
  );
}

export function useAsyncResource<T>(
  path: string,
  unavailableMessage: string,
  intervalMilliseconds = 15000,
): RefreshableState<T> {
  const [state, setState] = useState<AsyncState<T>>({
    data: null,
    error: null,
    loading: true,
  });
  const [refreshToken, setRefreshToken] = useState(0);
  const previousPath = useRef(path);
  const refresh = useCallback(() => setRefreshToken((value) => value + 1), []);

  useEffect(() => {
    let active = true;
    let latestRequest = 0;
    const pathChanged = previousPath.current !== path;
    previousPath.current = path;
    if (pathChanged) {
      setState({ data: null, error: null, loading: true });
    } else {
      setState((current) => ({
        ...current,
        error: null,
        loading: current.data === null,
      }));
    }
    const requestPath = refreshToken
      ? `${path}${path.includes("?") ? "&" : "?"}refresh=1`
      : path;
    const load = () => {
      const requestNumber = ++latestRequest;
      apiFetch<T>(requestPath)
        .then((data) => {
          if (!active || requestNumber !== latestRequest) return;
          if (isUnavailable(data)) {
            setState((current) => ({
              ...current,
              error: unavailableMessage,
              loading: false,
              stale: Boolean(current.data),
            }));
            return;
          }
          setState({
            data,
            error: null,
            loading: false,
            lastUpdated: observedAt(data),
            stale: responseIsStale(data),
          });
        })
        .catch((error: unknown) => {
          if (active && requestNumber === latestRequest) {
            setState((current) => ({
              ...current,
              error:
                error instanceof Error ? error.message : unavailableMessage,
              loading: false,
              stale: Boolean(current.data),
            }));
          }
        });
    };
    load();
    const interval = window.setInterval(load, intervalMilliseconds);
    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, [intervalMilliseconds, path, refreshToken, unavailableMessage]);

  const visibleState =
    previousPath.current === path
      ? state
      : { data: null, error: null, loading: true };
  return { ...visibleState, refresh };
}

export function useContexts(): RefreshableState<ContextProjection[]> {
  const resource = useAsyncResource<ContextResponse>(
    "/api/console/v1/contexts",
    "Contexts are unavailable",
  );
  return { ...resource, data: resource.data?.items ?? null };
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

export type HealthResponse = {
  state: string;
  status?: string;
  observedAt?: string;
};

export type CacheResponse = {
  state: string;
  stats?: {
    exactKeys: number;
    redisMemory?: string;
  };
};

export function useServerInfo(
  contextName: string,
): RefreshableState<ServerInfoResponse> {
  return useAsyncResource(
    `/api/console/v1/server-info?context=${encodeURIComponent(contextName)}`,
    "Server metadata is unavailable",
  );
}

export function useHealth(
  contextName: string,
): RefreshableState<HealthResponse> {
  return useAsyncResource(
    `/api/console/v1/health?context=${encodeURIComponent(contextName)}`,
    "Health is unavailable",
  );
}

export function useCache(
  contextName: string,
): RefreshableState<CacheResponse> {
  return useAsyncResource(
    `/api/console/v1/cache?context=${encodeURIComponent(contextName)}`,
    "Cache data is unavailable",
  );
}

export function useDeployment(
  contextName: string,
): RefreshableState<DeploymentResponse> {
  return useAsyncResource(
    `/api/console/v1/deployment?context=${encodeURIComponent(contextName)}`,
    "Deployment is unavailable",
  );
}
