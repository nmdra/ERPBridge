package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardedCacheSeparatesVerifiedRoles(t *testing.T) {
	s := NewServer(nil, cache.NewMemoryManager(10, logger.Init()), logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "role-cache", Version: testVersion100},
		Spec: ToolSpec{
			Security: Security{AllowedRoles: []string{testRoleAdmin, testRoleOperator}},
			Cache:    &cache.Config{Enabled: true, IsReadOnly: false},
		},
	}
	calls := 0
	handler := s.RoleAuthzMiddleware(tool)(s.CacheMiddleware(tool)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls++
		return mcp.NewToolResultText("fresh"), nil
	}))

	call := func(role string) {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]any{roleSelectorField: role, testAmountField: 1}
		identity := WithCallerIdentity(context.Background(), CallerIdentity{PrincipalID: role, Roles: []string{role}})
		_, err := handler(identity, request)
		require.NoError(t, err)
	}
	call(testRoleOperator)
	call(testRoleAdmin)
	call(testRoleOperator)
	require.Equal(t, 2, calls)
}

func TestReadOnlyGuardedCacheRemainsShared(t *testing.T) {
	s := NewServer(nil, cache.NewMemoryManager(10, logger.Init()), logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "shared-role-cache", Version: testVersion100},
		Spec: ToolSpec{
			Security: Security{AllowedRoles: []string{testRoleAdmin, testRoleOperator}},
			Cache:    &cache.Config{Enabled: true, IsReadOnly: true},
		},
	}
	calls := 0
	var next server.ToolHandlerFunc = func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls++
		return mcp.NewToolResultText("fresh"), nil
	}
	handler := s.RoleAuthzMiddleware(tool)(s.CacheMiddleware(tool)(next))

	for _, role := range []string{testRoleOperator, testRoleAdmin} {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]any{roleSelectorField: role, testAmountField: 1}
		identity := WithCallerIdentity(context.Background(), CallerIdentity{PrincipalID: role, Roles: []string{role}})
		_, err := handler(identity, request)
		require.NoError(t, err)
	}
	require.Equal(t, 1, calls)
}

func TestGuardedCacheRejectsRoleAfterAllowListChange(t *testing.T) {
	s := NewServer(nil, cache.NewMemoryManager(10, logger.Init()), logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "mutable-role-cache", Version: testVersion100},
		Spec: ToolSpec{
			Security: Security{AllowedRoles: []string{testRoleOperator}},
			Cache:    &cache.Config{Enabled: true, IsReadOnly: false},
		},
	}
	calls := 0
	handler := s.RoleAuthzMiddleware(tool)(s.CacheMiddleware(tool)(func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls++
		return mcp.NewToolResultText("fresh"), nil
	}))

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{roleSelectorField: testRoleOperator, testAmountField: 1}
	identity := WithCallerIdentity(context.Background(), CallerIdentity{PrincipalID: testClientOne, Roles: []string{testRoleOperator}})
	result, err := handler(identity, request)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, 1, calls)

	tool.Spec.Security.AllowedRoles = []string{testRoleAdmin}
	result, err = handler(identity, request)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Equal(t, 1, calls)
}
