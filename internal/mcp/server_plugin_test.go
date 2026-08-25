package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

const (
	pluginResultSourceField = "source"
	pluginStepFirst         = "first"
	pluginStepSecond        = "second"
	pluginInvalidField      = "invalid"
	pluginSchemaTypeField   = "type"
	pluginStepField         = "step"
)

type fakePluginProcessor struct {
	mu          sync.Mutex
	invocations []PluginInvocation
	process     func(PluginInvocation) (*PluginResponse, error)
}

func (f *fakePluginProcessor) Process(_ context.Context, _ *Plugin, invocation PluginInvocation) (*PluginResponse, error) {
	f.mu.Lock()
	f.invocations = append(f.invocations, invocation)
	f.mu.Unlock()
	return f.process(invocation)
}

func (f *fakePluginProcessor) Calls() []PluginInvocation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PluginInvocation(nil), f.invocations...)
}

func installActivePluginBindings(s *Server, tool *Tool, bindings ...PluginBinding) {
	plugins := make([]*Plugin, 0, len(bindings))
	bindingPointers := make([]*PluginBinding, 0, len(bindings))
	for i := range bindings {
		binding := &bindings[i]
		bindingPointers = append(bindingPointers, binding)
		plugins = append(plugins, &Plugin{
			APIVersion: PluginAPIVersion,
			Kind:       PluginKind,
			Metadata: PluginMetadata{
				Name:     binding.Spec.PluginRef.Name,
				Version:  binding.Spec.PluginRef.Version,
				IsActive: true,
			},
			Spec: PluginSpec{Endpoint: "http://plugin.example.test", TimeoutMilliseconds: 1000},
		})
	}
	snapshot := buildPluginBindingSnapshot(plugins, bindingPointers, []*Tool{tool})
	s.pluginRegistry.Replace(snapshot)
}

func newPluginPipelineTool(name string) *Tool {
	return &Tool{
		Metadata: Metadata{Name: name, Version: testVersion100, IsActive: true},
		Spec:     ToolSpec{Description: Description{Short: "plugin pipeline test"}},
		Handler: func(context.Context, map[string]any) (*ToolResult, error) {
			return &ToolResult{Result: map[string]any{pluginResultSourceField: true}}, nil
		},
	}
}

func invokeMCPHandler(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), name string) *mcp.CallToolResult {
	t.Helper()
	request := mcp.CallToolRequest{}
	request.Params.Name = name
	request.Params.Arguments = map[string]any{}
	result, err := handler(context.Background(), request)
	require.NoError(t, err)
	return result
}

func textResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	var value map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &value))
	return value
}

func TestServerPlugin_OrdersBindingsAndTransformsResult(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("ordered-plugin-tool")
	s.RegisterTool(tool)

	first := validPluginBindingForTest()
	first.Spec.ToolRef.Name = tool.Metadata.Name
	first.Metadata.Name = pluginStepSecond
	first.Spec.Priority = 20
	first.Spec.Config = map[string]any{pluginStepField: pluginStepSecond}
	second := validPluginBindingForTest()
	second.Spec.ToolRef.Name = tool.Metadata.Name
	second.Metadata.Name = pluginStepFirst
	second.Spec.Priority = 10
	second.Spec.Config = map[string]any{pluginStepField: pluginStepFirst}
	installActivePluginBindings(s, tool, first, second)

	processor := &fakePluginProcessor{process: func(invocation PluginInvocation) (*PluginResponse, error) {
		result := make(map[string]any)
		for key, value := range invocation.Result.(map[string]any) {
			result[key] = value
		}
		result[invocation.Config[pluginStepField].(string)] = true
		return &PluginResponse{Result: result}, nil
	}}
	s.pluginClient = processor

	result := invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
	value := textResult(t, result)
	require.Equal(t, true, value["source"])
	require.Equal(t, true, value["first"])
	require.Equal(t, true, value["second"])
	calls := processor.Calls()
	require.Len(t, calls, 2)
	require.Equal(t, pluginStepFirst, calls[0].Config[pluginStepField])
	require.Equal(t, pluginStepSecond, calls[1].Config[pluginStepField])
}

func TestServerPlugin_UnboundResultPreservesLegacyMCPAndDirectContracts(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("unbound-plugin-tool")
	s.RegisterTool(tool)
	processor := &fakePluginProcessor{process: func(invocation PluginInvocation) (*PluginResponse, error) {
		return &PluginResponse{Result: invocation.Result}, nil
	}}
	s.pluginClient = processor

	mcpResult := invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
	text, ok := mcpResult.Content[0].(mcp.TextContent)
	require.True(t, ok)
	require.Equal(t, `{"source":true}`, text.Text)
	require.Empty(t, processor.Calls())

	request := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(`{"name":"unbound-plugin-tool","arguments":{}}`))
	recorder := httptest.NewRecorder()
	s.handleDirectInvoke(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, `{"result":{"source":true}}
`, recorder.Body.String())
}

func TestServerPlugin_MCPAndDirectUseTheSameTransformedResult(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("equivalent-plugin-tool")
	s.RegisterTool(tool)
	binding := validPluginBindingForTest()
	binding.Spec.ToolRef.Name = tool.Metadata.Name
	binding.Spec.Config = map[string]any{"processed": true}
	installActivePluginBindings(s, tool, binding)
	s.pluginClient = &fakePluginProcessor{process: func(invocation PluginInvocation) (*PluginResponse, error) {
		result := invocation.Result.(map[string]any)
		result["processed"] = invocation.Config["processed"]
		return &PluginResponse{Result: result}, nil
	}}

	mcpResult := invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
	mcpValue := textResult(t, mcpResult)

	request := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(`{"name":"equivalent-plugin-tool","arguments":{}}`))
	recorder := httptest.NewRecorder()
	s.handleDirectInvoke(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var direct ToolResult
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &direct))
	require.Equal(t, mcpValue, direct.Result)
}

func TestServerPlugin_FailurePoliciesAreSafe(t *testing.T) {
	for _, policy := range []string{PluginFailurePolicyContinue, PluginFailurePolicyFail} {
		t.Run(policy, func(t *testing.T) {
			s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
			tool := newPluginPipelineTool("failure-plugin-tool-" + policy)
			s.RegisterTool(tool)
			binding := validPluginBindingForTest()
			binding.Spec.ToolRef.Name = tool.Metadata.Name
			binding.Spec.FailurePolicy = policy
			installActivePluginBindings(s, tool, binding)
			s.pluginClient = &fakePluginProcessor{process: func(PluginInvocation) (*PluginResponse, error) {
				return nil, errors.New("private endpoint and response data")
			}}

			handler := s.handleMCPToolCall(tool.Metadata.Name)
			if policy == PluginFailurePolicyContinue {
				result := invokeMCPHandler(t, handler, tool.Metadata.Name)
				require.Equal(t, true, textResult(t, result)["source"])
				return
			}

			result := invokeMCPHandler(t, handler, tool.Metadata.Name)
			require.True(t, result.IsError)
			require.Contains(t, result.Content, mcp.TextContent{Type: textContentType, Text: pluginProcessingFailureMessage})

			request := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(`{"name":"`+tool.Metadata.Name+`","arguments":{}}`))
			recorder := httptest.NewRecorder()
			s.handleDirectInvoke(recorder, request)
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			var directError map[string]string
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &directError))
			require.Equal(t, map[string]string{pluginErrorField: pluginProcessingFailureMessage}, directError)
			require.NotContains(t, recorder.Body.String(), "private")
		})
	}
}

func TestServerPlugin_ContinueFailureRestoresOriginalResult(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("rollback-plugin-tool")
	s.RegisterTool(tool)
	first := validPluginBindingForTest()
	first.Spec.ToolRef.Name = tool.Metadata.Name
	first.Metadata.Name = "first-transform"
	first.Spec.Priority = 10
	first.Spec.Config = map[string]any{pluginStepField: pluginStepFirst}
	second := validPluginBindingForTest()
	second.Spec.ToolRef.Name = tool.Metadata.Name
	second.Metadata.Name = "second-failure"
	second.Spec.Priority = 20
	second.Spec.Config = map[string]any{pluginStepField: pluginStepSecond}
	installActivePluginBindings(s, tool, first, second)
	s.pluginClient = &fakePluginProcessor{process: func(invocation PluginInvocation) (*PluginResponse, error) {
		if invocation.Config[pluginStepField] == pluginStepSecond {
			return nil, errors.New("second binding failed")
		}
		original := invocation.Result.(map[string]any)
		result := make(map[string]any, len(original)+1)
		for key, value := range original {
			result[key] = value
		}
		result[pluginStepFirst] = true
		return &PluginResponse{Result: result}, nil
	}}

	result := invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
	value := textResult(t, result)
	require.Equal(t, true, value[pluginResultSourceField])
	require.NotContains(t, value, pluginStepFirst)
}

func TestServerPlugin_RevalidatesTransformedOutput(t *testing.T) {
	schema := any(map[string]any{
		pluginSchemaTypeField: "object",
		"required":            []string{"source"},
	})
	validationTool := &Tool{Spec: ToolSpec{OutputSchema: &schema}}
	require.NoError(t, validationTool.ValidateResult(map[string]any{"source": true}))
	require.Error(t, validationTool.ValidateResult(map[string]any{pluginInvalidField: true}))
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("schema-plugin-tool")
	tool.Spec.OutputSchema = &schema
	s.RegisterTool(tool)
	binding := validPluginBindingForTest()
	binding.Spec.ToolRef.Name = tool.Metadata.Name
	installActivePluginBindings(s, tool, binding)
	s.pluginClient = &fakePluginProcessor{process: func(PluginInvocation) (*PluginResponse, error) {
		return &PluginResponse{Result: map[string]any{pluginInvalidField: true}}, nil
	}}

	result := invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
	require.Equal(t, true, textResult(t, result)["source"])

	binding.Spec.FailurePolicy = PluginFailurePolicyFail
	installActivePluginBindings(s, tool, binding)
	result = invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
	require.True(t, result.IsError)
	require.Contains(t, result.Content, mcp.TextContent{Type: textContentType, Text: pluginProcessingFailureMessage})

	baseInvalid := newPluginPipelineTool("base-invalid-plugin-tool")
	baseInvalid.Spec.OutputSchema = &schema
	baseInvalid.Handler = func(context.Context, map[string]any) (*ToolResult, error) {
		return &ToolResult{Result: map[string]any{pluginInvalidField: true}}, nil
	}
	s.RegisterTool(baseInvalid)
	baseBinding := validPluginBindingForTest()
	baseBinding.Spec.ToolRef.Name = baseInvalid.Metadata.Name
	installActivePluginBindings(s, baseInvalid, baseBinding)
	processor := &fakePluginProcessor{process: func(invocation PluginInvocation) (*PluginResponse, error) {
		return &PluginResponse{Result: invocation.Result}, nil
	}}
	s.pluginClient = processor
	invokeMCPHandler(t, s.handleMCPToolCall(baseInvalid.Metadata.Name), baseInvalid.Metadata.Name)
	require.Empty(t, processor.Calls())
}

func TestServerPlugin_MissingCredentialMCPAndDirectPoliciesMakeNoOutboundCalls(t *testing.T) {
	for _, policy := range []string{PluginFailurePolicyContinue, PluginFailurePolicyFail} {
		t.Run(policy, func(t *testing.T) {
			const missingCredentialRef = "PLUGIN_MISSING_RUNTIME_CREDENTIAL" // #nosec G101 -- environment-variable reference used by this test.
			t.Setenv(missingCredentialRef, "")

			calls := 0
			client := NewPluginClient(&http.Client{Transport: pluginRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("plugin transport must not be called")
			})})
			s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
			tool := newPluginPipelineTool("missing-credential-tool-" + policy)
			s.RegisterTool(tool)
			binding := validPluginBindingForTest()
			binding.Spec.ToolRef.Name = tool.Metadata.Name
			binding.Spec.FailurePolicy = policy
			plugin := validPluginForTest("https://plugin.example.test")
			plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: missingCredentialRef}
			s.pluginRegistry.Replace(buildPluginBindingSnapshot([]*Plugin{&plugin}, []*PluginBinding{&binding}, []*Tool{tool}))
			s.pluginClient = client

			mcpResult := invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
			if policy == PluginFailurePolicyContinue {
				require.False(t, mcpResult.IsError)
				require.Equal(t, true, textResult(t, mcpResult)[pluginResultSourceField])
			} else {
				require.True(t, mcpResult.IsError)
				require.Contains(t, mcpResult.Content, mcp.TextContent{Type: textContentType, Text: pluginProcessingFailureMessage})
			}

			request := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(`{"name":"`+tool.Metadata.Name+`","arguments":{}}`))
			recorder := httptest.NewRecorder()
			s.handleDirectInvoke(recorder, request)
			if policy == PluginFailurePolicyContinue {
				require.Equal(t, http.StatusOK, recorder.Code)
				require.JSONEq(t, `{"result":{"source":true}}`, recorder.Body.String())
			} else {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
				require.JSONEq(t, `{"error":"`+pluginProcessingFailureMessage+`"}`, recorder.Body.String())
			}
			require.Zero(t, calls)
		})
	}
}

func TestServerPlugin_CacheCredentialRotationUsesNewValueAfterFlush(t *testing.T) {
	const credentialA = "plugin-credential-a" // #nosec G101 -- test-only rotation sentinel.
	const credentialB = "plugin-credential-b" // #nosec G101 -- test-only rotation sentinel.
	t.Setenv(pluginTestCredentialRef, credentialA)

	var headers []string
	transport := pluginRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		headers = append(headers, request.Header.Get(pluginAuthorizationHeader))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"result":{"source":true,"processed":true}}`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	cacheManager := cache.NewMemoryManager(10, logger.Init())
	s := NewServer(nil, cacheManager, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("rotating-cached-plugin-tool")
	tool.Spec.Cache = &cache.Config{Enabled: true, TTLSeconds: 60}
	s.RegisterTool(tool)
	binding := validPluginBindingForTest()
	binding.Spec.ToolRef.Name = tool.Metadata.Name
	plugin := validPluginForTest("https://plugin.example.test")
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginTestCredentialRef}
	s.pluginRegistry.Replace(buildPluginBindingSnapshot([]*Plugin{&plugin}, []*PluginBinding{&binding}, []*Tool{tool}))
	s.pluginClient = NewPluginClient(&http.Client{Transport: transport})
	handler := s.CacheMiddleware(tool)(s.handleMCPToolCall(tool.Metadata.Name))

	invokeMCPHandler(t, handler, tool.Metadata.Name)
	invokeMCPHandler(t, handler, tool.Metadata.Name)
	require.Equal(t, []string{"Bearer " + credentialA}, headers)

	t.Setenv(pluginTestCredentialRef, credentialB)
	t.Setenv(authTokenEnv, "runtime-admin-token")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")

	unauthenticatedFlush := httptest.NewRequest(http.MethodPost, "/api/cache/flush?tool="+tool.Metadata.Name, nil)
	unauthenticatedRecorder := httptest.NewRecorder()
	mux.ServeHTTP(unauthenticatedRecorder, unauthenticatedFlush)
	require.Equal(t, http.StatusUnauthorized, unauthenticatedRecorder.Code)

	authenticatedFlush := httptest.NewRequest(http.MethodPost, "/api/cache/flush?tool="+tool.Metadata.Name, nil)
	authenticatedFlush.Header.Set("Authorization", "Bearer runtime-admin-token")
	authenticatedRecorder := httptest.NewRecorder()
	mux.ServeHTTP(authenticatedRecorder, authenticatedFlush)
	require.Equal(t, http.StatusOK, authenticatedRecorder.Code)

	// Recreate the runtime client to model a rollout after credential rotation.
	s.pluginClient = NewPluginClient(&http.Client{Transport: transport})
	invokeMCPHandler(t, handler, tool.Metadata.Name)
	require.Equal(t, []string{"Bearer " + credentialA, "Bearer " + credentialB}, headers)
}

func TestServerPlugin_CacheHitCallsPluginOnce(t *testing.T) {
	cacheManager := cache.NewMemoryManager(10, logger.Init())
	s := NewServer(nil, cacheManager, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("cached-plugin-tool")
	tool.Spec.Cache = &cache.Config{Enabled: true, TTLSeconds: 60}
	s.RegisterTool(tool)
	binding := validPluginBindingForTest()
	binding.Spec.ToolRef.Name = tool.Metadata.Name
	installActivePluginBindings(s, tool, binding)
	processor := &fakePluginProcessor{process: func(invocation PluginInvocation) (*PluginResponse, error) {
		result := invocation.Result.(map[string]any)
		result["processed"] = true
		return &PluginResponse{Result: result}, nil
	}}
	s.pluginClient = processor
	handler := s.CacheMiddleware(tool)(s.handleMCPToolCall(tool.Metadata.Name))

	invokeMCPHandler(t, handler, tool.Metadata.Name)
	invokeMCPHandler(t, handler, tool.Metadata.Name)
	require.Len(t, processor.Calls(), 1)
}

func TestServerPlugin_DeniedAndERPErrorCallsNoPlugin(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := newPluginPipelineTool("guarded-plugin-tool")
	tool.Spec.Security.AllowedRoles = []string{"admin"}
	s.RegisterTool(tool)
	binding := validPluginBindingForTest()
	binding.Spec.ToolRef.Name = tool.Metadata.Name
	installActivePluginBindings(s, tool, binding)
	processor := &fakePluginProcessor{process: func(invocation PluginInvocation) (*PluginResponse, error) {
		return &PluginResponse{Result: invocation.Result}, nil
	}}
	s.pluginClient = processor

	request := mcp.CallToolRequest{}
	request.Params.Name = tool.Metadata.Name
	request.Params.Arguments = map[string]any{}
	guarded := s.RoleAuthzMiddleware(tool)(s.handleMCPToolCall(tool.Metadata.Name))
	result, err := guarded(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Empty(t, processor.Calls())

	errTool := newPluginPipelineTool("erp-error-plugin-tool")
	errTool.Handler = func(context.Context, map[string]any) (*ToolResult, error) {
		return &ToolResult{Result: map[string]any{pluginResultSourceField: true}, IsError: true}, nil
	}
	s.RegisterTool(errTool)
	errBinding := validPluginBindingForTest()
	errBinding.Spec.ToolRef.Name = errTool.Metadata.Name
	installActivePluginBindings(s, errTool, errBinding)
	errResult := invokeMCPHandler(t, s.handleMCPToolCall(errTool.Metadata.Name), errTool.Metadata.Name)
	require.False(t, errResult.IsError, "legacy MCP serialization must remain unchanged for ERP error results")
	require.Empty(t, processor.Calls())
}
