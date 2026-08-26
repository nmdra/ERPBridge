package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_ToolAPI(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	t.Run("Apply Valid Tool", func(t *testing.T) {
		toolJSON := testToolJSON
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"applied"`)
	})

	t.Run("Apply Invalid Tool - Bad JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(`{bad`))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Apply Invalid Tool - Validation Failed (Missing Name)", func(t *testing.T) {
		toolJSON := `{"metadata":{"name":"","version":"1.0.0"}}`
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("Apply Invalid Tool - Validation Failed (Missing Version)", func(t *testing.T) {
		toolJSON := `{"metadata":{"name":"test-tool","version":""}}`
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("Apply Invalid Tool - Validation Failed (Secrets key)", func(t *testing.T) {
		toolJSON := `{"metadata":{"name":"test-sec","version":"1.0.0"},"spec":{"description":{"short":"test"},"execution":{"endpoint":"http://a.com?key=123"}}}`
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("Apply Invalid Tool - Validation Failed (Secrets token)", func(t *testing.T) {
		toolJSON := `{"metadata":{"name":"test-sec-token","version":"1.0.0"},"spec":{"description":{"short":"test"},"execution":{"endpoint":"http://a.com token 123"}}}`
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("Apply Invalid Tool - Validation Failed (HTTP verbs)", func(t *testing.T) {
		toolJSON := `{"metadata":{"name":"get-users","version":"1.0.0"}}`
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("List Tools", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/apis/erpbridge.io/v1/tools", nil)
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"name":"test-tool"`)
	})

	t.Run("List Tools with exact filters", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/apis/erpbridge.io/v1/tools?name=test-tool&version=1.0.0", nil)
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"name":"test-tool"`)
	})

	t.Run("Delete Tool", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/apis/erpbridge.io/v1/tools?name=test-tool&version=1.0.0", nil)
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("Delete Tool - Missing Params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/apis/erpbridge.io/v1/tools", nil)
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/apis/erpbridge.io/v1/tools", nil)
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestServer_Reconcile_And_Deregister(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	tool := &Tool{
		Metadata: Metadata{Name: "recon-tool", Version: testVersion100, IsActive: true},
		Spec:     ToolSpec{Description: Description{Short: testDescShort}},
	}
	err := s.store.Save(tool)
	assert.NoError(t, err)

	s.Reconcile(context.Background())
	regTool, err := s.registry.Resolve("recon-tool", testVersion100)
	assert.NoError(t, err)
	assert.NotNil(t, regTool)

	s.Reconcile(context.Background())
	err = s.store.Delete("recon-tool", testVersion100)
	assert.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
	s.Reconcile(context.Background())
	_, err = s.registry.Resolve("recon-tool", testVersion100)
	assert.Error(t, err)
	s.DeregisterTool("nonexistent", testVersion100)
}

func TestServer_StartController(_ *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	ctx, cancel := context.WithCancel(context.Background())
	go s.StartController(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestServer_HandleMCPToolCall(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	tool := &Tool{
		Metadata: Metadata{Name: "mcp-tool", Version: testVersion100},
		Spec:     ToolSpec{Description: Description{Short: testDescShort}},
		Handler: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{testStatusField: testStatusOk}}, nil
		},
	}
	s.RegisterTool(tool)

	handler := s.handleMCPToolCall("mcp-tool")
	req := mcp.CallToolRequest{}
	req.Params.Name = "mcp-tool"
	req.Params.Arguments = map[string]any{"arg1": "val1"}

	res, err := handler(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.False(t, res.IsError)
	assert.Len(t, res.Content, 1)
	textRes := res.Content[0].(mcp.TextContent)
	assert.Contains(t, textRes.Text, `"status":"ok"`)

	req.Params.Arguments = "invalid string instead of map"
	_, err = handler(context.Background(), req)
	assert.Error(t, err)

	handlerNotFound := s.handleMCPToolCall("nonexistent-tool")
	req.Params.Name = "nonexistent-tool"
	req.Params.Arguments = map[string]any{}
	_, err = handlerNotFound(context.Background(), req)
	assert.Error(t, err)
}

func TestServer_HandleMCPToolCall_ExecuteError(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	tool := &Tool{
		Metadata: Metadata{Name: "err-tool", Version: testVersion100},
		Spec:     ToolSpec{Description: Description{Short: testDescShort}},
		Handler: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return nil, fmt.Errorf("execute error")
		},
	}
	s.RegisterTool(tool)

	handler := s.handleMCPToolCall("err-tool")
	req := mcp.CallToolRequest{}
	req.Params.Name = "err-tool"
	req.Params.Arguments = map[string]any{}

	res, err := handler(context.Background(), req)
	assert.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Len(t, res.Content, 1)
	textRes := res.Content[0].(mcp.TextContent)
	assert.Contains(t, textRes.Text, "execute error")
}

func TestServer_ToolAPINoStore(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, "/invalid/path/db")
	s.store = nil

	t.Run("Apply Tool - No Store", func(t *testing.T) {
		toolJSON := testToolJSON
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), `store not available`)
	})

	t.Run("Reconcile - No Store", func(_ *testing.T) {
		s.Reconcile(context.Background())
	})
}

func TestServer_StoreErrors(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	_ = s.store.Close()

	t.Run("Apply Tool - DB Error", func(t *testing.T) {
		toolJSON := testToolJSON
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(toolJSON))
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("List Tools - DB Error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/apis/erpbridge.io/v1/tools", nil)
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Delete Tool - DB Error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/apis/erpbridge.io/v1/tools?name=test&version=1", nil)
		w := httptest.NewRecorder()
		s.handleToolAPI(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Reconcile - DB Error", func(_ *testing.T) {
		s.Reconcile(context.Background())
	})
}

func TestServerAPIProbeResolvesCredentialAndReturnsBoundedSummary(t *testing.T) {
	t.Setenv(authTokenEnv, "admin-token")
	const probeValue = "probe-value" // #nosec G101 -- test-only credential sentinel.
	t.Setenv("ERP_PROBE_KEY", probeValue)

	erp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+probeValue, r.Header.Get("X-Probe-Token"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Upstream-Secret", "do-not-forward")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"secret":"erp-body"}`))
	}))
	defer erp.Close()
	setProbeInsecureHost(t, erp.URL)

	s := NewServer(connector.NewClient(logger.Init()), nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")

	// #nosec G101 -- test-only environment reference, not a credential value.
	payload, err := json.Marshal(APIProbeRequest{
		URL:           erp.URL,
		Method:        http.MethodGet,
		AuthType:      "bearer",
		AuthHeader:    "X-Probe-Token",
		CredentialRef: "ERP_PROBE_KEY",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, apiProbePath, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.NotContains(t, body, probeValue)
	require.NotContains(t, body, "erp-body")
	require.NotContains(t, body, "do-not-forward")
	var response APIProbeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, http.StatusCreated, response.Status)
	require.Equal(t, "application/json", response.ContentType)
	require.GreaterOrEqual(t, response.Latency, int64(0))
	require.True(t, response.Success)
}

func TestServerAPIProbeIsAdminOnly(t *testing.T) {
	t.Setenv(authTokenEnv, "admin-token")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")

	req := httptest.NewRequest(http.MethodPost, apiProbePath, strings.NewReader(`{"url":"http://127.0.0.1:1","method":"GET"}`))
	req.Header.Set("Authorization", "Bearer not-admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestToolPrepareERPCallRewritesOnlyExactLocalhost(t *testing.T) {
	t.Setenv("ERP_BASE_URL", "https://configured.erp.test")
	for _, test := range []struct {
		name string
		url  string
		want string
	}{
		{name: "localhost", url: "http://localhost:8081/invoices", want: "https://configured.erp.test/invoices"},
		{name: "localhost with suffix", url: "http://localhost.example/invoices", want: "http://localhost.example/invoices"},
		{name: "loopback", url: "http://127.0.0.1:8081/invoices", want: "https://configured.erp.test/invoices"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := &Tool{Spec: ToolSpec{Execution: Execution{Method: http.MethodGet, Endpoint: test.url}}}
			ep, _, _, err := tool.prepareERPCall(nil)
			require.NoError(t, err)
			require.Equal(t, test.want, ep.Path)
		})
	}
}

func TestServerAPIProbeDoesNotFollowCredentialedRedirect(t *testing.T) {
	t.Setenv(authTokenEnv, "admin-token")
	t.Setenv("ERP_PROBE_KEY", "redirect-value")
	finalCalls := 0
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalCalls++
		if r.Header.Get("Authorization") != "" {
			t.Errorf("redirect target received authorization")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	setProbeInsecureHost(t, redirect.URL)

	s := NewServer(connector.NewClient(logger.Init()), nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")
	// #nosec G101 -- test-only environment reference, not a credential value.
	payload, err := json.Marshal(APIProbeRequest{URL: redirect.URL, Method: http.MethodGet, AuthType: "bearer", CredentialRef: "ERP_PROBE_KEY"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, apiProbePath, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var response APIProbeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, http.StatusTemporaryRedirect, response.Status)
	require.False(t, response.Success)
	require.Equal(t, 0, finalCalls)
}

func setProbeInsecureHost(t *testing.T, rawURL string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	t.Setenv(security.InsecureAuthAllowedHostsEnv, u.Host)
}
