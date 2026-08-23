// Package metrics provides Prometheus metrics instruments for tracking requests, tool invocations, cache performance, and sessions.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	labelMethod      = "method"
	labelPath        = "path"
	labelStatus      = "status"
	labelTool        = "tool"
	labelCacheStatus = "cache_status"
)

var (
	// ERPRequestsTotal counts outbound ERP HTTP requests.
	ERPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "erp_requests_total",
		Help: "Total number of outbound ERP requests",
	}, []string{labelMethod, labelPath, labelStatus})

	// ERPLatency measures the latency of outbound ERP requests.
	ERPLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "erp_request_duration_seconds",
		Help:    "Latency of outbound ERP requests",
		Buckets: prometheus.DefBuckets,
	}, []string{labelMethod, labelPath})

	// ToolInvocationsTotal counts MCP tool calls.
	ToolInvocationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_tool_invocations_total",
		Help: "Total number of MCP tool calls",
	}, []string{labelTool, labelCacheStatus})

	// ToolLatency measures latency of MCP tool calls.
	ToolLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mcp_tool_duration_seconds",
		Help:    "Latency of MCP tool calls",
		Buckets: prometheus.DefBuckets,
	}, []string{labelTool})

	// CacheHitsTotal counts cache hits by type.
	CacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total number of cache hits",
	}, []string{"type"}) // exact

	// CacheMissesTotal counts total cache misses.
	CacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total number of cache misses",
	})

	// ServerStartsTotal counts total MCP server starts.
	ServerStartsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mcp_server_starts_total",
		Help: "Total number of MCP server starts",
	})

	// ServerStopsTotal counts total MCP server stops.
	ServerStopsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mcp_server_stops_total",
		Help: "Total number of MCP server stops",
	})

	// SessionsStartedTotal counts total MCP sessions started.
	SessionsStartedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mcp_sessions_started_total",
		Help: "Total number of MCP sessions started",
	})

	// SessionsEndedTotal counts total MCP sessions ended.
	SessionsEndedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mcp_sessions_ended_total",
		Help: "Total number of MCP sessions ended",
	})

	// SessionsActive gauges the number of active MCP sessions.
	SessionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mcp_sessions_active",
		Help: "Number of active MCP sessions",
	})
)

// InitializeToolMetrics creates zero-valued MCP metric series for a registered tool.
//
// Prometheus vectors do not export a labeled series until the label values have
// been accessed. Initializing the known series at registration makes cold-start
// scrapes useful without changing the values recorded by middleware.
func InitializeToolMetrics(toolName string) {
	ToolInvocationsTotal.WithLabelValues(toolName, "SUCCESS")
	ToolLatency.WithLabelValues(toolName)
}
