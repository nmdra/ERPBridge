package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
)

func TestUpstreamURLStateRejectsMalformedConfiguredURLs(t *testing.T) {
	cfg := &config.Config{
		CurrentContext: "bad",
		Contexts: map[string]config.Context{
			"bad": { //nolint:gosec // malformed credential URL is a validation fixture
				Server:    "http://user:secret@bridge.example:8082",
				MCPServer: "localhost:8080",
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
		t.Fatalf("status = %d", recorder.Code)
	}
	var response ContextListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Items[0].ServerState != "invalid" || response.Items[0].MCPServerState != "invalid" {
		t.Fatalf("URL states = %+v", response.Items[0])
	}
}
