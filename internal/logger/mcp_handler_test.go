// internal/logger/mcp_handler_test.go
package logger

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/types"
)

func TestMCPHandler_Redaction(t *testing.T) {
	// We don't need a real server to test redaction, just the handler's internal buffer
	srv := server.NewMCPServer("test", "1.0.0")
	h := NewMCPHandler(srv, "test-logger")

	// Create a mock session and add it to context
	notifCh := make(chan mcp.JSONRPCNotification, 10)
	sess := &mockFullSession{
		level: mcp.LoggingLevelDebug,
		notif: notifCh,
	}
	ctx := srv.WithContext(context.Background(), sess)

	type testStruct struct {
		Token types.APIToken
		PII   types.PII
		Plain string
		Key   string `masq:"secret"`
	}

	data := testStruct{
		Token: types.APIToken("secret-123"),
		PII:   types.PII("alice@example.com"),
		Plain: "normal-data",
		Key:   "private-key",
	}

	record := slog.Record{
		Level:   slog.LevelInfo,
		Message: "test message",
	}
	record.AddAttrs(slog.Any("data", data))
	record.AddAttrs(slog.String(redactedPasswordKey, "p123")) // Exact field name redaction
	record.AddAttrs(slog.String("authorization", "Bearer mcp-log-secret"))

	if err := h.Handle(ctx, record); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	output := h.buf.String()

	// Check for redactions
	if strings.Contains(output, "secret-123") {
		t.Errorf("Output contains unredacted Token: %s", output)
	}
	if strings.Contains(output, "alice@example.com") {
		t.Errorf("Output contains unredacted PII: %s", output)
	}
	if strings.Contains(output, "private-key") {
		t.Errorf("Output contains unredacted Key: %s", output)
	}
	if strings.Contains(output, "p123") {
		t.Errorf("Output contains unredacted password: %s", output)
	}
	if strings.Contains(output, "mcp-log-secret") {
		t.Errorf("Output contains unredacted authorization value: %s", output)
	}

	// Verify [REDACTED] is present (masq default)
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("Output does not contain [REDACTED]: %s", output)
	}

	// Verify plain data is preserved
	if !strings.Contains(output, "normal-data") {
		t.Errorf("Output missing plain data: %s", output)
	}

	select {
	case notification := <-notifCh:
		encoded, err := json.Marshal(notification)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "mcp-log-secret") {
			t.Fatalf("MCP notification contains unredacted authorization value: %s", encoded)
		}
	default:
		t.Fatal("expected an MCP log notification")
	}

	// Verify map redaction
	h.buf.Reset()
	m := map[string]any{
		redactedAPIKey: "key-456",
		"other":        testSafeString,
	}
	record2 := slog.Record{Level: slog.LevelInfo, Message: "map test"}
	record2.AddAttrs(slog.Any("map", m))
	if err := h.Handle(ctx, record2); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	output2 := h.buf.String()
	if strings.Contains(output2, "key-456") {
		t.Errorf("Map api_key not redacted: %s", output2)
	}
}

func TestMultiHandler(t *testing.T) {
	var h1Called, h2Called bool

	h1 := &mockHandler{enabled: true, handleFunc: func() { h1Called = true }}
	h2 := &mockHandler{enabled: true, handleFunc: func() { h2Called = true }}

	m := MultiHandler{h1, h2}

	ctx := context.Background()
	record := slog.Record{Level: slog.LevelInfo, Message: "multi test"}

	if !m.Enabled(ctx, slog.LevelInfo) {
		t.Error("MultiHandler should be enabled if at least one sub-handler is enabled")
	}

	if err := m.Handle(ctx, record); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if !h1Called || !h2Called {
		t.Errorf("MultiHandler failed to fan out: h1Called=%v, h2Called=%v", h1Called, h2Called)
	}
}

type mockHandler struct {
	enabled    bool
	handleFunc func()
}

func (h *mockHandler) Enabled(context.Context, slog.Level) bool { return h.enabled }
func (h *mockHandler) Handle(context.Context, slog.Record) error {
	if h.handleFunc != nil {
		h.handleFunc()
	}
	return nil
}
func (h *mockHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *mockHandler) WithGroup(_ string) slog.Handler      { return h }

type mockFullSession struct {
	server.ClientSession
	level mcp.LoggingLevel
	notif chan mcp.JSONRPCNotification
}

func (s *mockFullSession) SetLogLevel(l mcp.LoggingLevel) { s.level = l }
func (s *mockFullSession) GetLogLevel() mcp.LoggingLevel  { return s.level }
func (s *mockFullSession) Initialized() bool              { return true }
func (s *mockFullSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return s.notif
}
func (s *mockFullSession) SessionID() string { return "test-session" }

func TestMCPHandler_Enabled(t *testing.T) {
	// This test is harder because server.ClientSessionFromContext depends on internal mcp-go state.
	// But we can at least verify it returns true when no session is present.
	srv := server.NewMCPServer("test", "1.0.0")
	h := NewMCPHandler(srv, "test")

	if !h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Enabled should return true when no session is in context")
	}
}
