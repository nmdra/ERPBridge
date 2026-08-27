package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistration(t *testing.T) {
	// Simply verifying that the variables are initialized and registered without panic
	if ERPRequestsTotal == nil {
		t.Error("ERPRequestsTotal is not initialized")
	}
	if ERPLatency == nil {
		t.Error("ERPLatency is not initialized")
	}
	if ToolInvocationsTotal == nil {
		t.Error("ToolInvocationsTotal is not initialized")
	}
	if ToolLatency == nil {
		t.Error("ToolLatency is not initialized")
	}
	if CacheHitsTotal == nil {
		t.Error("CacheHitsTotal is not initialized")
	}
	if CacheMissesTotal == nil {
		t.Error("CacheMissesTotal is not initialized")
	}
	if CredentialResolutionsTotal == nil {
		t.Error("CredentialResolutionsTotal is not initialized")
	}
}

func TestMetricsUsage(_ *testing.T) {
	// Test that we can use the metrics without panic
	ERPRequestsTotal.With(prometheus.Labels{labelMethod: "GET", labelPath: "/test", labelStatus: "200"}).Inc()
	ERPLatency.With(prometheus.Labels{labelMethod: "GET", labelPath: "/test"}).Observe(0.1)
	ToolInvocationsTotal.With(prometheus.Labels{labelTool: "test-tool", labelCacheStatus: "hit"}).Inc()
	ToolLatency.With(prometheus.Labels{labelTool: "test-tool"}).Observe(0.5)
	CacheHitsTotal.With(prometheus.Labels{"type": "exact"}).Inc()
	CacheMissesTotal.Inc()
	CredentialResolutionsTotal.WithLabelValues("file", "success").Inc()
}
