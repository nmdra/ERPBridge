package logger

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

const testSafeString = "safe"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"WARN", slog.LevelWarn},
		{"Error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		if got := parseLevel(tt.input); got != tt.expected {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestNewRequestID(t *testing.T) {
	id1 := NewRequestID()
	id2 := NewRequestID()

	if id1 == id2 {
		t.Errorf("Expected unique IDs, got %v and %v", id1, id2)
	}

	if len(id1) < 5 {
		t.Errorf("Expected ID to have prefix and hex suffix, got %v", id1)
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	ch := Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	Unsubscribe(ch)

	// Verify channel is closed
	_, ok := <-ch
	if ok {
		t.Error("Expected channel to be closed after Unsubscribe")
	}
}

func TestBroadcastHandler(t *testing.T) {
	// Reset global state for test
	listenersMu.Lock()
	logListeners = nil
	logBuffer = nil
	listenersMu.Unlock()

	ch := Subscribe()
	defer Unsubscribe(ch)

	l := Init()
	l.Info("test message")

	// Check buffer
	recent := GetRecentLogs()
	if len(recent) != 1 {
		t.Errorf("Expected 1 log in buffer, got %d", len(recent))
	}

	// Check broadcast
	select {
	case msg := <-ch:
		if len(msg) == 0 {
			t.Error("Received empty message from subscriber")
		}
	default:
		t.Error("Subscriber did not receive the broadcast message")
	}
}

func TestComponent(t *testing.T) {
	root := slog.Default()
	comp := Component(root, "test-comp")

	if comp == nil {
		t.Fatal("Component returned nil logger")
	}

	// Test override via environment
	t.Setenv("LOG_LEVEL_OVERRIDE", "debug")

	compOverride := Component(root, "override")
	if compOverride == nil {
		t.Fatal("Component with override returned nil logger")
	}
}

func TestRedactArgs(t *testing.T) {
	input := map[string]any{
		"username":          "alice",
		redactedPasswordKey: "p123",
		"nested": map[string]any{
			"access_token": "bearer-secret",
			testSafeString: "value",
		},
		"items": []any{map[string]any{redactedAPIKey: "key-secret"}},
	}

	redacted, ok := RedactArgs(input).(map[string]any)
	if !ok {
		t.Fatalf("RedactArgs returned %T, want map[string]any", RedactArgs(input))
	}

	assertRedacted := func(value any, name string) {
		if value != redactionMarker {
			t.Errorf("%s = %v, want %s", name, value, redactionMarker)
		}
	}
	assertRedacted(redacted[redactedPasswordKey], redactedPasswordKey)
	assertRedacted(redacted["nested"].(map[string]any)["access_token"], "nested access_token")
	assertRedacted(redacted["items"].([]any)[0].(map[string]any)[redactedAPIKey], "items api_key")
	if redacted["username"] != "alice" {
		t.Errorf("safe value changed: %v", redacted["username"])
	}
}

func TestRedactHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer secret-token"},
		"X-Trace-ID":    []string{"trace-1"},
	}

	redacted := RedactHeaders(headers)
	if got := redacted.Get("Authorization"); got != redactionMarker {
		t.Fatalf("authorization header was not redacted: %q", got)
	}
	if got := redacted.Get("X-Trace-ID"); got != "trace-1" {
		t.Fatalf("safe header changed: %q", got)
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-token") {
		t.Fatalf("redacted headers contain the original token: %v", redacted)
	}
}

func TestBroadcastHandlerRedactsSensitiveValues(t *testing.T) {
	listenersMu.Lock()
	logListeners = nil
	logBuffer = nil
	listenersMu.Unlock()

	log := Init()
	log.Info("sensitive event", slog.Any("arguments", map[string]any{
		redactedPasswordKey: "p123",
		testSafeString:      "ok",
	}))

	recent := GetRecentLogs()
	if len(recent) == 0 {
		t.Fatal("expected a buffered log record")
	}
	var payload map[string]any
	if err := json.Unmarshal(recent[len(recent)-1], &payload); err != nil {
		t.Fatal(err)
	}
	encoded := string(recent[len(recent)-1])
	if strings.Contains(encoded, "p123") {
		t.Fatalf("broadcast log contains unredacted secret: %s", encoded)
	}
	if payload["msg"] != "sensitive event" {
		t.Fatalf("unexpected log payload: %v", payload)
	}
}
