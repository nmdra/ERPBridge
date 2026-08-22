package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
