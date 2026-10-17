package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/stretchr/testify/require"
)

// logStatsFixture serves a bounded recent-log payload for the summary command.
func logStatsFixture(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/logs/recent" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{"level":"info","tool_name":"list_employees","message":"raw one"},
			{"level":"info","tool_name":"list_employees"},
			{"level":"error","tool_name":"get_invoice","message":"raw two"}
		]`)
	}))
	t.Cleanup(server.Close)
	return server
}

func withLogStatsContext(t *testing.T, serverURL string, format output.Format) *bytes.Buffer {
	t.Helper()
	cfg = &config.Config{
		CurrentContext: "test",
		Contexts:       map[string]config.Context{"test": {Server: serverURL}},
	}
	buffer := &bytes.Buffer{}
	formatter = &output.Formatter{Format: format, Out: buffer}
	outputFormat = string(format)
	logStatsCmd.SetOut(buffer)
	t.Cleanup(func() {
		outputFormat = "table"
		logStatsCmd.SetOut(nil)
	})
	return buffer
}

// TestLogStatsReportsAuthenticationFailure covers issue #19 on the log summary
// path, which decoded the body before it considered the status.
func TestLogStatsReportsAuthenticationFailure(t *testing.T) {
	server := authFailureServer(t)
	_ = withLogStatsContext(t, server.URL, output.FormatTable)

	err := logStatsCmd.RunE(logStatsCmd, nil)

	actionable, ok := errors.AsType[*AgentActionableError](err)
	if !ok {
		t.Fatalf("error = %v, want an AgentActionableError rather than a decode failure", err)
	}
	require.Equal(t, "AUTHENTICATION_FAILED", actionable.ErrorCode)
	require.Equal(t, CodeAuthFail, actionable.Code)
}

func TestLogStatsKeepsTableOutput(t *testing.T) {
	server := logStatsFixture(t)
	buffer := withLogStatsContext(t, server.URL, output.FormatTable)

	if err := logStatsCmd.RunE(logStatsCmd, nil); err != nil {
		t.Fatalf("log stats: %v", err)
	}
	got := buffer.String()
	for _, want := range []string{
		"Log Statistics (Recent 1000 events)",
		"Level breakdown:",
		"info",
		"error",
		"list_employees",
		"get_invoice",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q:\n%s", want, got)
		}
	}
}

func TestLogStatsEmitsStructuredCounts(t *testing.T) {
	server := logStatsFixture(t)
	buffer := withLogStatsContext(t, server.URL, output.FormatJSON)

	if err := logStatsCmd.RunE(logStatsCmd, nil); err != nil {
		t.Fatalf("log stats: %v", err)
	}

	var payload struct {
		LevelCounts map[string]int `json:"levelCounts"`
		ToolCounts  map[string]int `json:"toolCounts"`
	}
	if err := json.Unmarshal(buffer.Bytes(), &payload); err != nil {
		t.Fatalf("structured output is not JSON: %v\n%s", err, buffer.String())
	}
	if payload.LevelCounts["info"] != 2 || payload.LevelCounts["error"] != 1 {
		t.Fatalf("levelCounts = %v, want info:2 error:1", payload.LevelCounts)
	}
	if payload.ToolCounts["list_employees"] != 2 || payload.ToolCounts["get_invoice"] != 1 {
		t.Fatalf("toolCounts = %v, want list_employees:2 get_invoice:1", payload.ToolCounts)
	}
	if strings.Contains(buffer.String(), "raw one") || strings.Contains(buffer.String(), "raw two") {
		t.Fatalf("structured output leaked a log message: %s", buffer.String())
	}
}
