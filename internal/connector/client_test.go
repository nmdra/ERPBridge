package connector

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
)

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

func TestClient_Call_APIKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-key" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

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
