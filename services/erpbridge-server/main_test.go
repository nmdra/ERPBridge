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

	"github.com/alicebob/miniredis/v2"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestInitializeCacheSelectsConfiguredBackend(t *testing.T) {
	t.Run("redis", func(t *testing.T) {
		mini := miniredis.RunT(t)
		t.Setenv("REDIS_URL", "redis://"+mini.Addr())
		manager, err := initializeCache(nil)
		require.NoError(t, err)
		require.NotNil(t, manager)
		require.Equal(t, "redis", manager.BackendName())
	})

	t.Run("memory", func(t *testing.T) {
		t.Setenv("REDIS_URL", "")
		t.Setenv("CACHE_MEMORY_MAX_ENTRIES", "10")
		manager, err := initializeCache(nil)
		require.NoError(t, err)
		require.NotNil(t, manager)
		require.Equal(t, "memory", manager.BackendName())
	})

	t.Run("invalid redis", func(t *testing.T) {
		t.Setenv("REDIS_URL", "://not-a-redis-url")
		manager, err := initializeCache(nil)
		require.Error(t, err)
		require.Nil(t, manager)
	})
}

func TestInvalidRedisURLStopsBeforeListenerStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".") // #nosec G204 -- test launches the local server package.
	cmd.Env = append(os.Environ(), "REDIS_URL=://not-a-redis-url", "MCP_PORT=0", "LOG_TO_STDERR=true")
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.False(t, ctx.Err() == context.DeadlineExceeded, "server did not reject invalid Redis URL before startup: %s", output)
	require.NotContains(t, string(output), "listening")
}

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

func TestParseRateLimitConfigUsesIncreasedDefaults(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPS", "")
	t.Setenv("RATE_LIMIT_BURST", "")
	config, err := parseRateLimitConfig()
	require.NoError(t, err)
	require.Equal(t, 10.0, config.RequestsPerSecond)
	require.Equal(t, 20, config.Burst)
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

func TestStdioProtocolListsPersistedToolMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "erpbridge.db")
	store, err := mcp.NewStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	readOnly, destructive := true, false
	annotated := &mcp.Tool{
		Metadata: mcp.Metadata{Name: "annotated-stdio", Version: "1.0.0", IsActive: true},
		Spec: mcp.ToolSpec{
			Description: mcp.Description{
				Short:        "Annotated stdio tool",
				WhenToUse:    []string{"When stdio needs metadata"},
				WhenNotToUse: []string{"When stdio should not mutate data"},
				Examples:     []string{"Show stdio metadata"},
			},
			Annotations: &mcp.ToolAnnotations{
				Title:           "Annotated stdio tool",
				ReadOnlyHint:    &readOnly,
				DestructiveHint: &destructive,
			},
			InputSchema: mcp.InputSchema{Type: "object", Properties: map[string]mcp.Property{}},
			Security:    mcp.Security{AllowedRoles: []string{"stdio_reader"}},
		},
	}
	legacy := &mcp.Tool{
		Metadata: mcp.Metadata{Name: "legacy-stdio", Version: "1.0.0", IsActive: true},
		Spec: mcp.ToolSpec{
			Description: mcp.Description{Short: "Legacy stdio tool"},
			InputSchema: mcp.InputSchema{Type: "object", Properties: map[string]mcp.Property{}},
		},
	}
	require.NoError(t, store.Save(annotated))
	require.NoError(t, store.Save(legacy))
	require.NoError(t, store.Close())

	binaryPath := filepath.Join(t.TempDir(), "erpbridge-server")
	//nolint:gosec // The test intentionally builds the local server package.
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}

	//nolint:gosec // The test intentionally launches the binary it just built.
	cmd := exec.Command(binaryPath, "--stdio")
	cmd.Env = append(os.Environ(),
		"DATABASE_PATH="+dbPath,
		"API_AUTH_TOKEN=",
		"MCP_ENABLE_TEST_TOOLS=",
		"MCP_PORT=0",
		"LOG_TO_STDERR=true",
	)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	lines := make(chan []byte, 4)
	scannerErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- append([]byte(nil), scanner.Bytes()...)
		}
		scannerErr <- scanner.Err()
		close(lines)
	}()
	readResponse := func(label string) []byte {
		select {
		case line, ok := <-lines:
			require.True(t, ok, "stdio %s response stream closed", label)
			return line
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for stdio %s response", label)
			return nil
		}
	}

	_, err = io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"metadata-stdio-test","version":"1.0"}}}`+"\n")
	require.NoError(t, err)
	var initializeResponse map[string]any
	require.NoError(t, json.Unmarshal(readResponse("initialize"), &initializeResponse))
	require.Equal(t, float64(1), initializeResponse["id"])

	_, err = io.WriteString(stdin, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`+"\n")
	require.NoError(t, err)
	var listResponse map[string]any
	require.NoError(t, json.Unmarshal(readResponse("tools/list"), &listResponse))
	tools := listResponse["result"].(map[string]any)["tools"].([]any)
	found := make(map[string]map[string]any, 2)
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if ok && (tool["name"] == annotated.Metadata.Name || tool["name"] == legacy.Metadata.Name) {
			found[tool["name"].(string)] = tool
		}
	}

	annotatedWire := found[annotated.Metadata.Name]
	require.NotNil(t, annotatedWire)
	require.Equal(t, "Annotated stdio tool", annotatedWire["title"])
	require.Equal(t, false, annotatedWire["annotations"].(map[string]any)["destructiveHint"])
	require.Equal(t, []any{"When stdio needs metadata"}, annotatedWire["_meta"].(map[string]any)["io.erpbridge/whenToUse"])
	require.Equal(t, []any{"stdio_reader"}, annotatedWire["_meta"].(map[string]any)["io.erpbridge/allowedRoles"])
	require.NotContains(t, annotatedWire, "endpoint")

	legacyWire := found[legacy.Metadata.Name]
	require.NotNil(t, legacyWire)
	require.Empty(t, legacyWire["annotations"].(map[string]any))
	require.NotContains(t, legacyWire, "_meta")

	require.NoError(t, stdin.Close())
	select {
	case scanErr := <-scannerErr:
		require.NoError(t, scanErr)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for stdio response stream to close")
	}
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
