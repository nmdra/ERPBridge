package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/config"
)

func TestMetricsParserKeepsDocumentedFamilies(t *testing.T) {
	parsed, err := parseMetrics([]byte(`# HELP erp_requests_total Total requests
# TYPE erp_requests_total counter
erp_requests_total{method="GET",path="/invoices",status="200"} 4
# TYPE mcp_sessions_active gauge
mcp_sessions_active 2
# TYPE unknown_total counter
unknown_total 99
# TYPE mcp_tool_duration_seconds histogram
mcp_tool_duration_seconds_sum{tool="list"} 1.5
mcp_tool_duration_seconds_count{tool="list"} 3
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Cumulative) != 2 || parsed.Cumulative[0].Name != "erp_requests_total" {
		t.Fatalf("cumulative metrics = %+v", parsed.Cumulative)
	}
	if parsed.HistogramCount != 3 || parsed.HistogramSum != 1.5 {
		t.Fatalf("histogram values = %v/%v", parsed.HistogramSum, parsed.HistogramCount)
	}
	for _, sample := range parsed.Cumulative {
		if strings.Contains(sample.Name, "unknown") {
			t.Fatal("unknown family was returned")
		}
	}
}

func TestMetricsParserRejectsMalformedAndOversizedExposition(t *testing.T) {
	if _, err := parseMetrics([]byte("not prometheus")); err == nil {
		t.Fatal("malformed exposition was accepted")
	}
	if _, err := parseMetrics(make([]byte, maxMetricsBytes+1)); err == nil {
		t.Fatal("oversized exposition was accepted")
	}
}

func TestMetricsSnapshotComputesSessionRateAndAverage(t *testing.T) {
	value := 1
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`# TYPE erp_requests_total counter
erp_requests_total ` + formatFloat(float64(value)) + `
# TYPE erp_request_duration_seconds histogram
erp_request_duration_seconds_sum ` + formatFloat(float64(value)) + `
erp_request_duration_seconds_count 2
`))
		value++
	}))
	defer upstream.Close()
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {MCPServer: upstream.URL}}}
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/metrics?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	first := httptest.NewRecorder()
	console.Handler().ServeHTTP(first, request)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	time.Sleep(10 * time.Millisecond)
	second := httptest.NewRecorder()
	console.Handler().ServeHTTP(second, request)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"state":"available"`) {
		t.Fatalf("second response = %d %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "perSecond") {
		t.Fatalf("session rate missing: %s", second.Body.String())
	}
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
