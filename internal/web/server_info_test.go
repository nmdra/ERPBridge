package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
)

func TestInfoProjectsSafeServerMetadataAndSupportsOlderDeployments(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			_, _ = w.Write([]byte(`{"version":"v1","commit":"abc","cacheBackend":"memory","activeToolCount":3,"secret":"do-not-send"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {MCPServer: upstream.URL}}}
	console, err := NewServer(Options{ListenAddress: "127.0.0.1:0", Handler: NewConsoleHandler(HandlerOptions{Config: cfg})})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/server-info?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"available"`) || strings.Contains(recorder.Body.String(), "do-not-send") {
		t.Fatalf("info response = %d %s", recorder.Code, recorder.Body.String())
	}

	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	cfg.Contexts["old"] = config.Context{MCPServer: old.URL}
	request = httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/server-info?context=old", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder = httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"unavailable"`) {
		t.Fatalf("old info response = %d %s", recorder.Code, recorder.Body.String())
	}
}
