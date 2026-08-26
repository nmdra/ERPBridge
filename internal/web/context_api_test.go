package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/config"
)

func TestContextRedactSensitiveFields(t *testing.T) {
	cfg := &config.Config{
		CurrentContext: "local",
		Contexts: map[string]config.Context{
			"local": { //nolint:gosec // sentinel credentials verify redaction
				Server:    "http://bridge.example:8082",
				MCPServer: "http://mcp.example:8080",
				APIToken:  "do-not-send-token",
				Auth:      config.AuthConfig{Key: "do-not-send-auth-key"},
			},
		},
	}
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	request := httptest.NewRequest(http.MethodGet, server.URL()+"/api/console/v1/contexts", nil)
	request.Host = server.Host()
	request.Header.Set(CapabilityHeader, server.Capability())

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "do-not-send") || strings.Contains(body, "bridge.example:8082") {
		t.Fatalf("safe context response contains sensitive data: %s", body)
	}
	var response ContextListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].Name != "local" || !response.Items[0].Current {
		t.Fatalf("context projection = %+v", response.Items)
	}
}

func TestContextAPIReadsDeploymentByKnownName(t *testing.T) {
	cfg := &config.Config{
		CurrentContext: "local",
		Contexts: map[string]config.Context{
			"local": {Server: "http://bridge.example:8082", MCPServer: "http://mcp.example:8080"},
		},
	}
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	request := httptest.NewRequest(http.MethodGet, server.URL()+"/api/console/v1/deployment?context=local", nil)
	request.Host = server.Host()
	request.Header.Set(CapabilityHeader, server.Capability())
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var deployment DeploymentResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &deployment); err != nil {
		t.Fatal(err)
	}
	if deployment.Context.Name != "local" || deployment.Console.State != "connected" {
		t.Fatalf("deployment projection = %+v", deployment)
	}
}

func TestContextAPIRejectsUnknownContextAndArbitraryProxy(t *testing.T) {
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {}}}
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	request := httptest.NewRequest(http.MethodGet, server.URL()+"/api/console/v1/deployment?context=missing", nil)
	request.Host = server.Host()
	request.Header.Set(CapabilityHeader, server.Capability())
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown context status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if strings.Contains(recorder.Body.String(), "missing") == false {
		t.Fatalf("unknown context error = %q", recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, server.URL()+"/api/console/v1/proxy?url=http://evil.example", nil)
	request.Host = server.Host()
	request.Header.Set(CapabilityHeader, server.Capability())
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("arbitrary proxy status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestContextProviderRetainsLastValidSnapshotOnReloadError(t *testing.T) {
	first := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{
		"local": {Server: "http://bridge.example:8082"},
	}}
	second := &config.Config{CurrentContext: "staging", Contexts: map[string]config.Context{
		"staging": {Server: "http://staging.example:8082"},
	}}
	current := first
	provider := func() (*config.Config, error) {
		if current == nil {
			return nil, fmt.Errorf("malformed config")
		}
		return current, nil
	}
	handler := NewConsoleHandler(HandlerOptions{
		ConfigProvider:        provider,
		ConfigRefreshInterval: time.Hour,
	})
	readContexts := func(query string) ContextListResponse {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/console/v1/contexts"+query, nil)
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var response ContextListResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	if response := readContexts(""); len(response.Items) != 1 || response.Items[0].Name != "local" {
		t.Fatalf("initial contexts = %+v", response.Items)
	}
	current = nil
	response := readContexts("?refresh=1")
	if len(response.Items) != 1 || response.Items[0].Name != "local" || !response.Stale {
		t.Fatalf("stale contexts = %+v, stale=%v", response.Items, response.Stale)
	}
	current = second
	response = readContexts("?refresh=1")
	if len(response.Items) != 1 || response.Items[0].Name != "staging" || response.Stale {
		t.Fatalf("reloaded contexts = %+v, stale=%v", response.Items, response.Stale)
	}
}

func TestContextAPIRejectsHostileOrigin(t *testing.T) {
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {}}}
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	request := httptest.NewRequest(http.MethodGet, server.URL()+"/api/console/v1/contexts", nil)
	request.Host = server.Host()
	request.Header.Set(CapabilityHeader, server.Capability())
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("hostile origin status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}
