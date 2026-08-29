package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/faults"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestToolExecutionErrorResultUsesSafeNamespacedMetadata(t *testing.T) {
	result := newToolExecutionResult(faults.New(
		faults.KindRateLimited,
		"the service is temporarily rate limited; retry after 10 seconds",
		true,
		10*time.Second,
		errors.New("private response body"),
	))

	require.True(t, result.IsError)
	require.Equal(t, "the service is temporarily rate limited; retry after 10 seconds", result.Content[0].(mcp.TextContent).Text)
	metadata, ok := result.Meta.AdditionalFields["com.erpbridge/error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "rate_limit", metadata["type"])
	require.Equal(t, true, metadata["retryable"])
	require.Equal(t, int64(10000), metadata["retryAfterMs"])
	require.NotContains(t, result.Content[0].(mcp.TextContent).Text, "private")
}

func TestToolExecutionErrorResultSanitizesUnexpectedErrors(t *testing.T) {
	result := newToolExecutionResult(errors.New("private database connection details"))

	require.True(t, result.IsError)
	require.Equal(t, "the tool could not complete; check server logs before retrying", result.Content[0].(mcp.TextContent).Text)
	require.NotContains(t, result.Content[0].(mcp.TextContent).Text, "database")
}

func TestMCPProtocolErrorsUseJSONRPCAndBusinessInputUsesToolResult(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "validated-tool", Version: testVersion100},
		Spec: ToolSpec{
			Description: Description{Short: testDescShort},
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{"name": {Type: "string"}},
				Required:   []string{"name"},
			},
		},
		Handler: func(_ context.Context, args map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: args}, nil
		},
	})

	parseResponse := s.MCPServer().HandleMessage(context.Background(), []byte("{"))
	parseError, ok := parseResponse.(mcp.JSONRPCError)
	require.True(t, ok)
	require.Equal(t, mcp.PARSE_ERROR, parseError.Error.Code)

	methodResponse := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"unsupported/method"}`))
	methodError, ok := methodResponse.(mcp.JSONRPCError)
	require.True(t, ok)
	require.Equal(t, mcp.METHOD_NOT_FOUND, methodError.Error.Code)

	unknownToolResponse := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"missing-tool","arguments":{}}}`))
	unknownToolError, ok := unknownToolResponse.(mcp.JSONRPCError)
	require.True(t, ok)
	require.Equal(t, mcp.INVALID_PARAMS, unknownToolError.Error.Code)

	invalidParamsResponse := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":[]}`))
	invalidParamsError, ok := invalidParamsResponse.(mcp.JSONRPCError)
	require.True(t, ok)
	require.Equal(t, mcp.INVALID_REQUEST, invalidParamsError.Error.Code)

	businessResponse := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"validated-tool","arguments":{"name":12}}}`))
	businessRPC, ok := businessResponse.(mcp.JSONRPCResponse)
	require.True(t, ok)
	businessResult, ok := businessRPC.Result.(*mcp.CallToolResult)
	require.True(t, ok)
	require.True(t, businessResult.IsError)
	require.NotContains(t, businessResult.Content[0].(mcp.TextContent).Text, "private")
}

func TestMCPDependencyTimeoutRemainsRetryableToolError(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "timeout-tool", Version: testVersion100},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			return nil, faults.New(faults.KindDependencyTimeout, "the ERP service timed out; retry later", true, 0, errors.New("private timeout"))
		},
	})

	response := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"timeout-tool","arguments":{}}}`))
	rpcResponse, ok := response.(mcp.JSONRPCResponse)
	require.True(t, ok)
	result, ok := rpcResponse.Result.(*mcp.CallToolResult)
	require.True(t, ok)
	require.True(t, result.IsError)
	metadata := result.Meta.AdditionalFields["com.erpbridge/error"].(map[string]any)
	require.Equal(t, "dependency_timeout", metadata["type"])
	require.Equal(t, true, metadata["retryable"])
	require.NotContains(t, result.Content[0].(mcp.TextContent).Text, "private")
}

func TestMCPPanicBecomesSanitizedInternalProtocolError(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	s.RegisterTool(&Tool{
		Metadata: Metadata{Name: "panic-tool", Version: testVersion100},
		Spec:     ToolSpec{Description: Description{Short: testDescShort}},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			panic("private panic details")
		},
	})

	response := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"panic-tool","arguments":{}}}`))
	errResponse, ok := response.(mcp.JSONRPCError)
	require.True(t, ok)
	require.Equal(t, mcp.INTERNAL_ERROR, errResponse.Error.Code)
	require.NotContains(t, errResponse.Error.Message, "private")
}
