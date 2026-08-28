package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_RegisterToolAdvertisesOutputSchema(t *testing.T) {
	log := logger.Init()
	s := NewServer(&MockConnector{}, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	schema := any(map[string]any{
		"type":       schemaTypeObject,
		"properties": map[string]any{"text": map[string]any{"type": schemaTypeString}},
		"required":   []string{"text"},
	})
	tool := &Tool{
		Metadata: Metadata{Name: "schema-advertised-tool", Version: testVersion100},
		Spec: ToolSpec{
			Description:  Description{Short: "schema tool"},
			InputSchema:  InputSchema{Type: schemaTypeObject},
			OutputSchema: &schema,
		},
	}
	s.RegisterTool(tool)

	registered := s.mcpServer.ListTools()[tool.Metadata.Name]
	encoded, err := json.Marshal(registered.Tool)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	require.Equal(t, schemaTypeObject, payload["outputSchema"].(map[string]any)["type"])
}

func TestServer_RegisterTool(t *testing.T) {
	log := logger.Init()
	mockConn := &MockConnector{}
	s := NewServer(mockConn, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")

	tool := &Tool{
		Metadata: Metadata{
			Name:    testToolName,
			Version: testVersion100,
		},
		Spec: ToolSpec{
			Description: Description{Short: "A test tool"},
			InputSchema: InputSchema{
				Type: schemaTypeObject,
				Properties: map[string]Property{
					"param1": {Type: schemaTypeString},
				},
			},
		},
	}

	s.RegisterTool(tool)

	registered, err := s.registry.Resolve(testToolName, "")
	assert.NoError(t, err)
	assert.NotNil(t, registered)
	assert.Equal(t, testToolName, registered.Metadata.Name)
}

func TestServer_RegisterToolProjectsAnnotationsAndMeta(t *testing.T) {
	log := logger.Init()
	s := NewServer(&MockConnector{}, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	readOnly, destructive, idempotent, openWorld := true, false, true, true
	tool := &Tool{
		Metadata: Metadata{Name: "list-orders", Version: testVersion100},
		Spec: ToolSpec{
			Description: Description{
				Short:        "List customer orders",
				WhenToUse:    []string{"When the user asks to review orders"},
				WhenNotToUse: []string{"When the user wants to create an order"},
				Examples:     []string{"Show my recent orders"},
			},
			Annotations: &ToolAnnotations{
				Title:           "List orders",
				ReadOnlyHint:    &readOnly,
				DestructiveHint: &destructive,
				IdempotentHint:  &idempotent,
				OpenWorldHint:   &openWorld,
			},
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{}},
			Security:    Security{AllowedRoles: []string{"sales_read", "sales_manager"}},
		},
	}
	s.RegisterTool(tool)

	registered := s.mcpServer.ListTools()[tool.Metadata.Name].Tool
	assert.Equal(t, "List orders", registered.Title)
	require.Equal(t, "List orders", registered.Annotations.Title)
	require.Equal(t, true, *registered.Annotations.ReadOnlyHint)
	require.Equal(t, false, *registered.Annotations.DestructiveHint)
	require.Equal(t, true, *registered.Annotations.IdempotentHint)
	require.Equal(t, true, *registered.Annotations.OpenWorldHint)
	require.NotNil(t, registered.Meta)

	encoded, err := json.Marshal(registered)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	meta := payload["_meta"].(map[string]any)
	assert.Equal(t, []any{"When the user asks to review orders"}, meta["io.erpbridge/whenToUse"])
	assert.Equal(t, []any{"When the user wants to create an order"}, meta["io.erpbridge/whenNotToUse"])
	assert.Equal(t, []any{"Show my recent orders"}, meta["io.erpbridge/examples"])
	assert.Equal(t, []any{"sales_manager", "sales_read"}, meta["io.erpbridge/allowedRoles"])
}

func TestServer_RegisterToolOmitsEmptyMCPMeta(t *testing.T) {
	log := logger.Init()
	s := NewServer(&MockConnector{}, nil, log, RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "empty-meta-tool", Version: testVersion100},
		Spec: ToolSpec{
			Description: Description{Short: "No metadata"},
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{}},
		},
	}
	s.RegisterTool(tool)

	assert.Nil(t, s.mcpServer.ListTools()[tool.Metadata.Name].Tool.Meta)
}

func TestToolAnnotationsAreOptionalAndPreserveExplicitFalse(t *testing.T) {
	tool := Tool{Spec: ToolSpec{}}
	encoded, err := json.Marshal(tool)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	assert.NotContains(t, payload["spec"], "annotations")

	falseValue := false
	tool.Spec.Annotations = &ToolAnnotations{DestructiveHint: &falseValue}
	encoded, err = json.Marshal(tool)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &payload))
	spec := payload["spec"].(map[string]any)
	annotations := spec["annotations"].(map[string]any)
	assert.Equal(t, false, annotations["destructiveHint"])
}

func TestTool_CallERP_CapturesResponseBeforeDecoding(t *testing.T) {
	mockConn := &MockConnector{
		CallWithOptionsFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader, options connector.CallOptions) (*http.Response, error) {
			assert.True(t, options.PreserveErrorResponses)
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     http.Header{"Content-Type": []string{"IMAGE/PNG; charset=binary"}},
				Body:       io.NopCloser(bytes.NewReader([]byte{0x89, 'P', 'N', 'G'})),
			}, nil
		},
	}
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
		Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint},
	}}

	response, err := tool.CallERP(context.Background(), nil, mockConn, connector.CallOptions{PreserveErrorResponses: true})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, response.Status)
	assert.Equal(t, "image/png", response.ContentType)
	assert.Equal(t, []byte{0x89, 'P', 'N', 'G'}, response.Body)
}

func TestTool_CallERP_CapturesEmptyAndMalformedBodies(t *testing.T) {
	for name, body := range map[string][]byte{
		"empty":     []byte{},
		"malformed": []byte(`{"not":`),
	} {
		t.Run(name, func(t *testing.T) {
			mockConn := &MockConnector{CallWithOptionsFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader, _ connector.CallOptions) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
			}}
			tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint}}}

			response, err := tool.CallERP(context.Background(), nil, mockConn, connector.CallOptions{PreserveErrorResponses: true})

			assert.NoError(t, err)
			assert.Equal(t, body, response.Body)
		})
	}
}

func TestTool_CallERP_RejectsOversizedResponse(t *testing.T) {
	mockConn := &MockConnector{CallWithOptionsFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader, _ connector.CallOptions) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, MaxPluginJSONBytes+1)))}, nil
	}}
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint}}}

	response, err := tool.CallERP(context.Background(), nil, mockConn, connector.CallOptions{PreserveErrorResponses: true})

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestTool_PrepareERPCallRoutesGeneratedParameterLocations(t *testing.T) {
	t.Setenv("ERP_BASE_URL", "")
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
		Execution: Execution{
			Method:             http.MethodPost,
			Endpoint:           "https://erp.example/items/{id}",
			Mapping:            map[string]string{"item_id": "id", "search": "q", "trace": "X-Trace"},
			ParameterLocations: map[string]string{"item_id": "path", "search": "query", "trace": "header", "details": "body"},
		},
	}}

	ep, query, body, err := tool.prepareERPCall(map[string]any{
		"item_id": "a/b",
		"search":  "open items",
		"trace":   "trace-1",
		"details": map[string]any{"enabled": true},
	})
	require.NoError(t, err)
	require.Equal(t, "https://erp.example/items/a%2Fb", ep.Path)
	require.Equal(t, "open items", query.Get("q"))
	require.Equal(t, "trace-1", ep.Headers["X-Trace"])
	require.NotNil(t, body)
	encoded, readErr := io.ReadAll(body)
	require.NoError(t, readErr)
	var bodyValue map[string]any
	require.NoError(t, json.Unmarshal(encoded, &bodyValue))
	require.Equal(t, map[string]any{"enabled": true}, bodyValue["details"])
}

func TestTool_PrepareERPCallUsesLegacyFallbackWithoutLocationMetadata(t *testing.T) {
	t.Setenv("ERP_BASE_URL", "")
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
		Execution: Execution{Method: http.MethodPost, Endpoint: "https://erp.example/items"},
	}}

	_, query, body, err := tool.prepareERPCall(map[string]any{"name": "item-1"})
	require.NoError(t, err)
	require.Empty(t, query)
	require.NotNil(t, body)
}

func TestTool_PrepareERPCallSerializesCompletePrimitiveBody(t *testing.T) {
	t.Setenv("ERP_BASE_URL", "")
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
		Execution: Execution{
			Method:       http.MethodPost,
			Endpoint:     "https://erp.example/items",
			BodyArgument: "body",
			ParameterLocations: map[string]string{
				"body": "body",
			},
		},
	}}

	_, _, body, err := tool.prepareERPCall(map[string]any{"body": []any{"item-1", "item-2"}})
	require.NoError(t, err)
	encoded, readErr := io.ReadAll(body)
	require.NoError(t, readErr)
	require.JSONEq(t, `["item-1","item-2"]`, string(encoded))
}

func TestTool_Execute(t *testing.T) {
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			respBody := `{"status": "success"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(respBody)),
			}, nil
		},
	}

	tool := &Tool{
		Metadata: Metadata{Name: testToolName},
		Spec: ToolSpec{
			Execution: Execution{
				Method:   http.MethodGet,
				Endpoint: testEndpoint,
			},
		},
	}

	ctx := context.Background()
	args := map[string]any{"key": testValue}
	result, err := tool.Execute(ctx, args, mockConn)

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	resultMap, ok := result.Result.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "success", resultMap["status"])
}

func TestTool_Execute_SuccessfulHeadAndNoContentSkipDecoding(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		status int
	}{
		{name: "head", method: http.MethodHead, status: http.StatusOK},
		{name: "no content", method: http.MethodGet, status: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			mockConn := &MockConnector{CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(bytes.NewBufferString("not-json"))}, nil
			}}
			tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
				Execution: Execution{Method: test.method, Endpoint: testEndpoint},
			}}

			result, err := tool.Execute(context.Background(), nil, mockConn)
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.Nil(t, result.Result)
		})
	}
}

func TestTool_Execute_MalformedNonEmptyResponseRemainsError(t *testing.T) {
	mockConn := &MockConnector{CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("not-json"))}, nil
	}}
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
		Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint},
	}}

	_, err := tool.Execute(context.Background(), nil, mockConn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode erp response")
}

func TestTool_Execute_Error(t *testing.T) {
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal server error"}`)),
			}, nil
		},
	}

	tool := &Tool{
		Metadata: Metadata{Name: testToolName},
		Spec: ToolSpec{
			Execution: Execution{
				Method:   http.MethodPost,
				Endpoint: testEndpoint,
			},
		},
	}

	ctx := context.Background()
	args := map[string]any{"key": testValue}
	result, err := tool.Execute(ctx, args, mockConn)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestTool_Execute_MissingCredentialRefFailsClosed(t *testing.T) {
	t.Setenv("ERPBRIDGE_TEST_SECRET", "")
	called := false
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			called = true
			return nil, nil
		},
	}

	tool := &Tool{
		Metadata: Metadata{Name: testToolName},
		Spec: ToolSpec{
			Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint},
			// #nosec G101 -- this is an environment-variable reference used by the test.
			Security: Security{CredentialRef: "ERPBRIDGE_TEST_SECRET"},
		},
	}

	_, err := tool.Execute(context.Background(), nil, mockConn)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "credential reference")
	assert.False(t, called, "the connector must not receive a call when a credential reference is unresolved")
}

func TestTool_Execute_EmptyCredentialRefDoesNotRequireAuth(t *testing.T) {
	called := false
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, ep connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			called = true
			assert.Empty(t, ep.Auth.Key)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"status":"ok"}`)),
			}, nil
		},
	}

	tool := &Tool{
		Metadata: Metadata{Name: testToolName},
		Spec: ToolSpec{
			Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint},
		},
	}

	_, err := tool.Execute(context.Background(), nil, mockConn)

	assert.NoError(t, err)
	assert.True(t, called)
}

func TestTool_Execute_ResponsePathSupportsNestedArrays(t *testing.T) {
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data":{"items":[{"id":"item-1"},{"id":"item-2"}]}}`)),
			}, nil
		},
	}
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
		Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint, ResponsePath: "data.items[1].id"},
	}}

	result, err := tool.Execute(context.Background(), nil, mockConn)

	assert.NoError(t, err)
	assert.Equal(t, "item-2", result.Result)
}

func TestTool_Execute_MissingResponsePathReturnsError(t *testing.T) {
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data":{}}`)),
			}, nil
		},
	}
	tool := &Tool{Metadata: Metadata{Name: testToolName}, Spec: ToolSpec{
		Execution: Execution{Method: http.MethodGet, Endpoint: testEndpoint, ResponsePath: "data.missing"},
	}}

	_, err := tool.Execute(context.Background(), nil, mockConn)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "response path")
}
