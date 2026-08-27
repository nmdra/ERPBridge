package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/security"
	"github.com/stretchr/testify/require"
)

func TestCredentialRotationUsesNewFileWithoutRestart(t *testing.T) {
	t.Setenv(authTokenEnv, "admin-token")
	dir := t.TempDir()
	t.Setenv("ERPBRIDGE_CREDENTIALS_DIR", dir)
	credentialPath := filepath.Join(dir, "ERP_ROTATION_KEY")
	require.NoError(t, os.WriteFile(credentialPath, []byte("rotation-version-a"), 0600))

	var received []string
	erp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer erp.Close()
	erpURL, err := url.Parse(erp.URL)
	require.NoError(t, err)
	t.Setenv(security.InsecureAuthAllowedHostsEnv, erpURL.Host)

	server := NewServer(connector.NewClient(logger.Init()), cache.NewMemoryManager(10, logger.Init()), logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	fileTool := &Tool{
		APIVersion: "erpbridge.io/v1",
		Kind:       "MCPTool",
		Metadata:   Metadata{Name: "rotation-file-tool", Version: "1.0.0", Module: "integration"},
		Spec: ToolSpec{
			Description: Description{Short: "Read a rotation fixture"},
			InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
			Execution:   Execution{Type: "http", Method: http.MethodGet, Endpoint: erp.URL},
			Cache:       &cache.Config{Enabled: true, TTLSeconds: 60, IsReadOnly: true},
			Security:    Security{AuthType: "api-key", CredentialRef: "ERP_ROTATION_KEY", CredentialSource: "file", DataClass: "public"}, // #nosec G101 -- logical credential reference, not a secret.
		},
	}
	server.RegisterTool(fileTool)

	mux := http.NewServeMux()
	server.ServeHTTP(mux, "")
	bridge := httptest.NewServer(mux)
	defer bridge.Close()
	invoke := func(name string) int {
		body, marshalErr := json.Marshal(ToolCallRequest{Name: name, Arguments: map[string]any{}})
		require.NoError(t, marshalErr)
		request, requestErr := http.NewRequest(http.MethodPost, bridge.URL+"/api/tools/invoke", bytes.NewReader(body))
		require.NoError(t, requestErr)
		request.Header.Set("Authorization", "Bearer admin-token")
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := http.DefaultClient.Do(request)
		require.NoError(t, requestErr)
		defer func() { _ = response.Body.Close() }()
		return response.StatusCode
	}

	require.Equal(t, http.StatusOK, invoke(fileTool.Metadata.Name))
	temporary := filepath.Join(dir, ".ERP_ROTATION_KEY.next")
	require.NoError(t, os.WriteFile(temporary, []byte("rotation-version-b"), 0600))
	require.NoError(t, os.Rename(temporary, credentialPath))
	require.Equal(t, http.StatusOK, invoke(fileTool.Metadata.Name))
	require.Equal(t, []string{"rotation-version-a", "rotation-version-b"}, received)

	require.NoError(t, os.WriteFile(temporary, []byte("rotation-version-\n"), 0600))
	require.NoError(t, os.Rename(temporary, credentialPath))
	require.NotEqual(t, http.StatusOK, invoke(fileTool.Metadata.Name))
	require.Equal(t, 2, len(received))

	t.Setenv("ERP_ENV_ROTATION_KEY", "environment-value")
	envTool := *fileTool
	envTool.Metadata.Name = "rotation-env-tool"
	envTool.Spec.Security = Security{AuthType: "api-key", CredentialRef: "ERP_ENV_ROTATION_KEY"} // #nosec G101 -- logical credential reference, not a secret.
	server.RegisterTool(&envTool)
	require.Equal(t, http.StatusOK, invoke(envTool.Metadata.Name))
	require.Equal(t, 3, len(received))
}

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
