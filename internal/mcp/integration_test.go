package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestRawResponseImageToTextPluginIntegration(t *testing.T) {
	t.Setenv(authTokenEnv, "admin-token")
	imageBytes := []byte{0x89, 'P', 'N', 'G'}
	pluginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.NotContains(t, payload, "result")
		require.NotContains(t, payload, "headers")
		require.NotContains(t, payload, "url")
		require.NotContains(t, payload, "credentials")
		require.NotContains(t, payload, "caller")
		raw, ok := payload["rawResponse"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, float64(http.StatusOK), raw["status"])
		require.Equal(t, "image/png", raw["contentType"])
		require.Equal(t, map[string]any{"encoding": "base64", "value": "iVBORw=="}, raw["body"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"text":"OCR text"}}`))
	}))
	defer pluginServer.Close()

	pluginURL, err := url.Parse(pluginServer.URL)
	require.NoError(t, err)
	t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", pluginURL.Host)
	tool := rawResponseTool("raw-image-integration-tool")
	erp := &MockConnector{CallWithOptionsFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader, options connector.CallOptions) (*http.Response, error) {
		require.True(t, options.PreserveErrorResponses)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"image/png"},
				"X-ERP-Secret": []string{"must-not-forward"},
			},
			Body: io.NopCloser(bytes.NewReader(imageBytes)),
		}, nil
	}}
	s := NewServer(erp, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	s.RegisterTool(tool)
	plugin := validPluginForTest(pluginServer.URL)
	binding := validPluginBindingForTest()
	binding.Spec.ToolRef.Name = tool.Metadata.Name
	binding.Spec.Phase = PluginPhaseRawResponse
	binding.Spec.FailurePolicy = PluginFailurePolicyFail
	s.pluginRegistry.Replace(buildPluginBindingSnapshot([]*Plugin{&plugin}, []*PluginBinding{&binding}, []*Tool{tool}))

	result := invokeMCPHandler(t, s.handleMCPToolCall(tool.Metadata.Name), tool.Metadata.Name)
	require.False(t, result.IsError)
	require.Equal(t, map[string]any{"text": "OCR text"}, result.StructuredContent)
	require.Equal(t, "OCR text", result.Content[0].(mcp.TextContent).Text)
}
