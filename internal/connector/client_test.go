package connector

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/security"
	"github.com/stretchr/testify/require"
)

const transportTestCredential = "test-value" // #nosec G101 -- test-only credential value.

func TestClientCallDoesNotLogRequestOrResponseBodies(t *testing.T) {
	const requestSecret = "request-body-secret"
	const responseSecret = "response-body-secret"
	const endpointSecret = "request-path-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseSecret))
	}))
	defer server.Close()

	var logs bytes.Buffer
	ctx := logger.WithLogger(context.Background(), slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	client := NewClient(slog.Default())

	resp, err := client.Call(ctx, EndpointConfig{
		Method:  http.MethodPost,
		Path:    "/" + endpointSecret,
		BaseURL: server.URL,
	}, nil, bytes.NewBufferString(requestSecret))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if encoded := logs.String(); bytes.Contains([]byte(encoded), []byte(requestSecret)) || bytes.Contains([]byte(encoded), []byte(responseSecret)) || bytes.Contains([]byte(encoded), []byte(endpointSecret)) {
		t.Fatalf("connector logs expose request data: %s", encoded)
	}
}

func TestClient_CallWithOptions_PreservesFinalTransientResponse(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream"}`))
	}))
	defer server.Close()

	resp, err := NewClient(slog.Default()).CallWithOptions(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: server.URL,
	}, nil, nil, CallOptions{PreserveErrorResponses: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"error":"upstream"}` {
		t.Fatalf("body = %q", body)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestClient_CallWithOptions_DisablesRedirectsForRawCapture(t *testing.T) {
	finalCalls := 0
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		finalCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	resp, err := NewClient(slog.Default()).CallWithOptions(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: redirect.URL,
	}, nil, nil, CallOptions{PreserveErrorResponses: true, DisableRedirects: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if finalCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", finalCalls)
	}
}

func TestClient_Call_ProtectsConnectorHeadersFromGeneratedOverrides(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Trace"); got != "trace-1" {
			t.Errorf("X-Trace = %q, want trace-1", got)
		}
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Errorf("Authorization = %q, want connector value", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want connector value", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	allowInsecureAuthHost(t, ts.URL)

	client := NewClient(slog.Default())
	ep := EndpointConfig{
		Method:  http.MethodGet,
		Path:    "/test",
		BaseURL: ts.URL,
		Headers: map[string]string{"X-Trace": "trace-1", "Authorization": "evil", "Content-Type": "text/plain"},
		Auth:    AuthConfig{Type: "api-key", Key: "test-key"},
	}
	resp, err := client.Call(context.Background(), ep, nil, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
}

func TestClient_Call_APIKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-key" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	allowInsecureAuthHost(t, ts.URL)

	client := NewClient(slog.Default())
	ep := EndpointConfig{
		Method:  http.MethodGet,
		Path:    "/test",
		BaseURL: ts.URL,
		Auth: AuthConfig{
			Type: "api-key",
			Key:  "test-key",
		},
	}

	resp, err := client.Call(context.Background(), ep, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestClient_Call_APIKeyUsesConfiguredHeader(t *testing.T) {
	sawHeader := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key header, got %s", r.Header.Get("X-API-Key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		sawHeader = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	allowInsecureAuthHost(t, ts.URL)

	_, err := NewClient(slog.Default()).Call(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		Path:    "/test",
		BaseURL: ts.URL,
		Auth:    AuthConfig{Type: "api-key", Key: "test-key", Header: "X-API-Key"},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sawHeader {
		t.Fatal("request was not observed")
	}
}

func TestClient_Call_RejectsCredentialedHTTPBeforeOutbound(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()

	_, err := NewClient(slog.Default()).Call(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: server.URL,
		Auth:    AuthConfig{Type: authTypeBearer, Key: transportTestCredential},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected credentialed HTTP request to be rejected")
	}
	if calls != 0 {
		t.Fatalf("outbound calls = %d, want 0", calls)
	}
}

func TestClient_Call_AllowsExactCredentialedHTTPHostWithWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+transportTestCredential {
			t.Fatal("credential was not sent to the allowed endpoint")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	allowInsecureAuthHost(t, server.URL)

	var logs bytes.Buffer
	ctx := logger.WithLogger(context.Background(), slog.New(slog.NewTextHandler(&logs, nil)))
	resp, err := NewClient(slog.Default()).Call(ctx, EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: server.URL,
		Auth:    AuthConfig{Type: authTypeBearer, Key: transportTestCredential},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !bytes.Contains(logs.Bytes(), []byte("credentialed outbound HTTP is allowed for development")) {
		t.Fatalf("missing insecure transport warning: %s", logs.String())
	}
	if bytes.Contains(logs.Bytes(), []byte(transportTestCredential)) {
		t.Fatalf("warning exposed credential: %s", logs.String())
	}
}

func TestClient_Call_RejectsNonmatchingCredentialedHTTPHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("nonmatching credentialed endpoint must not be called")
	}))
	defer server.Close()
	t.Setenv(security.InsecureAuthAllowedHostsEnv, "localhost:80")

	_, err := NewClient(slog.Default()).Call(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: server.URL,
		Auth:    AuthConfig{Type: authTypeBearer, Key: transportTestCredential},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected credentialed HTTP request to be rejected")
	}
}

func TestClient_Call_AllowsUnauthenticatedHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := NewClient(slog.Default()).Call(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: server.URL,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
}

func TestClient_Call_RejectsURLUserinfoBeforeOutbound(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.User = url.UserPassword("user", "password") // #nosec G101 -- regression test for rejected URL userinfo.

	_, err = NewClient(slog.Default()).Call(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: endpoint.String(),
	}, nil, nil)
	if err == nil {
		t.Fatal("expected URL userinfo to be rejected")
	}
	if calls != 0 {
		t.Fatalf("outbound calls = %d, want 0", calls)
	}
}

func TestClient_Call_DoesNotFollowCredentialedRedirect(t *testing.T) {
	finalCalls := 0
	final := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		finalCalls++
	}))
	defer final.Close()

	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	client := NewClient(slog.Default())
	client.http = redirect.Client()
	resp, err := client.Call(context.Background(), EndpointConfig{
		Method:  http.MethodGet,
		BaseURL: redirect.URL,
		Auth:    AuthConfig{Type: authTypeBearer, Key: transportTestCredential},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	if finalCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", finalCalls)
	}
}

func allowInsecureAuthHost(t *testing.T, rawURL string) {
	t.Helper()
	endpoint, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(security.InsecureAuthAllowedHostsEnv, endpoint.Host)
}
