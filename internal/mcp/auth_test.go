package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
