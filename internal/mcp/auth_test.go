package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthHandler_OpenModeAllowsRoute(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	handler := s.AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := CallerIdentityFromContext(r.Context())
		assert.False(t, ok)
		w.WriteHeader(http.StatusNoContent)
	}), "mcp", false)

	req := httptest.NewRequest(http.MethodGet, "/mcp/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestAuthHandler_ValidatesAdminAndScopedToken(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	t.Setenv("API_AUTH_ADMIN_ROLES", "operator,admin")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	record, raw, err := s.store.CreateToken(TokenCreateRequest{
		Name:   "mcp-client",
		Scopes: []string{scopeMCP},
		Roles:  []string{testRoleOperator},
	})
	require.NoError(t, err)

	protected := s.AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := CallerIdentityFromContext(r.Context())
		require.True(t, ok)
		assert.Equal(t, record.ID, identity.PrincipalID)
		assert.Equal(t, []string{testRoleOperator}, identity.Roles)
		assert.False(t, identity.IsAdmin)
		assert.Equal(t, record.ID, rateLimitPrincipal(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	}), scopeMCP, false)

	for name, authorization := range map[string]string{
		"missing":        "",
		"invalid scheme": "Basic " + raw,
		"invalid token":  "Bearer invalid",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp/", nil)
			req.Header.Set("Authorization", authorization)
			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/mcp/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	adminOnly := s.AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "", true)
	req = httptest.NewRequest(http.MethodGet, "/api/tools/invoke", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec = httptest.NewRecorder()
	adminOnly.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/tools/invoke", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	adminOnly.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestAuthHandler_RejectsMissingScope(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	_, raw, err := s.store.CreateToken(TokenCreateRequest{Name: "logs-client", Scopes: []string{"logs"}})
	require.NoError(t, err)

	handler := s.AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), scopeMCP, false)
	req := httptest.NewRequest(http.MethodGet, "/mcp/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAuthHandler_RejectsRevokedAndExpiredTokens(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	protected := s.AuthHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), scopeMCP, false)

	revokedRecord, revokedRaw, err := s.store.CreateToken(TokenCreateRequest{Name: "revoked", Scopes: []string{scopeMCP}})
	require.NoError(t, err)
	require.NoError(t, s.store.RevokeToken(revokedRecord.ID))

	expiredRecord, expiredRaw, err := s.store.CreateToken(TokenCreateRequest{
		Name:      "expired",
		Scopes:    []string{scopeMCP},
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Minute)),
	})
	require.NoError(t, err)
	_, err = s.store.db.Exec("UPDATE api_tokens SET expires_at = ? WHERE id = ?", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), expiredRecord.ID)
	require.NoError(t, err)

	for name, raw := range map[string]string{
		"revoked": revokedRaw,
		"expired": expiredRaw,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp/", nil)
			req.Header.Set("Authorization", "Bearer "+raw)
			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestServer_StreamableMCPReceivesAuthenticatedCallerIdentity(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	record, raw, err := s.store.CreateToken(TokenCreateRequest{
		Name:   "mcp-client",
		Scopes: []string{scopeMCP},
		Roles:  []string{testRoleOperator},
	})
	require.NoError(t, err)

	var receivedIdentity CallerIdentity
	var receivedRole string
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "authenticated-context", Version: testVersion100},
		Spec: ToolSpec{
			Description: Description{Short: "authenticated context test"},
			InputSchema: InputSchema{Type: schemaTypeObject},
			Security:    Security{AllowedRoles: []string{testRoleOperator}},
		},
		Handler: func(ctx context.Context, _ map[string]any) (*ToolResult, error) {
			receivedIdentity, _ = CallerIdentityFromContext(ctx)
			receivedRole, _ = CallerRoleFromContext(ctx)
			return &ToolResult{Result: map[string]any{"ok": true}}, nil
		},
	})

	mux := http.NewServeMux()
	s.ServeHTTP(mux, "http://localhost:8080")

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	initRequest := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(initialize))
	initRequest.Header.Set("Authorization", "Bearer "+raw)
	initRequest.Header.Set("Content-Type", "application/json")
	initRecorder := httptest.NewRecorder()
	mux.ServeHTTP(initRecorder, initRequest)
	require.Equal(t, http.StatusOK, initRecorder.Code)
	sessionID := initRecorder.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID)

	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"authenticated-context","arguments":{"role":"operator"}}}`
	callRequest := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(call))
	callRequest.Header.Set("Authorization", "Bearer "+raw)
	callRequest.Header.Set("Content-Type", "application/json")
	callRequest.Header.Set("Mcp-Session-Id", sessionID)
	callRecorder := httptest.NewRecorder()
	mux.ServeHTTP(callRecorder, callRequest)
	require.Equal(t, http.StatusOK, callRecorder.Code)
	assert.Contains(t, callRecorder.Body.String(), `"id":2`)
	assert.Equal(t, record.ID, receivedIdentity.PrincipalID)
	assert.Equal(t, []string{testRoleOperator}, receivedIdentity.Roles)
	assert.False(t, receivedIdentity.IsAdmin)
	assert.Equal(t, testRoleOperator, receivedRole)
}

func TestServer_StreamableHTTPPreservesAgentClientContract(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	_, raw, err := s.store.CreateToken(TokenCreateRequest{Name: "mcp-compat", Scopes: []string{scopeMCP}})
	require.NoError(t, err)
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "compatibility-tool", Version: testVersion100},
		Spec: ToolSpec{
			Description: Description{Short: "Compatibility test tool"},
			InputSchema: InputSchema{Type: schemaTypeObject},
		},
		Handler: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{"status": testStatusOk}}, nil
		},
	})

	mux := http.NewServeMux()
	s.ServeHTTP(mux, "http://localhost:8080")

	for _, protocolVersion := range []string{"2025-03-26", "2025-11-25"} {
		t.Run(protocolVersion, func(t *testing.T) {
			initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + protocolVersion + `","capabilities":{},"clientInfo":{"name":"agent-compatibility-test","version":"1.0"}}}`
			initRequest := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(initialize))
			setMCPHeaders(initRequest, raw, "")
			initRecorder := httptest.NewRecorder()
			mux.ServeHTTP(initRecorder, initRequest)
			require.Equal(t, http.StatusOK, initRecorder.Code)
			sessionID := initRecorder.Header().Get("Mcp-Session-Id")
			require.NotEmpty(t, sessionID)
			initResponse := decodeMCPResponse(t, initRecorder, 1)
			assert.Equal(t, protocolVersion, initResponse["result"].(map[string]any)["protocolVersion"])

			listRequest := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
			setMCPHeaders(listRequest, raw, sessionID)
			listRecorder := httptest.NewRecorder()
			mux.ServeHTTP(listRecorder, listRequest)
			require.Equal(t, http.StatusOK, listRecorder.Code)
			listResponse := decodeMCPResponse(t, listRecorder, 2)
			tools := listResponse["result"].(map[string]any)["tools"].([]any)
			foundTool := false
			for _, rawTool := range tools {
				tool, ok := rawTool.(map[string]any)
				if ok && tool["name"] == "compatibility-tool" {
					foundTool = true
					assert.Equal(t, "Compatibility test tool", tool["description"])
				}
			}
			assert.True(t, foundTool, "tools/list should expose compatibility-tool")

			callRequest := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"compatibility-tool","arguments":{}}}`))
			setMCPHeaders(callRequest, raw, sessionID)
			callRecorder := httptest.NewRecorder()
			mux.ServeHTTP(callRecorder, callRequest)
			require.Equal(t, http.StatusOK, callRecorder.Code)
			callResponse := decodeMCPResponse(t, callRecorder, 3)
			assert.Equal(t, float64(3), callResponse["id"])
			callResult := callResponse["result"].(map[string]any)
			assert.NotEqual(t, true, callResult["isError"])
			assert.Contains(t, callResult["content"], map[string]any{"type": textContentType, "text": `{"status":"ok"}`})

			unauthenticatedCall := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(`{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{}}`))
			unauthenticatedCall.Header.Set("Content-Type", "application/json")
			unauthenticatedCall.Header.Set("Mcp-Session-Id", sessionID)
			unauthenticatedRecorder := httptest.NewRecorder()
			mux.ServeHTTP(unauthenticatedRecorder, unauthenticatedCall)
			assert.Equal(t, http.StatusUnauthorized, unauthenticatedRecorder.Code)
		})
	}
}

func TestServer_PersistedToolMetadataSurvivesReconcileAndHTTPDiscovery(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	dbPath := filepath.Join(t.TempDir(), "erpbridge.db")
	initial := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, dbPath)
	readOnly, destructive := true, false
	annotated := &Tool{
		Metadata: Metadata{Name: "annotated-tool", Version: testVersion100, IsActive: true},
		Spec: ToolSpec{
			Description: Description{
				Short:        "Annotated tool",
				WhenToUse:    []string{"When the model needs annotated data"},
				WhenNotToUse: []string{"When the model needs to change data"},
				Examples:     []string{"Show annotated data"},
			},
			Annotations: &ToolAnnotations{
				Title:           "Annotated tool",
				ReadOnlyHint:    &readOnly,
				DestructiveHint: &destructive,
			},
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{}},
			Security:    Security{AllowedRoles: []string{"agent_reader"}},
		},
	}
	legacy := &Tool{
		Metadata: Metadata{Name: "legacy-tool", Version: testVersion100, IsActive: true},
		Spec: ToolSpec{
			Description: Description{Short: "Legacy tool"},
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{}},
		},
	}
	require.NoError(t, initial.store.Save(annotated))
	require.NoError(t, initial.store.Save(legacy))
	require.NoError(t, initial.store.Close())

	fresh := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, dbPath)
	defer func() { _ = fresh.store.Close() }()
	fresh.Reconcile(context.Background())

	mux := http.NewServeMux()
	fresh.ServeHTTP(mux, "http://localhost:8080")
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"metadata-test","version":"1.0"}}}`
	initRequest := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(initialize))
	setMCPHeaders(initRequest, "admin-secret", "")
	initRecorder := httptest.NewRecorder()
	mux.ServeHTTP(initRecorder, initRequest)
	require.Equal(t, http.StatusOK, initRecorder.Code)
	sessionID := initRecorder.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID)

	listRequest := httptest.NewRequest(http.MethodPost, "/mcp/", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	setMCPHeaders(listRequest, "admin-secret", sessionID)
	listRecorder := httptest.NewRecorder()
	mux.ServeHTTP(listRecorder, listRequest)
	require.Equal(t, http.StatusOK, listRecorder.Code)
	listResponse := decodeMCPResponse(t, listRecorder, 2)
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
	assert.Equal(t, "Annotated tool", annotatedWire["title"])
	assert.Equal(t, "Annotated tool", annotatedWire["annotations"].(map[string]any)["title"])
	assert.Equal(t, false, annotatedWire["annotations"].(map[string]any)["destructiveHint"])
	assert.Equal(t, []any{"When the model needs annotated data"}, annotatedWire["_meta"].(map[string]any)["io.erpbridge/whenToUse"])
	assert.Equal(t, []any{"When the model needs to change data"}, annotatedWire["_meta"].(map[string]any)["io.erpbridge/whenNotToUse"])
	assert.Equal(t, []any{"Show annotated data"}, annotatedWire["_meta"].(map[string]any)["io.erpbridge/examples"])
	assert.Equal(t, []any{"agent_reader"}, annotatedWire["_meta"].(map[string]any)["io.erpbridge/allowedRoles"])
	assert.NotContains(t, annotatedWire, "endpoint")
	assert.NotContains(t, annotatedWire, "credentialRef")

	legacyWire := found[legacy.Metadata.Name]
	require.NotNil(t, legacyWire)
	assert.Empty(t, legacyWire["annotations"].(map[string]any))
	assert.NotContains(t, legacyWire, "_meta")
}

func setMCPHeaders(request *http.Request, token, sessionID string) {
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		request.Header.Set("Mcp-Session-Id", sessionID)
	}
}

func decodeMCPResponse(t *testing.T, recorder *httptest.ResponseRecorder, id float64) map[string]any {
	t.Helper()
	body := strings.TrimSpace(recorder.Body.String())
	if strings.HasPrefix(body, "{") {
		var response map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &response))
		assert.Equal(t, id, response["id"])
		return response
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			var response map[string]any
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &response))
			if response["id"] == id {
				return response
			}
		}
	}
	t.Fatalf("MCP response did not contain response id %v: %q", id, recorder.Body.String())
	return nil
}

func TestTokenAPI_AdminOnlyAndOneTimeValue(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-secret")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	body := `{"name":"cli-client","scopes":["mcp"],"roles":["operator"]}`
	request := httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(body))
	record := httptest.NewRecorder()
	s.handleTokenAPI(record, request)
	assert.Equal(t, http.StatusUnauthorized, record.Code)

	request = httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin-secret")
	record = httptest.NewRecorder()
	s.handleTokenAPI(record, request)
	require.Equal(t, http.StatusCreated, record.Code)
	var created struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(record.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.NotEmpty(t, created.Token)

	request = httptest.NewRequest(http.MethodGet, "/api/auth/tokens", nil)
	request.Header.Set("Authorization", "Bearer admin-secret")
	record = httptest.NewRecorder()
	s.handleTokenAPI(record, request)
	require.Equal(t, http.StatusOK, record.Code)
	assert.NotContains(t, record.Body.String(), created.Token)

	request = httptest.NewRequest(http.MethodDelete, "/api/auth/tokens/"+created.ID, nil)
	request.Header.Set("Authorization", "Bearer admin-secret")
	record = httptest.NewRecorder()
	s.handleTokenAPI(record, request)
	assert.Equal(t, http.StatusNoContent, record.Code)
}
