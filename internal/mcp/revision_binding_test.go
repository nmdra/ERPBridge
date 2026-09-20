package mcp

import (
	"context"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestMCPServingTransitionKeepsOneInvocationSnapshot(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })

	started := make(chan struct{})
	release := make(chan struct{})
	var pauseOnce sync.Once
	v1 := &Tool{
		Metadata: Metadata{Name: "revision-test", Version: "1.0.0", IsActive: true, IsServing: true, ResourceDigest: "digest-v1"},
		Spec:     ToolSpec{Description: Description{Short: "schema-v1"}},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			pauseOnce.Do(func() {
				close(started)
				<-release
			})
			return &ToolResult{Result: map[string]any{"revision": "v1"}}, nil
		},
	}
	v2 := &Tool{
		Metadata: Metadata{Name: "revision-test", Version: "2.0.0", IsActive: true, ResourceDigest: "digest-v2"},
		Spec:     ToolSpec{Description: Description{Short: "schema-v2"}},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{"revision": "v2"}}, nil
		},
	}
	s.RegisterTool(v1)
	s.RegisterTool(v2)

	require.NotNil(t, s.MCPServer().GetTool("revision-test"))
	require.NotNil(t, s.MCPServer().GetTool(QualifiedToolName("revision-test", "1.0.0")))
	require.NotNil(t, s.MCPServer().GetTool(QualifiedToolName("revision-test", "2.0.0")))

	request := mcp.CallToolRequest{}
	request.Params.Name = "revision-test"
	request.Params.Arguments = map[string]any{}
	type outcome struct {
		result *mcp.CallToolResult
		err    error
	}
	completed := make(chan outcome, 1)
	go func() {
		result, err := s.handleBoundMCPToolCall("revision-test")(context.Background(), request)
		completed <- outcome{result: result, err: err}
	}()
	<-started

	s.mu.Lock()
	require.NoError(t, s.registry.SetServing("revision-test", "2.0.0"))
	s.mu.Unlock()
	close(release)
	first := <-completed
	require.NoError(t, first.err)
	require.Equal(t, "v1", textResult(t, first.result)["revision"])

	second, err := s.handleBoundMCPToolCall("revision-test")(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "v2", textResult(t, second)["revision"])

	exactV1, err := s.handleBoundMCPToolRevisionCall("revision-test", "1.0.0")(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "v1", textResult(t, exactV1)["revision"])
}

func TestMCPDigestBoundCallNeverFallsForward(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })

	calls := 0
	tool := &Tool{
		Metadata: Metadata{Name: "digest-test", Version: "1.0.0", IsActive: true, IsServing: true, ResourceDigest: "approved-digest"},
		Spec:     ToolSpec{Description: Description{Short: "digest"}},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			calls++
			return &ToolResult{Result: map[string]any{"ok": true}}, nil
		},
	}
	s.RegisterTool(tool)

	request := mcp.CallToolRequest{}
	request.Params.Name = tool.Metadata.Name
	request.Params.Arguments = map[string]any{}
	request.Params.Meta = mcp.NewMetaFromMap(map[string]any{"toolplane.resourceDigest": "stale-digest"})
	result, err := s.handleBoundMCPToolCall(tool.Metadata.Name)(context.Background(), request)
	require.Error(t, err)
	require.Nil(t, result)
	require.Zero(t, calls)

	request.Params.Meta = mcp.NewMetaFromMap(map[string]any{"toolplane.resourceDigest": "approved-digest"})
	result, err = s.handleBoundMCPToolCall(tool.Metadata.Name)(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, true, textResult(t, result)["ok"])
	require.Equal(t, 1, calls)
}

func TestMCPDigestBoundCallUsesSelectedRevisionSchema(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })

	v1 := &Tool{
		Metadata: Metadata{Name: "schema-bound", Version: "1.0.0", IsActive: true, IsServing: true, ResourceDigest: "digest-v1"},
		Spec: ToolSpec{InputSchema: InputSchema{
			Type: schemaTypeObject, Properties: map[string]Property{"legacy": {Type: schemaTypeString}}, Required: []string{"legacy"},
		}},
		Handler: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{"revision": "v1"}}, nil
		},
	}
	v2 := &Tool{
		Metadata: Metadata{Name: "schema-bound", Version: "2.0.0", IsActive: true, IsServing: true, ResourceDigest: "digest-v2"},
		Spec: ToolSpec{InputSchema: InputSchema{
			Type: schemaTypeObject, Properties: map[string]Property{"current": {Type: schemaTypeNumber}}, Required: []string{"current"},
		}},
		Handler: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{"revision": "v2"}}, nil
		},
	}
	s.RegisterTool(v1)
	s.RegisterTool(v2)

	response := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"schema-bound","arguments":{"legacy":"ok"},"_meta":{"toolplane.resourceDigest":"digest-v1"}}}`))
	rpcResponse, ok := response.(mcp.JSONRPCResponse)
	require.True(t, ok)
	result, ok := rpcResponse.Result.(*mcp.CallToolResult)
	require.True(t, ok)
	require.False(t, result.IsError)
	require.Equal(t, "v1", textResult(t, result)["revision"])
}

func TestMCPExactRevisionCachesAreIsolated(t *testing.T) {
	log := logger.Init()
	s := NewServer(nil, cache.NewMemoryManager(10, log), log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })

	newRevision := func(version, digest string) *Tool {
		return &Tool{
			Metadata: Metadata{Name: "cache-revision", Version: version, IsActive: true, ResourceDigest: digest},
			Spec:     ToolSpec{Cache: &cache.Config{Enabled: true, TTLSeconds: 60}},
			Handler: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
				return &ToolResult{Result: map[string]any{"revision": version}}, nil
			},
		}
	}
	v1 := newRevision("1.0.0", "digest-v1")
	v1.Metadata.IsServing = true
	v2 := newRevision("2.0.0", "digest-v2")
	s.RegisterTool(v1)
	s.RegisterTool(v2)

	call := func(name string) *mcp.CallToolResult {
		response := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`))
		rpcResponse, ok := response.(mcp.JSONRPCResponse)
		require.True(t, ok)
		result, ok := rpcResponse.Result.(*mcp.CallToolResult)
		require.True(t, ok)
		return result
	}

	require.Equal(t, "1.0.0", textResult(t, call(QualifiedToolName(v1.Metadata.Name, v1.Metadata.Version)))["revision"])
	require.Equal(t, "2.0.0", textResult(t, call(QualifiedToolName(v2.Metadata.Name, v2.Metadata.Version)))["revision"])
}

func TestMCPToolsListRefreshesServingMetadataForEveryExactRevision(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	v1 := &Tool{Metadata: Metadata{Name: "serving-meta", Version: "1.0.0", IsActive: true, IsServing: true, ResourceDigest: "digest-v1"}}
	v2 := &Tool{Metadata: Metadata{Name: "serving-meta", Version: "2.0.0", IsActive: true, ResourceDigest: "digest-v2"}}
	s.RegisterTool(v1)
	s.RegisterTool(v2)
	v2.Metadata.IsServing = true
	s.RegisterTool(v2)

	response := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	rpcResponse, ok := response.(mcp.JSONRPCResponse)
	require.True(t, ok)
	var tools []mcp.Tool
	switch list := rpcResponse.Result.(type) {
	case mcp.ListToolsResult:
		tools = list.Tools
	case *mcp.ListToolsResult:
		tools = list.Tools
	default:
		require.FailNow(t, "unexpected tools/list result type")
	}
	servingByName := make(map[string]bool)
	for _, tool := range tools {
		if tool.Meta == nil {
			continue
		}
		serving, _ := tool.Meta.AdditionalFields["toolplane.serving"].(bool)
		servingByName[tool.Name] = serving
	}
	require.False(t, servingByName[QualifiedToolName(v1.Metadata.Name, v1.Metadata.Version)])
	require.True(t, servingByName[QualifiedToolName(v2.Metadata.Name, v2.Metadata.Version)])
	require.True(t, servingByName[v2.Metadata.Name])
}
