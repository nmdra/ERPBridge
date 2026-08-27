package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestRateLimitMiddleware(t *testing.T) {
	// 2 req/s, burst 1
	m := NewRateLimitMiddleware(2, 1)
	handler := m.Handle()(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = testString

	// First request - allowed
	res, err := handler(ctx, req)
	assert.NoError(t, err)
	assert.False(t, res.IsError)

	// Second request - blocked (burst is 1)
	res, err = handler(ctx, req)
	assert.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content[0].(mcp.TextContent).Text, "rate limit exceeded")
}

func TestRateLimitMiddleware_EvictsIdleEntries(t *testing.T) {
	m := NewRateLimitMiddleware(2, 1)
	now := time.Date(2026, time.August, 22, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	m.limiters = make(map[string]*limiterEntry, maxLimiterEntries)
	for i := 0; i < maxLimiterEntries; i++ {
		m.limiters[fmt.Sprintf("old-%d", i)] = &limiterEntry{
			limiter:  rate.NewLimiter(rate.Limit(1), 1),
			lastSeen: now.Add(-limiterIdleTTL - time.Second),
		}
	}

	m.getLimiter("new")
	assert.Len(t, m.limiters, 1)
}

func TestRateLimitMiddleware_UsesPrincipalWhenProvided(t *testing.T) {
	m := NewRateLimitMiddleware(1, 1)
	handler := m.Handle()(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	req := mcp.CallToolRequest{}

	principalA := WithRateLimitPrincipal(context.Background(), "principal-a")
	_, err := handler(principalA, req)
	assert.NoError(t, err)
	blocked, err := handler(principalA, req)
	assert.NoError(t, err)
	assert.True(t, blocked.IsError)

	principalB := WithRateLimitPrincipal(context.Background(), "principal-b")
	allowed, err := handler(principalB, req)
	assert.NoError(t, err)
	assert.False(t, allowed.IsError)
}

func TestLoggingMiddlewareRedactsToolArguments(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	handler := LoggingMiddleware(log)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})

	req := mcp.CallToolRequest{}
	req.Params.Name = "test-tool"
	req.Params.Arguments = map[string]any{
		"password": "p123",
		"nested":   map[string]any{"token": "t456"},
		"safe":     "ok",
	}

	_, err := handler(context.Background(), req)

	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if strings.Contains(output, "p123") || strings.Contains(output, "t456") {
		t.Fatalf("tool argument log contains unredacted secret: %s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("tool argument log does not contain redaction marker: %s", output)
	}
}

func TestCacheMiddleware_PreservesMCPResult(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(10, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "cached-tool", Version: testVersion100},
		Spec:     ToolSpec{Cache: &cache.Config{Enabled: true}},
	}
	called := 0
	next := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called++
		return &mcp.CallToolResult{
			Content:           []mcp.Content{mcp.TextContent{Type: textContentType, Text: "cached"}},
			StructuredContent: map[string]any{testValue: "cached"},
		}, nil
	}
	handler := s.CacheMiddleware(tool)(next)
	req := mcp.CallToolRequest{}
	req.Params.Name = tool.Metadata.Name
	req.Params.Arguments = map[string]any{"id": "1"}

	first, err := handler(context.Background(), req)
	assert.NoError(t, err)
	second, err := handler(context.Background(), req)
	assert.NoError(t, err)
	firstJSON, marshalErr := json.Marshal(first)
	assert.NoError(t, marshalErr)
	secondJSON, marshalErr := json.Marshal(second)
	assert.NoError(t, marshalErr)
	assert.JSONEq(t, string(firstJSON), string(secondJSON))
	assert.Equal(t, 1, called)
}

func TestCacheMiddleware_DoesNotCacheErrorResults(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(10, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "error-cached-tool", Version: testVersion100},
		Spec:     ToolSpec{Cache: &cache.Config{Enabled: true}},
	}
	calls := 0
	next := func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls++
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: textContentType, Text: "safe error"}}, IsError: true}, nil
	}
	handler := s.CacheMiddleware(tool)(next)
	req := mcp.CallToolRequest{}
	req.Params.Name = tool.Metadata.Name
	req.Params.Arguments = map[string]any{"id": "1"}

	_, err := handler(context.Background(), req)
	assert.NoError(t, err)
	_, err = handler(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestCacheMiddleware_BypassesFileBackedCredentials(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(10, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "file-cached-tool", Version: testVersion100},
		Spec: ToolSpec{
			Cache: &cache.Config{Enabled: true},
			Security: Security{
				CredentialRef:    "ERP_FILE_CACHE_KEY", // #nosec G101 -- logical credential reference, not a secret.
				CredentialSource: "file",
			},
		},
	}
	called := 0
	handler := s.CacheMiddleware(tool)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called++
		return mcp.NewToolResultText("fresh"), nil
	})
	req := mcp.CallToolRequest{}
	req.Params.Name = tool.Metadata.Name
	req.Params.Arguments = map[string]any{"id": "1"}
	_, err := handler(context.Background(), req)
	assert.NoError(t, err)
	_, err = handler(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 2, called)
}

func TestCacheMiddleware_UsesCurrentToolSourceMetadata(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(10, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	current := &Tool{
		Metadata: Metadata{Name: "updated-cache-tool", Version: testVersion100, IsActive: true},
		Spec: ToolSpec{
			Cache: &cache.Config{Enabled: true},
			Security: Security{
				CredentialRef:    "ERP_UPDATED_CACHE_KEY", // #nosec G101 -- logical credential reference, not a secret.
				CredentialSource: "file",
			},
		},
	}
	require.NoError(t, s.registry.Add(current))
	stale := &Tool{
		Metadata: Metadata{Name: current.Metadata.Name, Version: current.Metadata.Version},
		Spec:     ToolSpec{Cache: &cache.Config{Enabled: true}},
	}
	called := 0
	handler := s.CacheMiddleware(stale)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called++
		return mcp.NewToolResultText("fresh"), nil
	})
	req := mcp.CallToolRequest{}
	req.Params.Name = current.Metadata.Name
	req.Params.Arguments = map[string]any{"id": "1"}
	_, err := handler(context.Background(), req)
	assert.NoError(t, err)
	_, err = handler(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 2, called)
}

func TestCacheMiddleware_BypassesFileBackedPluginCredentials(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(10, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "plugin-file-cache-tool", Version: testVersion100, IsActive: true},
		Spec:     ToolSpec{Cache: &cache.Config{Enabled: true}},
	}
	plugin := &Plugin{
		Metadata: PluginMetadata{Name: "file-plugin", Version: testVersion100, IsActive: true},
		Spec: PluginSpec{Auth: &PluginAuth{
			Type:             PluginAuthTypeBearer,
			CredentialRef:    "PLUGIN_FILE_CACHE_KEY", // #nosec G101 -- logical credential reference, not a secret.
			CredentialSource: "file",
		}},
	}
	s.pluginRegistry.Replace(map[string][]*ActivePluginBinding{
		tool.Metadata.Name + "@" + tool.Metadata.Version: {{
			Binding: &PluginBinding{Spec: PluginBindingSpec{
				ToolRef: ToolRef{Name: tool.Metadata.Name, Version: tool.Metadata.Version},
			}},
			Plugin: plugin,
		}},
	})
	called := 0
	handler := s.CacheMiddleware(tool)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called++
		return mcp.NewToolResultText("fresh"), nil
	})
	_, err := handler(context.Background(), mcp.CallToolRequest{})
	assert.NoError(t, err)
	_, err = handler(context.Background(), mcp.CallToolRequest{})
	assert.NoError(t, err)
	assert.Equal(t, 2, called)
}

func TestCacheMiddleware_FlushesOnDisabledWrite(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(10, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	readTool := &Tool{
		Metadata: Metadata{Name: "read-tool", Version: testVersion100},
		Spec:     ToolSpec{Cache: &cache.Config{Enabled: true}},
	}
	readCalls := 0
	readHandler := s.CacheMiddleware(readTool)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		readCalls++
		return mcp.NewToolResultText("fresh"), nil
	})
	req := mcp.CallToolRequest{}
	req.Params.Name = readTool.Metadata.Name
	req.Params.Arguments = map[string]any{"id": "1"}
	_, err := readHandler(context.Background(), req)
	assert.NoError(t, err)
	_, err = readHandler(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 1, readCalls)

	writeTool := &Tool{
		Metadata: Metadata{Name: "write-tool", Version: testVersion100},
		Spec:     ToolSpec{Cache: &cache.Config{FlushOn: []string{"read-tool"}}},
	}
	writeHandler := s.CacheMiddleware(writeTool)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("written"), nil
	})
	writeReq := mcp.CallToolRequest{}
	writeReq.Params.Name = writeTool.Metadata.Name
	_, err = writeHandler(context.Background(), writeReq)
	assert.NoError(t, err)

	_, err = readHandler(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 2, readCalls)
}
