package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewServerRejectsNonLoopbackAddresses(t *testing.T) {
	for _, address := range []string{
		":0",
		"0.0.0.0:0",
		"[::]:0",
		"localhost:0",
		"192.0.2.1:0",
		"[::ffff:127.0.0.1]:0",
	} {
		t.Run(address, func(t *testing.T) {
			if _, err := NewServer(Options{ListenAddress: address}); err == nil {
				t.Fatalf("NewServer(%q) accepted a non-loopback address", address)
			}
		})
	}
}

func TestServerCapabilityURL(t *testing.T) {
	server, err := NewServer(Options{ListenAddress: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	if strings.Contains(server.URL(), "#") {
		t.Fatalf("base URL contains a fragment: %q", server.URL())
	}
	capabilityURL := server.CapabilityURL()
	if !strings.HasPrefix(capabilityURL, server.URL()+"#cap=") {
		t.Fatalf("capability URL %q does not use the expected fragment", capabilityURL)
	}
	if server.Capability() == "" {
		t.Fatal("server capability is empty")
	}
}

func TestCapabilityRequiredForDataRoutes(t *testing.T) {
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	request := httptest.NewRequest(http.MethodGet, "http://"+server.Host()+"/api/test", nil)
	request.Host = server.Host()
	responseRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(responseRecorder, request)
	if responseRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing capability status = %d, want %d", responseRecorder.Code, http.StatusUnauthorized)
	}

	request.Header.Set(CapabilityHeader, "wrong")
	responseRecorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(responseRecorder, request)
	if responseRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("wrong capability status = %d, want %d", responseRecorder.Code, http.StatusUnauthorized)
	}

	request.Header.Set(CapabilityHeader, server.Capability())
	responseRecorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(responseRecorder, request)
	if responseRecorder.Code != http.StatusNoContent {
		t.Fatalf("valid capability status = %d, want %d", responseRecorder.Code, http.StatusNoContent)
	}

	healthRequest := httptest.NewRequest(http.MethodGet, "http://"+server.Host()+"/healthz", nil)
	healthRequest.Host = server.Host()
	healthRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(healthRecorder, healthRequest)
	if healthRecorder.Code != http.StatusNoContent {
		t.Fatalf("health status = %d, want %d", healthRecorder.Code, http.StatusNoContent)
	}
}

func TestServerRequestLogOmitsCapabilityAndQuery(t *testing.T) {
	var logs bytes.Buffer
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Logger:        slog.New(slog.NewTextHandler(&logs, nil)),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	request := httptest.NewRequest(http.MethodGet, server.URL()+"/api/test?token=query-secret", nil)
	request.Host = server.Host()
	request.Header.Set(CapabilityHeader, server.Capability())
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if strings.Contains(logs.String(), server.Capability()) || strings.Contains(logs.String(), "query-secret") {
		t.Fatalf("request log contains sensitive request data: %q", logs.String())
	}
}

func TestServerShutdown(t *testing.T) {
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "ok")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve() }()

	client := &http.Client{Timeout: time.Second}
	request, err := http.NewRequest(http.MethodGet, server.URL()+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = server.Host()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		t.Fatal(err)
	}
	if err := <-serveErr; err != nil {
		t.Fatalf("Serve() returned %v", err)
	}
}
