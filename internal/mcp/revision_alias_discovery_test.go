package mcp

import (
	"context"
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

func advertisedNames(s *Server) map[string]bool {
	registered := s.MCPServer().ListTools()
	tools := make([]mcp.Tool, 0, len(registered))
	for _, entry := range registered {
		tools = append(tools, entry.Tool)
	}
	names := make(map[string]bool, len(tools))
	for _, tool := range s.FilterToolsList(tools) {
		names[tool.Name] = true
	}
	return names
}

func TestRevisionAliasesAreHiddenByDefault(t *testing.T) {
	s := aliasTestServer(t)
	names := advertisedNames(s)
	require.True(t, names["alias-test"], "the unqualified serving name must stay visible")
	require.False(t, names[QualifiedToolName("alias-test", "1.0.0")])
	require.False(t, names[QualifiedToolName("alias-test", "2.0.0")])
}

func TestRevisionAliasesCanBeAdvertised(t *testing.T) {
	t.Setenv(advertiseRevisionAliasesEnv, "true")
	s := aliasTestServer(t)
	names := advertisedNames(s)
	require.True(t, names["alias-test"])
	require.True(t, names[QualifiedToolName("alias-test", "1.0.0")])
	require.True(t, names[QualifiedToolName("alias-test", "2.0.0")])
}

func TestHiddenRevisionAliasStillExecutes(t *testing.T) {
	s := aliasTestServer(t)
	exact := QualifiedToolName("alias-test", "1.0.0")
	require.NotNil(t, s.MCPServer().GetTool(exact), "the exact revision must remain registered")

	request := mcp.CallToolRequest{}
	request.Params.Name = exact
	request.Params.Arguments = map[string]any{}
	result, err := s.handleBoundMCPToolRevisionCall("alias-test", "1.0.0")(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "v1")
}
