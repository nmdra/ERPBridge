package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	"github.com/nmdra/ERPBridge/internal/config"
)

func TestUpstreamResponseRemainsReadableUntilClosed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 64<<10))
	}))
	defer upstream.Close()
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {MCPServer: upstream.URL}}}
	handler := &consoleHandler{config: cfg}
	request := httptest.NewRequest(http.MethodGet, "http://console/api/console/v1/tools?context=local", nil)
	ctx, ok := handler.context("local")
	if !ok {
		t.Fatal("test context was not found")
	}
	response, err := handler.upstreamRequest(request, ctx, bridgeclient.TargetMCPServer, "/apis/erpbridge.io/v1/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 64<<10 {
		t.Fatalf("body length = %d, want %d", len(body), 64<<10)
	}
}

func TestHealthToolsCacheSafeDTO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer context-token" {
			t.Fatalf("authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/cache/stats":
			_, _ = io.WriteString(w, `{"apiVersion":"v1","kind":"CacheStats","status":"active","stats":{"exactKeys":4,"redisMemory":"1MB"}}`)
		case "/mcp/health":
			_, _ = io.WriteString(w, `{"status":"ok","secret":"do-not-send"}`)
		case "/apis/erpbridge.io/v1/tools":
			_, _ = io.WriteString(w, `[{"apiVersion":"v1","kind":"MCPTool","metadata":{"name":"list-invoices","version":"1.0.0","module":"finance","status":"ready","isActive":true},"spec":{"description":{"short":"List invoices","whenToUse":["Find invoices"]},"inputSchema":{"type":"object","properties":{"company":{"type":"string","description":"Company name"}},"required":["company"]},"outputSchema":{"type":"array"},"execution":{"type":"http","method":"GET","endpoint":"https://erp.example/api/invoices?token=secret","responsePath":"data.items"},"security":{"authType":"api-key","credentialRef":"ERP_SECRET"},"cache":{"enabled":true,"ttlSeconds":60,"isReadOnly":true}}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		CurrentContext: "local",
		Contexts: map[string]config.Context{
			"local": {Server: server.URL, MCPServer: server.URL, APIToken: "context-token"},
		},
	}
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "health", path: "/api/console/v1/health?context=local"},
		{name: "tools", path: "/api/console/v1/tools?context=local"},
		{name: "cache", path: "/api/console/v1/cache?context=local"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, console.URL()+test.path, nil)
			request.Host = console.Host()
			request.Header.Set(CapabilityHeader, console.Capability())
			recorder := httptest.NewRecorder()
			console.Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "erp.example") || strings.Contains(recorder.Body.String(), "ERP_SECRET") {
				t.Fatalf("safe response contains sensitive data: %s", recorder.Body.String())
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/tools?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	var tools ToolListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &tools); err != nil {
		t.Fatal(err)
	}
	if tools.State != "available" || len(tools.Items) != 1 || tools.Items[0].EndpointPath != "/api/invoices" {
		t.Fatalf("tool response = %+v", tools)
	}
	manifest := tools.Items[0].Manifest
	if manifest == nil || manifest.OutputType != "array" || len(manifest.InputFields) != 1 || !manifest.InputFields[0].Required {
		t.Fatalf("manifest projection = %+v", manifest)
	}
}

func TestObservabilityReportsUnavailableWithoutTargetOrOn404(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	cfg := &config.Config{
		CurrentContext: "local",
		Contexts: map[string]config.Context{
			"local": {Server: upstream.URL},
		},
	}
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/health?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"unavailable"`) {
		t.Fatalf("404 health response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/cache?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder = httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"unavailable"`) {
		t.Fatalf("missing cache response = %d %s", recorder.Code, recorder.Body.String())
	}
}
