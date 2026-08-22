package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
)

func TestShouldPrint(t *testing.T) {
	// reset globals
	logComponent = ""
	logTool = ""
	logLevel = ""
	logRequestID = ""

	msg := `{"component":"mcp", "tool_name":"test-tool", "level":"ERROR", "request_id":"req-1"}`

	if !shouldPrint(msg) {
		t.Errorf("expected shouldPrint to be true")
	}

	logComponent = cliMCPScope
	if !shouldPrint(msg) {
		t.Errorf("expected shouldPrint to be true with mcp component")
	}

	logComponent = "other"
	if shouldPrint(msg) {
		t.Errorf("expected shouldPrint to be false with other component")
	}

	logComponent = ""
	logTool = "test-tool"
	if !shouldPrint(msg) {
		t.Errorf("expected true")
	}

	logTool = "other"
	if shouldPrint(msg) {
		t.Errorf("expected false")
	}

	logTool = ""
	logLevel = "error"
	if !shouldPrint(msg) {
		t.Errorf("expected true")
	}

	logLevel = "info"
	if shouldPrint(msg) {
		t.Errorf("expected false")
	}

	logLevel = ""
	logRequestID = "req-1"
	if !shouldPrint(msg) {
		t.Errorf("expected true")
	}

	logRequestID = "req-2"
	if shouldPrint(msg) {
		t.Errorf("expected false")
	}

	logComponent = cliMCPScope
	if shouldPrint(`{"message":"component\":\"mcp"}`) {
		t.Errorf("expected substring-only match to be filtered")
	}
	if !shouldPrint(`not-json`) {
		t.Errorf("malformed records should pass through")
	}
}

func TestLogStatsCmd(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"level":"INFO", "tool_name":"tool1"}]`))
	}))
	defer ts.Close()

	cfg = &config.Config{
		CurrentContext: testContextName,
		Contexts: map[string]config.Context{
			testContextName: {Server: ts.URL},
		},
	}

	var buf bytes.Buffer
	logStatsCmd.SetOut(&buf)
	logStatsCmd.SetContext(context.Background())
	err := logStatsCmd.RunE(logStatsCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("INFO")) {
		t.Errorf("expected output to contain INFO")
	}
}

func TestLogTailCmd(t *testing.T) {
	// reset globals
	logComponent = ""
	logTool = ""
	logLevel = ""
	logRequestID = ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"level\":\"INFO\"}\n\n"))
	}))
	defer ts.Close()

	cfg = &config.Config{
		CurrentContext: "test",
		Contexts: map[string]config.Context{
			"test": {Server: ts.URL},
		},
	}

	var buf bytes.Buffer
	logTailCmd.SetOut(&buf)
	logTailCmd.SetContext(context.Background())
	err := logTailCmd.RunE(logTailCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("INFO")) {
		t.Errorf("expected output to contain INFO, got: %q", buf.String())
	}
}
