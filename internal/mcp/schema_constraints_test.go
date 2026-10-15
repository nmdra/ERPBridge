package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestAdditionalPropertiesPolicies(t *testing.T) {
	for _, policy := range []string{"", `,"additionalProperties":true`, `,"additionalProperties":false`, `,"additionalProperties":{"type":"integer","enum":[0]}`} {
		t.Run(policy, func(t *testing.T) {
			var tool Tool
			require.NoError(t, json.Unmarshal([]byte(`{"spec":{"inputSchema":{"type":"object","properties":{"nested":{"type":"object"`+policy+`},"rows":{"type":"array","items":{"type":"object"`+policy+`}}}`+policy+`}}}`), &tool))
			for _, args := range []map[string]any{
				{"extra": "private-marker"},
				{"nested": map[string]any{"extra": "private-marker"}},
				{"rows": []any{map[string]any{"extra": "private-marker"}}},
			} {
				err := validateToolArguments(&tool, args)
				if policy == "" || policy == `,"additionalProperties":true` {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}
			if strings.Contains(policy, `"enum"`) {
				require.NoError(t, validateToolArguments(&tool, map[string]any{"extra": 0, "nested": map[string]any{"extra": 0}, "rows": []any{map[string]any{"extra": 0}}}))
			}
		})
	}
	// An otherwise empty legacy schema must still enforce explicit closure.
	tool := &Tool{Spec: ToolSpec{InputSchema: InputSchema{AdditionalProperties: false}}}
	require.Error(t, validateToolArguments(tool, map[string]any{"cmd": "harmless"}))
}

func TestClosedInputGuardedRoleAndValidationBeforeMiddleware(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	executions, middlewares := 0, 0
	tool := &Tool{
		Metadata: Metadata{Name: "closed-role", Version: testVersion100},
		Spec: ToolSpec{
			InputSchema: InputSchema{Type: "object", AdditionalProperties: false, Properties: map[string]Property{"amount": {Type: "integer", Enum: []any{0}}}},
			Security:    Security{AllowedRoles: []string{testRoleOperator}},
		},
		Handler: func(_ context.Context, args map[string]any) (*ToolResult, error) {
			executions++
			require.NotContains(t, args, "role")
			return &ToolResult{Result: map[string]any{"ok": true}}, nil
		},
	}
	// Use the existing middleware seam to prove rejection happens before it.
	s.toolMiddlewares = append(s.toolMiddlewares, func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request protocol.CallToolRequest) (*protocol.CallToolResult, error) {
			middlewares++
			return next(ctx, request)
		}
	})
	s.RegisterTool(tool)
	ctx := WithCallerIdentity(context.Background(), CallerIdentity{PrincipalID: testClientOne, Roles: []string{testRoleOperator}})
	for _, invalid := range []bool{false, true} {
		for _, direct := range []bool{false, true} {
			args := map[string]any{"amount": 0}
			if invalid {
				args["cmd"] = "harmless.private-marker"
			}
			beforeExecutions, beforeMiddleware := executions, middlewares
			if direct {
				body, err := json.Marshal(ToolCallRequest{Name: tool.Metadata.Name, Arguments: args})
				require.NoError(t, err)
				request := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", strings.NewReader(string(body))).WithContext(ctx)
				request.Header.Set("X-ERPBridge-Role", testRoleOperator)
				recorder := httptest.NewRecorder()
				s.handleDirectInvoke(recorder, request)
				require.Equal(t, !invalid, recorder.Code == http.StatusOK)
				require.NotContains(t, recorder.Body.String(), "private-marker")
			} else {
				args["role"] = testRoleOperator
				request := protocol.CallToolRequest{}
				request.Params.Name = tool.Metadata.Name
				request.Params.Arguments = args
				result, err := s.handleBoundMCPToolCall(tool.Metadata.Name)(ctx, request)
				require.NoError(t, err)
				require.Equal(t, invalid, result.IsError)
			}
			increment := 1
			if invalid {
				increment = 0
			}
			require.Equal(t, beforeExecutions+increment, executions)
			require.Equal(t, beforeMiddleware+increment, middlewares)
		}
	}
	require.NotContains(t, tool.Spec.InputSchema.Properties, "role")
}
