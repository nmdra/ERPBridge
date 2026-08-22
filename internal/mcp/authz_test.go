package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleAuthzMiddleware_SelectsAndRemovesMCPRole(t *testing.T) {
	tool := &Tool{Spec: ToolSpec{Security: Security{AllowedRoles: []string{testRoleOperator}}}}
	identity := CallerIdentity{PrincipalID: testClientOne, Roles: []string{testRoleOperator}}
	var received map[string]any
	var receivedRole string
	handler := (&Server{}).RoleAuthzMiddleware(tool)(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		received, _ = request.Params.Arguments.(map[string]any)
		receivedRole, _ = CallerRoleFromContext(ctx)
		return mcp.NewToolResultText("ok"), nil
	})

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{roleSelectorField: testRoleOperator, testAmountField: 10}
	result, err := handler(WithCallerIdentity(context.Background(), identity), request)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, map[string]any{testAmountField: 10}, received)
	assert.Equal(t, testRoleOperator, receivedRole)
}

func TestRoleAuthzMiddleware_DeniesWithoutVerifiedMembership(t *testing.T) {
	tool := &Tool{Spec: ToolSpec{Security: Security{AllowedRoles: []string{testRoleOperator}}}}
	handler := (&Server{}).RoleAuthzMiddleware(tool)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Fatal("authorized handler must not run")
		return nil, nil
	})
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{roleSelectorField: testRoleOperator}

	result, err := handler(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "verified identity")
}

func TestRoleAuthzMiddleware_PreservesOpenBusinessRole(t *testing.T) {
	tool := &Tool{}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{roleSelectorField: "sales", testAmountField: 10}
	var received map[string]any
	handler := (&Server{}).RoleAuthzMiddleware(tool)(func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		received, _ = request.Params.Arguments.(map[string]any)
		return mcp.NewToolResultText("ok"), nil
	})

	_, err := handler(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, request.Params.Arguments, received)
}

func TestServer_DirectInvokeRoleAuthorization(t *testing.T) {
	called := 0
	var receivedArgs map[string]any
	var receivedRole string
	s := NewServer(&MockConnector{}, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "guarded-direct", Version: testVersion100},
		Spec: ToolSpec{
			Security: Security{AllowedRoles: []string{testRoleOperator}},
		},
		Handler: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			called++
			receivedArgs = args
			receivedRole, _ = CallerRoleFromContext(ctx)
			return &ToolResult{Result: map[string]any{"ok": true}}, nil
		},
	})

	body := `{"name":"guarded-direct","arguments":{"amount":10}}`
	request := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", strings.NewReader(body))
	request.Header.Set("X-ERPBridge-Role", testRoleOperator)
	request = request.WithContext(WithCallerIdentity(request.Context(), CallerIdentity{PrincipalID: testClientOne, Roles: []string{testRoleOperator}}))
	recorder := httptest.NewRecorder()
	s.handleDirectInvoke(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, 1, called)
	assert.Equal(t, map[string]any{testAmountField: float64(10)}, receivedArgs)
	assert.Equal(t, testRoleOperator, receivedRole)

	collision := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", strings.NewReader(`{"name":"guarded-direct","arguments":{"role":"operator"}}`))
	collision.Header.Set("X-ERPBridge-Role", testRoleOperator)
	collision = collision.WithContext(WithCallerIdentity(collision.Context(), CallerIdentity{PrincipalID: testClientOne, Roles: []string{testRoleOperator}}))
	recorder = httptest.NewRecorder()
	s.handleDirectInvoke(recorder, collision)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, 1, called)

	denied := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", strings.NewReader(body))
	denied.Header.Set("X-ERPBridge-Role", testRoleOperator)
	denied = denied.WithContext(WithCallerIdentity(denied.Context(), CallerIdentity{PrincipalID: "client-2", Roles: []string{"other"}}))
	recorder = httptest.NewRecorder()
	s.handleDirectInvoke(recorder, denied)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Equal(t, 1, called)
}

func TestRoleAuthorizationErrorIsSafeToSerialize(t *testing.T) {
	errorValue := &RoleAuthorizationError{Status: http.StatusForbidden, Reason: "role is not allowed"}
	encoded, err := json.Marshal(errorValue)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret")
}
