package web

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const maxMetricsBytes = 1 << 20

var documentedMetricFamilies = map[string]bool{
	"erp_requests_total":           true,
	"mcp_tool_invocations_total":   true,
	"cache_hits_total":             true,
	"cache_misses_total":           true,
	"mcp_server_starts_total":      true,
	"mcp_server_stops_total":       true,
	"mcp_sessions_started_total":   true,
	"mcp_sessions_ended_total":     true,
	"mcp_sessions_active":          true,
	"erp_request_duration_seconds": true,
	"mcp_tool_duration_seconds":    true,
}

// MetricSample is a cumulative or gauge metric value.
type MetricSample struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

// MetricRate is a session-local rate derived from successive samples.
type MetricRate struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	PerSecond float64           `json:"perSecond"`
}

// MetricsResponse is a bounded live metrics snapshot.
type MetricsResponse struct {
	State                 string         `json:"state"`
	ObservedAt            time.Time      `json:"observedAt"`
	SampleWindowStart     time.Time      `json:"sampleWindowStart"`
	Cumulative            []MetricSample `json:"cumulative"`
	Rates                 []MetricRate   `json:"rates"`
	AverageLatencySeconds float64        `json:"averageLatencySeconds,omitempty"`
}

type parsedMetrics struct {
	Cumulative     []MetricSample
	HistogramSum   float64
	HistogramCount float64
}

type metricsBaseline struct {
	ObservedAt time.Time
	Values     map[string]float64
}

func parseMetrics(data []byte) (parsedMetrics, error) {
	if len(data) == 0 || len(data) > maxMetricsBytes {
		return parsedMetrics{}, errors.New("metrics exposition exceeds size limit")
	}
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return parsedMetrics{}, errors.New("metrics exposition is malformed")
	}
	parsed := parsedMetrics{Cumulative: make([]MetricSample, 0)}
	for name, family := range families {
		if !documentedMetricFamilies[name] {
			continue
		}
		switch family.GetType() {
		case dto.MetricType_COUNTER:
			for _, metric := range family.GetMetric() {
				parsed.Cumulative = append(parsed.Cumulative, MetricSample{Name: name, Labels: metricLabels(metric), Value: metric.GetCounter().GetValue()})
			}
		case dto.MetricType_GAUGE:
			for _, metric := range family.GetMetric() {
				parsed.Cumulative = append(parsed.Cumulative, MetricSample{Name: name, Labels: metricLabels(metric), Value: metric.GetGauge().GetValue()})
			}
		case dto.MetricType_HISTOGRAM:
			for _, metric := range family.GetMetric() {
				parsed.HistogramSum += metric.GetHistogram().GetSampleSum()
				parsed.HistogramCount += float64(metric.GetHistogram().GetSampleCount())
			}
		}
	}
	sort.Slice(parsed.Cumulative, func(i, j int) bool {
		return metricKey(parsed.Cumulative[i]) < metricKey(parsed.Cumulative[j])
	})
	return parsed, nil
}

func metricLabels(metric *dto.Metric) map[string]string {
	labels := make(map[string]string, len(metric.GetLabel()))
	for _, label := range metric.GetLabel() {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}

func metricKey(sample MetricSample) string {
	keys := make([]string, 0, len(sample.Labels))
	for key := range sample.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString(sample.Name)
	for _, key := range keys {
		builder.WriteByte('|')
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(sample.Labels[key])
	}
	return builder.String()
}

func makeMetricsSnapshot(parsed parsedMetrics, now time.Time, previous *metricsBaseline) MetricsResponse {
	snapshot := MetricsResponse{
		State:             stateAvailable,
		ObservedAt:        now,
		SampleWindowStart: now,
		Cumulative:        parsed.Cumulative,
		Rates:             make([]MetricRate, 0),
	}
	if parsed.HistogramCount > 0 {
		snapshot.AverageLatencySeconds = parsed.HistogramSum / parsed.HistogramCount
	}
	if previous == nil {
		return snapshot
	}
	snapshot.SampleWindowStart = previous.ObservedAt
	seconds := now.Sub(previous.ObservedAt).Seconds()
	if seconds <= 0 {
		return snapshot
	}
	for _, sample := range parsed.Cumulative {
		previousValue, ok := previous.Values[metricKey(sample)]
		if !ok || sample.Value < previousValue {
			continue
		}
		snapshot.Rates = append(snapshot.Rates, MetricRate{
			Name:      sample.Name,
			Labels:    sample.Labels,
			PerSecond: (sample.Value - previousValue) / seconds,
		})
	}
	return snapshot
}

func metricBaselineFor(parsed parsedMetrics, now time.Time) metricsBaseline {
	values := make(map[string]float64, len(parsed.Cumulative))
	for _, sample := range parsed.Cumulative {
		values[metricKey(sample)] = sample.Value
	}
	return metricsBaseline{ObservedAt: now, Values: values}
}

func (h *consoleHandler) metricsSnapshot(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctxName := r.URL.Query().Get("context")
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/metrics")
	if err != nil {
		writeJSON(w, http.StatusOK, MetricsResponse{State: stateUnavailable, Cumulative: []MetricSample{}, Rates: []MetricRate{}})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, MetricsResponse{State: upstreamState(response.StatusCode), Cumulative: []MetricSample{}, Rates: []MetricRate{}})
		return
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		writeJSON(w, http.StatusOK, MetricsResponse{State: stateUnavailable, Cumulative: []MetricSample{}, Rates: []MetricRate{}})
		return
	}
	parsed, err := parseMetrics(data)
	if err != nil {
		writeJSON(w, http.StatusOK, MetricsResponse{State: stateUnavailable, Cumulative: []MetricSample{}, Rates: []MetricRate{}})
		return
	}
	now := time.Now().UTC()
	h.metricsMu.Lock()
	previousValue, hasPrevious := h.metricBaselines[ctxName]
	var previous *metricsBaseline
	if hasPrevious {
		previous = &previousValue
	}
	snapshot := makeMetricsSnapshot(parsed, now, previous)
	h.metricBaselines[ctxName] = metricBaselineFor(parsed, now)
	h.metricsMu.Unlock()
	writeJSON(w, http.StatusOK, snapshot)
}
