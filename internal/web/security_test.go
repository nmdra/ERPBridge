package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginRejectsHostAndOriginMismatches(t *testing.T) {
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

	tests := []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{name: "wrong host", host: "localhost:" + server.Port(), want: http.StatusForbidden},
		{name: "wrong origin", host: server.Host(), origin: "http://localhost:" + server.Port(), want: http.StatusForbidden},
		{name: "valid same origin", host: server.Host(), origin: server.Origin(), want: http.StatusNoContent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, server.URL()+"/api/test", nil)
			request.Host = test.host
			request.Header.Set(CapabilityHeader, server.Capability())
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

func TestServerRejectsCrossOriginSSE(t *testing.T) {
	server, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	request := httptest.NewRequest(http.MethodGet, server.URL()+"/api/logs/stream", nil)
	request.Host = server.Host()
	request.Header.Set(CapabilityHeader, server.Capability())
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin SSE status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestServerSetsSecurityHeaders(t *testing.T) {
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

	request := httptest.NewRequest(http.MethodGet, server.URL()+"/healthz", nil)
	request.Host = server.Host()
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
		"X-Frame-Options":        "DENY",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("Content-Security-Policy is empty")
	}
}
