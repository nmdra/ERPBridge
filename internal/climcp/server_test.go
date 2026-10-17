package climcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	testInitRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	testInitialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	testPingRequest = `{"jsonrpc":"2.0","id":2,"method":"ping"}`
	testListRequest = `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`
)

func testSnapshot() Snapshot {
	return Snapshot{
		ContextName: "test",
		Server:      "http://127.0.0.1:1",
		MCPServer:   "http://127.0.0.1:2",
		APIToken:    "test-token",
	}
}

func newTestSession(t *testing.T, stdout io.Writer, stdin io.Reader, groups ...WriteGroup) *Server {
	t.Helper()
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	t.Cleanup(func() { _ = stager.Cleanup() })
	session, err := NewServer(Options{
		Snapshot: testSnapshot(),
		Version:  "test-version",
		Groups:   groups,
		Stdin:    stdin,
		Stdout:   stdout,
		Stager:   stager,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return session
}

func serveInput(t *testing.T, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	session := newTestSession(t, &out, strings.NewReader(input))
	err := session.Serve(context.Background())
	return out.String(), err
}

func decodeMessages(t *testing.T, raw string) []map[string]any {
	t.Helper()
	messages := make([]map[string]any, 0, 4)
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("protocol stdout line is not JSON-RPC: %q (%v)", line, err)
		}
		if message["jsonrpc"] != "2.0" {
			t.Fatalf("protocol stdout line is not JSON-RPC 2.0: %q", line)
		}
		messages = append(messages, message)
	}
	return messages
}

func responseFor(t *testing.T, messages []map[string]any, id float64) map[string]any {
	t.Helper()
	for _, message := range messages {
		if message["id"] == id {
			return message
		}
	}
	t.Fatalf("no response for id %v in %v", id, messages)
	return nil
}

func TestServeAnswersInitializePingAndDefaultToolList(t *testing.T) {
	input := strings.Join([]string{testInitRequest, testInitialized, testPingRequest, testListRequest}, "\n") + "\n"

	raw, err := serveInput(t, input)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	messages := decodeMessages(t, raw)
	if len(messages) != 3 {
		t.Fatalf("received %d messages, want 3 (the initialized notification has no response): %q", len(messages), raw)
	}

	initResult, ok := responseFor(t, messages, 1)["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize response had no result: %q", raw)
	}
	serverInfo, ok := initResult["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result had no serverInfo: %q", raw)
	}
	if serverInfo["name"] != ServerName {
		t.Fatalf("serverInfo.name = %v, want %q", serverInfo["name"], ServerName)
	}
	if serverInfo["version"] != "test-version" {
		t.Fatalf("serverInfo.version = %v, want %q", serverInfo["version"], "test-version")
	}

	if _, ok := responseFor(t, messages, 2)["result"]; !ok {
		t.Fatalf("ping response had no result: %q", raw)
	}

	listResult, ok := responseFor(t, messages, 3)["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list response had no result: %q", raw)
	}
	tools, ok := listResult["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result had no tools array: %q", raw)
	}
	advertised := make([]string, 0, len(tools))
	for _, entry := range tools {
		tool, _ := entry.(map[string]any)
		name, _ := tool["name"].(string)
		advertised = append(advertised, name)
	}
	if got, want := sortedCopy(advertised), sortedCopy(defaultToolNames()); !equalStrings(got, want) {
		t.Fatalf("tools/list advertised %v, want the default read catalogue %v", got, want)
	}
}

func TestServeReportsMalformedJSONAndContinues(t *testing.T) {
	input := strings.Join([]string{testInitRequest, `{"jsonrpc":`, testPingRequest}, "\n") + "\n"

	raw, err := serveInput(t, input)
	if err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	messages := decodeMessages(t, raw)
	if len(messages) != 3 {
		t.Fatalf("received %d messages, want 3: %q", len(messages), raw)
	}
	parseError, ok := messages[1]["error"].(map[string]any)
	if !ok {
		t.Fatalf("second message had no error object: %q", raw)
	}
	if code, _ := parseError["code"].(float64); code != -32700 {
		t.Fatalf("parse error code = %v, want -32700", parseError["code"])
	}
	if _, ok := responseFor(t, messages, 2)["result"]; !ok {
		t.Fatalf("session did not continue after a malformed message: %q", raw)
	}
}

func TestServeEndsWhenInputCloses(t *testing.T) {
	raw, err := serveInput(t, testInitRequest+"\n")
	if err != nil {
		t.Fatalf("Serve returned error on EOF: %v", err)
	}
	if len(decodeMessages(t, raw)) != 1 {
		t.Fatalf("unexpected output on EOF: %q", raw)
	}
}

func TestServeRejectsOversizedMessageAndEndsSession(t *testing.T) {
	oversized := `{"jsonrpc":"2.0","id":9,"method":"ping","params":{"pad":"` +
		strings.Repeat("x", MaxCallMessageBytes+16) + `"}}` + "\n"

	raw, err := serveInput(t, oversized)
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("Serve error = %v, want ErrMessageTooLarge", err)
	}
	if strings.Contains(raw, "xxxx") {
		t.Fatalf("oversized input was echoed to protocol stdout: %q", raw[:min(len(raw), 200)])
	}
}

func TestBoundedLineReaderLimitsOneMessage(t *testing.T) {
	reader := newBoundedLineReader(strings.NewReader("short\n"+strings.Repeat("y", 40)+"\n"), 16)

	buffer := make([]byte, 64)
	var seen error
	for range 8 {
		_, err := reader.Read(buffer)
		if err != nil {
			seen = err
			break
		}
	}
	if !errors.Is(seen, ErrMessageTooLarge) {
		t.Fatalf("Read error = %v, want ErrMessageTooLarge", seen)
	}
}

func TestServeStopsWhenContextIsCancelled(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	defer func() { _ = stdinWriter.Close() }()
	defer func() { _ = stdinReader.Close() }()

	out := &syncBuffer{}
	session := newTestSession(t, out, stdinReader)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Serve(ctx) }()

	if _, err := io.WriteString(stdinWriter, testInitRequest+"\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for out.String() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop after cancellation")
	}
}

func TestNewServerRejectsUnknownWriteGroup(t *testing.T) {
	_, err := NewServer(Options{Snapshot: testSnapshot(), Groups: []WriteGroup{"*"}})
	if err == nil {
		t.Fatal("NewServer accepted a wildcard write group")
	}
}

func TestRegisteredToolReturnsAdapterEnvelope(t *testing.T) {
	input := strings.Join([]string{testInitRequest, testInitialized,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"bridgectl_test_probe","arguments":{"value":"ok"}}}`}, "\n") + "\n"

	var out bytes.Buffer
	session := newTestSession(t, &out, strings.NewReader(input))
	session.register(mcp.NewTool("bridgectl_test_probe",
		mcp.WithDescription("Test-only probe"),
		mcp.WithString("value", mcp.Required()),
	), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return NewSuccess("bridgectl_test_probe", "test", map[string]any{
			"value": request.GetString("value", ""),
		}).callToolResult()
	})

	if err := session.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	messages := decodeMessages(t, out.String())
	callResult, ok := responseFor(t, messages, 4)["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call response had no result: %q", out.String())
	}
	if isError, _ := callResult["isError"].(bool); isError {
		t.Fatalf("test probe result was an error: %q", out.String())
	}
	structured, ok := callResult["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("result had no structuredContent: %q", out.String())
	}
	if structured["operation"] != "bridgectl_test_probe" || structured["success"] != true {
		t.Fatalf("structuredContent = %v, want a successful adapter envelope", structured)
	}
}
