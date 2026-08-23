package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

const (
	testVersion100  = "1.0.0"
	testVersion110  = "1.1.0"
	testTool1       = "tool1"
	testTool2       = "tool2"
	testToolName    = "test-tool"
	testToolJSON    = `{"metadata":{"name":"test-tool","version":"1.0.0"},"spec":{"description":{"short":"test"}}}`
	testDescShort   = "Test"
	testStatusOk    = "ok"
	testStatusField = "status"
	testEndpoint    = "/test"
	testURITemplate = "erp://test"
	testPromptName  = "test-prompt"
	testFieldName   = "name"
	testString      = "test"
	testValue       = "value"
)

func TestServer_RegisterResource(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	r := &Resource{
		Name:        "test-resource",
		Description: testDescShort,
		URITemplate: testURITemplate,
		MimeType:    "text/plain",
	}

	s.RegisterResource(r)

	assert.NotNil(t, s.resources[testURITemplate])
	assert.Equal(t, "test-resource", s.resources[testURITemplate].Name)
}

func TestServer_HandleResourceRead(t *testing.T) {
	log := logger.Init()

	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("resource content")),
			}, nil
		},
	}
	s := NewServer(mockConn, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	r := &Resource{
		Name:        "test-resource",
		URITemplate: testURITemplate,
		MimeType:    "text/plain",
		Execution:   Execution{Endpoint: testEndpoint},
	}
	s.RegisterResource(r)

	req := mcp.ReadResourceRequest{}
	req.Params.URI = testURITemplate

	res, err := s.handleMCPResourceRead(context.Background(), req)
	assert.NoError(t, err)
	assert.Len(t, res, 1)

	textRes := res[0].(mcp.TextResourceContents)
	assert.Equal(t, "resource content", textRes.Text)

	// test not found
	req.Params.URI = "erp://unknown"
	_, err = s.handleMCPResourceRead(context.Background(), req)
	assert.Error(t, err)
}

func TestServer_RegisterPrompt(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	p := &Prompt{
		Name:        testPromptName,
		Description: "Test prompt",
		Template:    "Do something",
		Arguments: []PromptArgument{
			{Name: "arg1", Description: "Arg 1", Required: true},
		},
	}

	s.RegisterPrompt(p)

	assert.NotNil(t, s.prompts[testPromptName])
	assert.Equal(t, testPromptName, s.prompts[testPromptName].Name)
}

func TestServer_HandlePromptGet(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	p := &Prompt{
		Name:     testPromptName,
		Template: "Hello {{name}}",
	}
	s.RegisterPrompt(p)

	req := mcp.GetPromptRequest{}
	req.Params.Name = testPromptName
	req.Params.Arguments = map[string]string{testFieldName: "world"}

	res, err := s.handleMCPPromptGet(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Len(t, res.Messages, 1)

	textContent := res.Messages[0].Content.(mcp.TextContent).Text
	assert.Contains(t, textContent, "Hello {{name}}")
	assert.Contains(t, textContent, "name: world")

	// test not found
	req.Params.Name = "unknown-prompt"
	_, err = s.handleMCPPromptGet(context.Background(), req)
	assert.Error(t, err)
}

func TestServer_Completions(t *testing.T) {
	rp := &ResourceCompletionProvider{}
	resComp, _ := rp.CompleteResourceArgument(context.Background(), "", mcp.CompleteArgument{}, mcp.CompleteContext{})
	assert.NotNil(t, resComp)
	assert.NotEmpty(t, resComp.Values)

	pp := &PromptCompletionProvider{}
	pComp, _ := pp.CompletePromptArgument(context.Background(), "", mcp.CompleteArgument{}, mcp.CompleteContext{})
	assert.NotNil(t, pComp)
	assert.NotEmpty(t, pComp.Values)
}

func TestServer_RegisterToolMarshaling(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	schema := InputSchema{
		Type: schemaTypeObject,
		Properties: map[string]Property{
			testString: {Type: schemaTypeString},
		},
	}

	s.RegisterTool(&Tool{
		Metadata: Metadata{
			Name:    testToolName,
			Version: testVersion100,
		},
		Spec: ToolSpec{
			Description: Description{Short: testDescShort},
			InputSchema: schema,
		},
	})

	// Access the registered tool from the registry
	tool, err := s.registry.Resolve(testToolName, "")
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Try to marshal the tools list as the server would during a tools/list request
	schemaJSON, _ := json.Marshal(schema)
	mcpTool := mcp.NewToolWithRawSchema("test-tool", "Test", json.RawMessage(schemaJSON))

	// Apply the same fix as in RegisterTool
	mcpTool.InputSchema = mcp.ToolInputSchema{}
	mcpTool.OutputSchema = mcp.ToolOutputSchema{}

	data, err := json.Marshal(mcpTool)
	assert.NoError(t, err, "Tool marshaling should not fail")
	assert.Contains(t, string(data), "\"inputSchema\":{\"type\":\"object\"")
}

func TestServer_RegisterToolInitializesMetrics(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	toolName := "issue-8-cold-start-tool"

	s.RegisterTool(&Tool{
		Metadata: Metadata{
			Name:    toolName,
			Version: testVersion100,
		},
		Spec: ToolSpec{
			Description: Description{Short: testDescShort},
			InputSchema: InputSchema{Type: schemaTypeObject},
		},
	})

	assertToolMetricSeriesInitialized(t, toolName, true)
}

func TestServer_NewServerInitializesBuiltinMetrics(t *testing.T) {
	log := logger.Init()
	NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	assertToolMetricSeriesInitialized(t, "system.progress_test", false)
	assertToolMetricSeriesInitialized(t, "system.sensitive_log_test", false)
}

func assertToolMetricSeriesInitialized(t *testing.T, toolName string, assertZero bool) {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	invocationInitialized := false
	durationInitialized := false
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			if family.GetName() == "mcp_tool_invocations_total" && labels["tool"] == toolName && labels["cache_status"] == "SUCCESS" {
				if assertZero {
					assert.Equal(t, float64(0), metric.GetCounter().GetValue())
				}
				invocationInitialized = true
			}
			if family.GetName() == "mcp_tool_duration_seconds" && labels["tool"] == toolName {
				if assertZero {
					assert.Equal(t, uint64(0), metric.GetHistogram().GetSampleCount())
				}
				durationInitialized = true
			}
		}
	}

	assert.True(t, invocationInitialized, "tool invocation metric should be exported before the first call")
	assert.True(t, durationInitialized, "tool duration metric should be exported before the first call")
}

func TestServer_ServeHTTP(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "http://localhost:8080")

	// test health endpoint
	req := httptest.NewRequest("GET", "/mcp/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"ok"`)
}

func TestServer_MCPBrowserCORS(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodOptions, "/mcp/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Mcp-Session-Id, MCP-Protocol-Version")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Less(t, w.Code, http.StatusMultipleChoices)
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "MCP-Protocol-Version")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Mcp-Session-Id")
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "Mcp-Session-Id")
}

func TestServer_MCPCORSPreflightRunsBeforeAuthentication(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "http://localhost:8080")

	preflight := httptest.NewRequest(http.MethodOptions, "/mcp/", nil)
	preflight.Header.Set("Origin", "http://localhost:3000")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, preflight)
	assert.Less(t, recorder.Code, http.StatusMultipleChoices)
	assert.Equal(t, "http://localhost:3000", recorder.Header().Get("Access-Control-Allow-Origin"))

	request := httptest.NewRequest(http.MethodGet, "/mcp/", nil)
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestServer_LogStream(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	req := httptest.NewRequest("GET", "/api/logs/stream", nil)

	// Create a context that will cancel quickly so the stream loop exits
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Trigger a log in a goroutine so it gets captured while streaming
	go func() {
		time.Sleep(10 * time.Millisecond)
		log.Info("stream log msg")
	}()

	s.handleLogStream(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "stream log msg")
}

func TestServer_HttpEndpoints(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	assert.NotNil(t, s.MCPServer())

	// Test cache flush (not enabled)
	req := httptest.NewRequest("POST", "/api/cache/flush?all=true", nil)
	w := httptest.NewRecorder()
	s.handleCacheFlush(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Test cache stats (not enabled)
	req = httptest.NewRequest("GET", "/api/cache/stats", nil)
	w = httptest.NewRecorder()
	s.handleCacheStats(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Enable cache with a dummy manager to cover parameter validation
	s.cache = cache.NewManager(nil, log)

	req = httptest.NewRequest("POST", "/api/cache/flush", nil)
	w = httptest.NewRecorder()
	s.handleCacheFlush(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Log recent
	log.Info("test log msg")
	time.Sleep(100 * time.Millisecond)
	req = httptest.NewRequest("GET", "/api/logs/recent", nil)
	w = httptest.NewRecorder()
	s.handleLogRecent(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "test log msg")
}

func TestServer_DirectInvoke(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	s.RegisterTool(&Tool{
		Metadata: Metadata{
			Name:    "test-invoke",
			Version: testVersion100,
		},
		Handler: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{"ok": true}}, nil
		},
	})

	// GET not allowed
	req := httptest.NewRequest(http.MethodGet, "/api/tools/invoke", nil)
	w := httptest.NewRecorder()
	s.handleDirectInvoke(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	// Valid POST
	body := `{"name":"test-invoke","arguments":{}}`
	req = httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	s.handleDirectInvoke(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ok":true`)

	// Tool not found
	bodyNotFound := `{"name":"unknown","arguments":{}}`
	req = httptest.NewRequest("POST", "/api/tools/invoke", bytes.NewBufferString(bodyNotFound))
	w = httptest.NewRecorder()
	s.handleDirectInvoke(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServer_HandleToolDelete(t *testing.T) {
	log := logger.Init()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, dbPath)

	tool := &Tool{
		Metadata: Metadata{
			Name:    "delete-me",
			Version: testVersion100,
			Module:  "test",
		},
	}
	s.RegisterTool(tool)
	_ = s.store.Save(tool)

	t.Run("MissingParams", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/apis/erpbridge.io/v1/tools", nil)
		w := httptest.NewRecorder()
		s.handleToolDelete(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("SoftDelete", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/apis/erpbridge.io/v1/tools?name=delete-me&version="+testVersion100, nil)
		w := httptest.NewRecorder()
		s.handleToolDelete(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify soft-deleted in store
		dbTool, _ := s.store.Get("delete-me", testVersion100)
		assert.False(t, dbTool.Metadata.IsActive)
	})

	t.Run("HardDelete", func(t *testing.T) {
		// Re-register and re-save
		tool.Metadata.IsActive = true
		s.RegisterTool(tool)
		_ = s.store.Save(tool)

		req := httptest.NewRequest("DELETE", "/apis/erpbridge.io/v1/tools?name=delete-me&version="+testVersion100+"&hard=true", nil)
		w := httptest.NewRecorder()
		s.handleToolDelete(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify hard-deleted from store
		_, err := s.store.Get("delete-me", testVersion100)
		assert.Error(t, err)
	})
}

func TestServer_HandleCacheFlushModuleIncludesInactiveTools(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(20, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	for _, tool := range []*Tool{
		{Metadata: Metadata{Name: "finance-active", Version: testVersion100, Module: "finance", IsActive: true}},
		{Metadata: Metadata{Name: "finance-inactive", Version: testVersion110, Module: "finance", IsActive: false}},
		{Metadata: Metadata{Name: "hr-active", Version: testVersion100, Module: "hr", IsActive: true}},
	} {
		assert.NoError(t, s.store.Save(tool))
	}
	cfg := cache.Config{Enabled: true}
	assert.NoError(t, s.cache.Set(context.Background(), "finance-active", "", map[string]any{"id": 1}, []byte(`{"ok":true}`), cfg))
	assert.NoError(t, s.cache.Set(context.Background(), "finance-inactive", "", map[string]any{"id": 1}, []byte(`{"ok":true}`), cfg))
	assert.NoError(t, s.cache.Set(context.Background(), "hr-active", "", map[string]any{"id": 1}, []byte(`{"ok":true}`), cfg))

	req := httptest.NewRequest(http.MethodGet, "/api/cache/flush?module=finance", nil)
	w := httptest.NewRecorder()
	s.handleCacheFlush(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"deleted":2`)
	financeKeys, err := s.cache.Get(context.Background(), "finance-active", "", map[string]any{"id": 1}, cfg)
	assert.NoError(t, err)
	assert.Equal(t, "miss", financeKeys.HitType)
	hrKeys, err := s.cache.Get(context.Background(), "hr-active", "", map[string]any{"id": 1}, cfg)
	assert.NoError(t, err)
	assert.Equal(t, "exact", hrKeys.HitType)
}
