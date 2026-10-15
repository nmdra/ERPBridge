package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

func aliasTestServer(t *testing.T) *Server {
	t.Helper()
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "alias-test", Version: "1.0.0", IsActive: true, IsServing: true, ResourceDigest: "digest-v1"},
		Spec:     ToolSpec{Description: Description{Short: "v1"}},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{"revision": "v1"}}, nil
		},
	})
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "alias-test", Version: "2.0.0", IsActive: true, ResourceDigest: "digest-v2"},
		Spec:     ToolSpec{Description: Description{Short: "v2"}},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{"revision": "v2"}}, nil
		},
	})
	return s
}

func advertisedNames(t *testing.T, s *Server) map[string]bool {
	t.Helper()
	response := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	rpc, ok := response.(mcp.JSONRPCResponse)
	require.True(t, ok)
	var listed []mcp.Tool
	switch result := rpc.Result.(type) {
	case mcp.ListToolsResult:
		listed = result.Tools
	case *mcp.ListToolsResult:
		listed = result.Tools
	default:
		require.FailNow(t, "unexpected tools/list result type")
	}
	names := make(map[string]bool, len(listed))
	for _, tool := range listed {
		names[tool.Name] = true
	}
	return names
}

func callTool(t *testing.T, s *Server, name string) *mcp.CallToolResult {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": map[string]any{}},
	})
	require.NoError(t, err)
	response := s.MCPServer().HandleMessage(context.Background(), payload)
	rpc, ok := response.(mcp.JSONRPCResponse)
	require.True(t, ok, "expected a JSON-RPC response")
	result, ok := rpc.Result.(*mcp.CallToolResult)
	require.True(t, ok, "expected a tool result, got %T", rpc.Result)
	return result
}

func TestRevisionAliasesAreHiddenByDefault(t *testing.T) {
	s := aliasTestServer(t)
	names := advertisedNames(t, s)
	require.True(t, names["alias-test"], "the unqualified serving name must stay visible")
	require.False(t, names[QualifiedToolName("alias-test", "1.0.0")])
	require.False(t, names[QualifiedToolName("alias-test", "2.0.0")])
}

func TestRevisionAliasesCanBeAdvertised(t *testing.T) {
	t.Setenv(advertiseRevisionAliasesEnv, "true")
	s := aliasTestServer(t)
	names := advertisedNames(t, s)
	require.True(t, names["alias-test"])
	require.True(t, names[QualifiedToolName("alias-test", "1.0.0")])
	require.True(t, names[QualifiedToolName("alias-test", "2.0.0")])
}

func TestHiddenRevisionAliasStillExecutes(t *testing.T) {
	s := aliasTestServer(t)
	result := callTool(t, s, QualifiedToolName("alias-test", "1.0.0"))
	require.False(t, result.IsError)
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "v1")
}

func TestUnqualifiedCallStillExecutes(t *testing.T) {
	s := aliasTestServer(t)
	result := callTool(t, s, "alias-test")
	require.False(t, result.IsError)
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "v1")
}
