package mcp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
)

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
