package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/mcp"
)

func TestPluginProjectionsHideEndpointCredentialsAndConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/erpbridge.io/v1/plugins":
			_ = json.NewEncoder(w).Encode([]mcp.Plugin{{
				Metadata: mcp.PluginMetadata{Name: "transformer", Version: "1.2.0", Type: "response", IsActive: true},
				Spec: mcp.PluginSpec{
					Endpoint:            "https://plugin.internal.example/v1/process",
					TimeoutMilliseconds: 1500,
					Auth:                &mcp.PluginAuth{Type: "bearer", CredentialRef: "PLUGIN_SECRET", Header: "Authorization"},
				},
			}})
		case "/apis/erpbridge.io/v1/pluginbindings":
			_ = json.NewEncoder(w).Encode([]mcp.PluginBinding{{
				Metadata: mcp.PluginBindingMetadata{Name: "transform-orders", IsActive: true},
				Spec: mcp.PluginBindingSpec{
					PluginRef:     mcp.PluginRef{Name: "transformer", Version: "1.2.0"},
					ToolRef:       mcp.ToolRef{Name: "list_orders", Version: "1.0.0"},
					Phase:         "after_response",
					Priority:      10,
					FailurePolicy: "continue",
					Config:        map[string]any{"mode": "private-config"},
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{
		CurrentContext: "local",
		Contexts: map[string]config.Context{
			"local": {MCPServer: upstream.URL},
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

	for _, path := range []string{
		"/api/console/v1/plugins?context=local",
		"/api/console/v1/plugin-bindings?context=local",
	} {
		request := httptest.NewRequest(http.MethodGet, console.URL()+path, nil)
		request.Host = console.Host()
		request.Header.Set(CapabilityHeader, console.Capability())
		recorder := httptest.NewRecorder()
		console.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", path, recorder.Code, recorder.Body.String())
		}
		body := recorder.Body.String()
		for _, secret := range []string{"plugin.internal.example", "PLUGIN_SECRET", "private-config", "Authorization"} {
			if strings.Contains(body, secret) {
				t.Fatalf("%s leaked %q: %s", path, secret, body)
			}
		}
		if strings.Contains(path, "/plugins?") {
			if !strings.Contains(body, `"endpointConfigured":true`) || !strings.Contains(body, `"configurationPresent":true`) {
				t.Fatalf("plugin safety booleans missing: %s", body)
			}
		} else if !strings.Contains(body, `"configurationPresent":true`) {
			t.Fatalf("binding configuration boolean missing: %s", body)
		}
	}
}

func TestPluginProjectionFeatureIsUnavailableOnOlderServer(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	cfg := &config.Config{
		CurrentContext: "local",
		Contexts:       map[string]config.Context{"local": {MCPServer: upstream.URL}},
	}
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/plugins?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"state":"unavailable"`) {
		t.Fatalf("expected unavailable feature state: %s", recorder.Body.String())
	}
}
