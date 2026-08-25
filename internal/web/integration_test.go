package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
)

func TestEmbeddedConsoleFlowKeepsThreatBoundary(t *testing.T) {
	const token = "integration-secret-token"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("upstream authorization = %q", got)
		}
		switch r.URL.Path {
		case "/mcp/health":
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case "/apis/erpbridge.io/v1/tools":
			_, _ = io.WriteString(w, `[{"metadata":{"name":"list","version":"1.0.0"},"spec":{"execution":{"method":"GET","endpoint":"http://erp.local/api/list"},"security":{"credentialRef":"ERP_SECRET"}}}]`)
		case "/api/cache/stats":
			_, _ = io.WriteString(w, `{"stats":{"exactKeys":1,"redisMemory":"0B"}}`)
		case "/api/logs/recent":
			_, _ = io.WriteString(w, `[{"level":"ERROR","msg":"token=raw-secret","payload":{"password":"raw-secret"}}]`)
		case "/metrics":
			_, _ = io.WriteString(w, "# TYPE erp_requests_total counter\nerp_requests_total 1\n")
		case "/api/info":
			_, _ = io.WriteString(w, `{"version":"test","secret":"hidden"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	registry, err := idp.NewRegistry(t.TempDir()+"/registry.json", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	registry.APIs["list"] = idp.API{Name: "list", URL: "http://erp.local/api/list", Method: "GET", AuthKey: "registry-secret"}
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {Server: upstream.URL, MCPServer: upstream.URL, ERPBase: "http://erp.local", APIToken: token, Auth: config.AuthConfig{Key: "auth-secret"}}}}
	console, err := NewServer(Options{ListenAddress: "127.0.0.1:0", Handler: NewConsoleHandler(HandlerOptions{Config: cfg, Registry: registry})})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	if !strings.Contains(console.CapabilityURL(), "#cap=") {
		t.Fatalf("capability URL = %q", console.CapabilityURL())
	}
	request := httptest.NewRequest(http.MethodGet, console.URL()+"/", nil)
	request.Host = console.Host()
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), token) {
		t.Fatalf("HTML response = %d %s", recorder.Code, recorder.Body.String())
	}

	for _, path := range []string{
		"/api/console/v1/contexts",
		"/api/console/v1/health?context=local",
		"/api/console/v1/tools?context=local",
		"/api/console/v1/cache?context=local",
		"/api/console/v1/logs/recent?context=local",
		"/api/console/v1/metrics?context=local",
		"/api/console/v1/server-info?context=local",
		"/api/console/v1/topology?context=local",
	} {
		request = httptest.NewRequest(http.MethodGet, console.URL()+path, nil)
		request.Host = console.Host()
		request.Header.Set(CapabilityHeader, console.Capability())
		recorder = httptest.NewRecorder()
		console.Handler().ServeHTTP(recorder, request)
		if recorder.Code < http.StatusOK || recorder.Code >= http.StatusMultipleChoices {
			t.Fatalf("%s status = %d body = %s", path, recorder.Code, recorder.Body.String())
		}
		for _, secret := range []string{token, "ERP_SECRET", "registry-secret", "auth-secret", "raw-secret", "hidden"} {
			if strings.Contains(recorder.Body.String(), secret) {
				t.Fatalf("%s leaked %q: %s", path, secret, recorder.Body.String())
			}
		}
	}

	request = httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/contexts", nil)
	request.Host = console.Host()
	recorder = httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing capability status = %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/contexts", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	request.Header.Set("Origin", "https://evil.example")
	recorder = httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("hostile origin status = %d", recorder.Code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := console.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
