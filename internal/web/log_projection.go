package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
)

const (
	maxLogResponseBytes = 1 << 20
	maxLogEventBytes    = 32 << 10
	maxLogEvents        = 1000
	maxLogStreams       = 4
)

var (
	logSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|(?:token|password|secret|api[_-]?key|authorization)\s*[=: ]+)\S+`)
	logEmailPattern  = regexp.MustCompile(`(?i)\b[\w.+-]+@[\w.-]+\.[a-z]{2,}\b`)
)

// LogEvent is the fixed, browser-safe log shape.
type LogEvent struct {
	Timestamp string `json:"timestamp,omitempty"`
	Level     string `json:"level,omitempty"`
	Component string `json:"component,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

func projectLogEvent(raw []byte) (LogEvent, error) {
	if len(raw) == 0 || len(raw) > maxLogEventBytes {
		return LogEvent{}, errors.New("log event exceeds size limit")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return LogEvent{}, errors.New("log event is malformed")
	}
	event := LogEvent{
		Timestamp: rawString(fields, "time", "timestamp"),
		Level:     strings.ToUpper(rawString(fields, "level")),
		Component: rawString(fields, "component"),
		ToolName:  rawString(fields, "tool_name", "toolName"),
		RequestID: rawString(fields, "request_id", "requestId"),
		Summary:   redactLogSummary(rawString(fields, "msg", "message")),
	}
	return event, nil
}

func redactLogSummary(summary string) string {
	if summary == "" {
		return ""
	}
	redacted := logSecretPattern.ReplaceAllString(summary, "[REDACTED]")
	return logEmailPattern.ReplaceAllString(redacted, "[REDACTED]")
}

func rawString(fields map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		var value string
		if err := json.Unmarshal(fields[name], &value); err == nil {
			return value
		}
	}
	return ""
}

func (h *consoleHandler) logsRecent(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetServer, "/api/logs/recent")
	if err != nil {
		writeJSON(w, http.StatusOK, LogListResponse{State: stateUnavailable, Items: []LogEvent{}})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, LogListResponse{State: upstreamState(response.StatusCode), Items: []LogEvent{}})
		return
	}
	var rawEvents []json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&rawEvents); err != nil {
		writeJSON(w, http.StatusOK, LogListResponse{State: stateUnavailable, Items: []LogEvent{}})
		return
	}
	if len(rawEvents) > maxLogEvents {
		rawEvents = rawEvents[len(rawEvents)-maxLogEvents:]
	}
	items := make([]LogEvent, 0, len(rawEvents))
	for _, raw := range rawEvents {
		event, err := projectLogEvent(raw)
		if err == nil {
			items = append(items, event)
		}
	}
	writeJSON(w, http.StatusOK, LogListResponse{State: stateAvailable, Items: items})
}

func (h *consoleHandler) logsStream(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	select {
	case h.logStreams <- struct{}{}:
		defer func() { <-h.logStreams }()
	default:
		writeAPIError(w, http.StatusTooManyRequests, "stream_limit", "too many console log streams are active")
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequestWithHeaders(r, ctx, bridgeclient.TargetServer, "/api/logs/stream", nil, http.Header{"Accept": []string{"text/event-stream"}})
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "upstream_unavailable", "the log stream is unavailable")
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeAPIError(w, http.StatusServiceUnavailable, "upstream_unavailable", "the log stream is unavailable")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 1024), maxLogEventBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		event, err := projectLogEvent(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: "))))
		if err != nil {
			continue
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			continue
		}
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(encoded)
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	if scanner.Err() != nil {
		return
	}
}
