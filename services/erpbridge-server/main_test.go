package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRateLimitConfigRejectsInvalidAndTrailingValues(t *testing.T) {
	for _, value := range []string{"0", "-1", "NaN", "+Inf", "1.0oops"} {
		t.Run("rps_"+value, func(t *testing.T) {
			t.Setenv("RATE_LIMIT_RPS", value)
			t.Setenv("RATE_LIMIT_BURST", "1")
			_, err := parseRateLimitConfig()
			require.Error(t, err)
		})
	}
	for _, value := range []string{"0", "-1", "1oops"} {
		t.Run("burst_"+value, func(t *testing.T) {
			t.Setenv("RATE_LIMIT_RPS", "0.5")
			t.Setenv("RATE_LIMIT_BURST", value)
			_, err := parseRateLimitConfig()
			require.Error(t, err)
		})
	}
}

func TestParseRateLimitConfigAcceptsSubsecondRate(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPS", "0.5")
	t.Setenv("RATE_LIMIT_BURST", "2")
	config, err := parseRateLimitConfig()
	require.NoError(t, err)
	require.Equal(t, 0.5, config.RequestsPerSecond)
	require.Equal(t, 2, config.Burst)
}

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

func TestStdioProtocolKeepsStdoutForJSONRPC(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "erpbridge-server")
	//nolint:gosec // The test intentionally builds the local package.
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}

	//nolint:gosec // The test intentionally launches the binary it just built.
	cmd := exec.Command(binaryPath, "--stdio")
	cmd.Env = append(os.Environ(),
		"DATABASE_PATH=:memory:",
		"LOG_LEVEL=info",
		"LOG_TO_STDERR=true",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("open stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("open stdout: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("open stderr: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	stderrBytesCh := make(chan []byte, 1)
	go func() {
		stderrBytes, _ := io.ReadAll(stderr)
		stderrBytesCh <- stderrBytes
	}()

	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"stdio-regression-test","version":"1.0"}}}` + "\n"
	if _, err := io.WriteString(stdin, initialize); err != nil {
		t.Fatalf("write initialize: %v", err)
	}

	lines := make(chan []byte, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- append([]byte(nil), scanner.Bytes()...)
			return
		}
		lines <- nil
	}()

	var response []byte
	select {
	case response = <-lines:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for stdio initialize response")
	}
	if !json.Valid(response) {
		t.Fatalf("first stdout line is not JSON-RPC: %q", response)
	}
	if bytes.Contains(response, []byte("ERPBridge Server")) {
		t.Fatalf("startup banner corrupted stdout protocol: %q", response)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("stop server: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("wait for server: %v", err)
		}
	}
	stderrBytes := <-stderrBytesCh
	if !bytes.Contains(stderrBytes, []byte("ERPBridge Server")) {
		t.Fatalf("startup banner should be written to stderr; got %q", stderrBytes)
	}
}
