package bridgeclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
)

func TestValidateServerURLRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"",
		"localhost:8080",
		"ftp://bridge.example",
		"http://",
		"http://user:pass@bridge.example",
		"http://bridge.example?token=secret",
		"http://bridge.example/#fragment",
	} {
		t.Run(raw, func(t *testing.T) {
			if err := ValidateServerURL(raw); err == nil {
				t.Fatalf("ValidateServerURL(%q) accepted an unsafe URL", raw)
			}
		})
	}
}

func TestClientUsesSplitServerAndMCPServerEndpoints(t *testing.T) {
	serverRequests := 0
	mcpRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverRequests++
		if r.URL.Path != "/api/cache/stats" {
			t.Fatalf("server path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer context-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "server")
	}))
	defer server.Close()
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mcpRequests++
		if r.URL.Path != "/mcp/health" {
			t.Fatalf("MCP path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "mcp")
	}))
	defer mcp.Close()

	client, err := New(config.Context{Server: server.URL, MCPServer: mcp.URL, APIToken: "context-token"}, "")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), TargetServer, http.MethodGet, "/api/cache/stats", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(body) != "server" || serverRequests != 1 {
		t.Fatalf("server response = %q, requests = %d", body, serverRequests)
	}

	response, err = client.Do(context.Background(), TargetMCPServer, http.MethodGet, "/mcp/health", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(body) != "mcp" || mcpRequests != 1 {
		t.Fatalf("MCP response = %q, requests = %d", body, mcpRequests)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	redirectTargetHit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetHit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, httptest.NewRequest(http.MethodGet, target.URL, nil), target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	client, err := New(config.Context{Server: redirect.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), TargetServer, http.MethodGet, "/api/logs/recent", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusFound)
	}
	if redirectTargetHit {
		t.Fatal("client followed redirect")
	}
}

func TestClientDoesNotForwardBrowserHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "" {
			t.Errorf("Cookie forwarded: %q", got)
		}
		if got := r.Header.Get("Origin"); got != "" {
			t.Errorf("Origin forwarded: %q", got)
		}
		if got := r.Header.Get("X-Forwarded-For"); got != "" {
			t.Errorf("X-Forwarded-For forwarded: %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := New(config.Context{Server: server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), TargetServer, http.MethodGet, "/api/logs/recent", nil, http.Header{
		"Accept":          []string{"application/json"},
		"Cookie":          []string{"secret"},
		"Origin":          []string{"https://evil.example"},
		"X-Forwarded-For": []string{"127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func TestResolveTokenPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, explicit, environment, context, want string
	}{
		{name: "explicit", explicit: "flag", environment: "env", context: "context", want: "flag"},
		{name: "environment", environment: "env", context: "context", want: "env"},
		{name: "context", context: "context", want: "context"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveToken(test.explicit, test.environment, test.context); got != test.want {
				t.Fatalf("ResolveToken() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestClientRejectsPathEscapes(t *testing.T) {
	client, err := New(config.Context{Server: "http://127.0.0.1:8080"}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), TargetServer, http.MethodGet, "/api/../secret", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("path escape error = %v", err)
	}
}

func TestClientBoundsResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "0123456789")
	}))
	defer server.Close()

	client, err := New(config.Context{Server: server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	client.MaxResponseBytes = 4
	response, err := client.Do(context.Background(), TargetServer, http.MethodGet, "/api/logs/recent", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("read error = %v, want %v", err, ErrResponseTooLarge)
	}
}
