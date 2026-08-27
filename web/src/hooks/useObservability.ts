import { useCallback, useEffect, useMemo, useState } from "react";

import {
  useAsyncResource,
  type AsyncState,
  type RefreshableState,
} from "./useConsole";
import { apiFetch, streamLogEvents } from "../lib/api";

export type LogEvent = {
  timestamp?: string;
  level?: string;
  component?: string;
  toolName?: string;
  requestId?: string;
  summary?: string;
};

export type MetricSample = {
  name: string;
  labels?: Record<string, string>;
  value: number;
};

export type MetricRate = {
  name: string;
  labels?: Record<string, string>;
  perSecond: number;
};

export type MetricsSnapshot = {
  state: string;
  observedAt?: string;
  sampleWindowStart?: string;
  cumulative: MetricSample[];
  rates: MetricRate[];
  averageLatencySeconds?: number;
};

export type MetricsHistoryPoint = MetricsSnapshot & { observedAt: string };

export function metricSeriesKey(name: string, labels?: Record<string, string>) {
  return [
    name,
    ...Object.keys(labels ?? {})
      .sort()
      .map((key) => `${key}=${labels?.[key] ?? ""}`),
  ].join("|");
}

type LogResponse = { state: string; items: LogEvent[] };

export type ToolManifest = {
  apiVersion?: string;
  kind?: string;
  description: {
    short?: string;
    whenToUse?: string[];
    whenNotToUse?: string[];
    examples?: string[];
  };
  inputType?: string;
  inputFields?: Array<{
    name: string;
    type?: string;
    description?: string;
    enum?: string[];
    required: boolean;
  }>;
  outputType?: string;
  execution: {
    type?: string;
    method?: string;
    endpointPath?: string;
    responsePath?: string;
    mapping?: Record<string, string>;
  };
  security: { authType?: string; allowedRoles?: string[] };
  routing?: {
    priority: number;
    signals?: string[];
    antiSignals?: string[];
  };
};

export type PluginProjection = {
  name: string;
  version: string;
  type?: string;
  active: boolean;
  endpointConfigured: boolean;
  timeoutMilliseconds: number;
  configurationPresent: boolean;
};

export type PluginBindingProjection = {
  name: string;
  active: boolean;
  pluginRef: { name: string; version: string };
  toolRef: { name: string; version: string };
  phase: string;
  priority: number;
  failurePolicy: string;
  configurationPresent: boolean;
};

export type ToolProjection = {
  name: string;
  version: string;
  module?: string;
  status?: string;
  active: boolean;
  description?: string;
  method?: string;
  endpointPath?: string;
  responsePath?: string;
  allowedRoles?: string[];
  cache?: { enabled: boolean; ttlSeconds: number; isReadOnly: boolean };
  lifecycle?: {
    status: string;
    deprecatedAt?: string;
    sunsetAt?: string;
    replacement?: string;
  };
  manifest?: ToolManifest;
};

type ToolResponse = {
  state: string;
  items: ToolProjection[];
  observedAt?: string;
};
type PluginResponse = {
  state: string;
  items: PluginProjection[];
  observedAt?: string;
};
type PluginBindingResponse = {
  state: string;
  items: PluginBindingProjection[];
  observedAt?: string;
};

export function useTools(contextName: string): RefreshableState<ToolResponse> {
  return useAsyncResource(
    `/api/console/v1/tools?context=${encodeURIComponent(contextName)}`,
    "Tools are unavailable",
  );
}

export function usePlugins(
  contextName: string,
): RefreshableState<PluginResponse> {
  return useAsyncResource(
    `/api/console/v1/plugins?context=${encodeURIComponent(contextName)}`,
    "Plugins are unavailable",
  );
}

export function usePluginBindings(
  contextName: string,
): RefreshableState<PluginBindingResponse> {
  return useAsyncResource(
    `/api/console/v1/plugin-bindings?context=${encodeURIComponent(contextName)}`,
    "Plugin bindings are unavailable",
  );
}

export function useLogs(
  contextName: string,
): AsyncState<LogEvent[]> & { streaming: boolean } {
  const [state, setState] = useState<AsyncState<LogEvent[]>>({
    data: null,
    error: null,
    loading: true,
  });
  const [streaming, setStreaming] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setState({ data: null, error: null, loading: true });
    const path = `/api/console/v1/logs/recent?context=${encodeURIComponent(contextName)}`;
    apiFetch<LogResponse>(path)
      .then((response) => {
        if (!active) return;
        setState({
          data: response.items,
          error: null,
          loading: false,
          lastUpdated: new Date().toISOString(),
          stale: false,
        });
        return streamLogEvents(
          `/api/console/v1/logs/stream?context=${encodeURIComponent(contextName)}`,
          controller.signal,
          (event) => {
            if (!active) return;
            setState((current) => ({
              ...current,
              data: [...(current.data ?? []), event as LogEvent].slice(-1000),
              lastUpdated: new Date().toISOString(),
              stale: false,
            }));
          },
        );
      })
      .then(() => {
        if (active) setStreaming(false);
      })
      .catch((error: unknown) => {
        if (active && !controller.signal.aborted) {
          setState((current) => ({
            ...current,
            error:
              error instanceof Error ? error.message : "Logs are unavailable",
            loading: false,
            stale: Boolean(current.data),
          }));
          setStreaming(false);
        }
      });
    setStreaming(true);
    return () => {
      active = false;
      controller.abort();
    };
  }, [contextName]);
  return { ...state, streaming };
}

export function useMetrics(contextName: string): AsyncState<MetricsSnapshot> & {
  history: MetricsHistoryPoint[];
  refresh: () => void;
} {
  const [state, setState] = useState<AsyncState<MetricsSnapshot>>({
    data: null,
    error: null,
    loading: true,
  });
  const [history, setHistory] = useState<MetricsHistoryPoint[]>([]);
  const [refreshToken, setRefreshToken] = useState(0);
  const refresh = useCallback(() => setRefreshToken((value) => value + 1), []);
  useEffect(() => {
    let active = true;
    const load = () => {
      const path = `/api/console/v1/metrics?context=${encodeURIComponent(contextName)}${refreshToken ? "&refresh=1" : ""}`;
      apiFetch<MetricsSnapshot>(path)
        .then((data) => {
          if (!active) return;
          const observedAt = data.observedAt ?? new Date().toISOString();
          setState({
            data,
            error: null,
            loading: false,
            lastUpdated: observedAt,
            stale: false,
          });
          setHistory((current) =>
            [...current, { ...data, observedAt }].slice(-30),
          );
        })
        .catch((error: unknown) => {
          if (active) {
            setState((current) => ({
              ...current,
              error:
                error instanceof Error
                  ? error.message
                  : "Metrics are unavailable",
              loading: false,
              stale: Boolean(current.data),
            }));
          }
        });
    };
    setState({ data: null, error: null, loading: true, stale: false });
    setHistory([]);
    load();
    const interval = window.setInterval(load, 10000);
    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, [contextName, refreshToken]);
  return { ...state, history, refresh };
}

export function useFilteredLogs(
  events: LogEvent[] | null,
  filters: {
    level: string;
    component: string;
    tool: string;
    requestId: string;
  },
) {
  return useMemo(() => {
    return (events ?? [])
      .filter((event) => {
        return (
          (!filters.level || event.level === filters.level) &&
          (!filters.component || event.component === filters.component) &&
          (!filters.tool || event.toolName === filters.tool) &&
          (!filters.requestId || event.requestId === filters.requestId)
        );
      })
      .sort((left, right) => {
        const leftTimestamp = left.timestamp ?? "";
        const rightTimestamp = right.timestamp ?? "";
        const leftTime = Date.parse(leftTimestamp);
        const rightTime = Date.parse(rightTimestamp);
        const leftValid = !Number.isNaN(leftTime);
        const rightValid = !Number.isNaN(rightTime);

        if (leftValid && rightValid && leftTime !== rightTime) {
          return rightTime - leftTime;
        }
        if (leftValid !== rightValid) return rightValid ? 1 : -1;
        if (leftTimestamp === rightTimestamp) return 0;
        return rightTimestamp > leftTimestamp ? 1 : -1;
      });
  }, [events, filters]);
}
