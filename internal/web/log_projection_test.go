package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/config"
)

func TestRedactLogProjection(t *testing.T) {
	raw := []byte(`{"time":"2026-08-25T12:00:00Z","level":"error","component":"mcp","tool_name":"list","request_id":"req-1","msg":"token=secret-value email user@example.com" ,"args":{"password":"secret"},"unknown":"do-not-send"}`)
	event, err := projectLogEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(event)
	body := string(encoded)
	if strings.Contains(body, "secret-value") || strings.Contains(body, "user@example.com") || strings.Contains(body, "do-not-send") {
		t.Fatalf("projected event contains sensitive data: %s", body)
	}
	if event.Level != "ERROR" || event.ToolName != "list" || event.RequestID != "req-1" {
		t.Fatalf("projected event = %+v", event)
	}
}

func TestMalformedAndOversizedLogEventsAreRejected(t *testing.T) {
	if _, err := projectLogEvent([]byte("not-json")); err == nil {
		t.Fatal("malformed event was accepted")
	}
	if _, err := projectLogEvent(make([]byte, maxLogEventBytes+1)); err == nil {
		t.Fatal("oversized event was accepted")
	}
}

func TestLogRecentProjectsSafeEvents(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/logs/recent" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"time":"now","level":"info","msg":"token=secret","payload":{"password":"secret"}},{"level":"warn","msg":"safe"}]`))
	}))
	defer upstream.Close()
	cfg := &config.Config{
		CurrentContext: "local",
		Contexts:       map[string]config.Context{"local": {Server: upstream.URL}},
	}
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/logs/recent?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "payload") {
		t.Fatalf("recent log response contains raw data: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"state":"available"`) {
		t.Fatalf("recent log state missing: %s", recorder.Body.String())
	}
}

func TestSSEProjectsEventsAndCancelsOnDisconnect(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := w.Write([]byte("data: {\"level\":\"info\",\"msg\":\"token=secret\"}\n\n")); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {Server: upstream.URL}}}
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/logs/stream?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	requestContext, cancel := context.WithCancel(request.Context())
	request = request.WithContext(requestContext)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		console.Handler().ServeHTTP(recorder, request)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("SSE response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSlowConsumerStreamLimit(t *testing.T) {
	console := &consoleHandler{
		config:     &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {}}},
		logStreams: make(chan struct{}, 1),
	}
	console.logStreams <- struct{}{}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/console/v1/logs/stream?context=local", nil)
	recorder := httptest.NewRecorder()
	console.logsStream(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("stream limit status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
}
