package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServeHTTPStopsWhenContextIsCanceled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		ReadHeaderTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- serveHTTP(ctx, server, listener)
	}()

	client := &http.Client{Timeout: time.Second}
	require.Eventually(t, func() bool {
		response, requestErr := client.Get("http://" + listener.Addr().String())
		if requestErr != nil {
			return false
		}
		defer func() { _ = response.Body.Close() }()
		return response.StatusCode == http.StatusNoContent
	}, time.Second, 10*time.Millisecond)

	cancel()
	select {
	case err := <-serveErr:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not stop after context cancellation")
	}

	_, err = client.Get("http://" + listener.Addr().String())
	require.Error(t, err)
}
