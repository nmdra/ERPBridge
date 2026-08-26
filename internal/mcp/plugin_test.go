package mcp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	pluginTestVersion        = testVersion100
	pluginTestModeKey        = "mode"
	pluginTestMode           = "safe"
	pluginTestResultID       = "order-1"
	pluginTestToolName       = "list-orders"
	pluginTimeoutError       = "timeout"
	pluginVersionError       = "metadata.version"
	pluginAuthHeaderError    = "spec.auth.header"
	pluginTestCredentialRef  = "PLUGIN_TEST_TOKEN" // #nosec G101 -- this is an environment-variable reference used by tests.
	pluginHTTPValidationText = "http"
)

func validPluginForTest(endpoint string) Plugin {
	return Plugin{
		APIVersion: "erpbridge.io/v1",
		Kind:       "Plugin",
		Metadata: PluginMetadata{
			Name:     "response-transformer",
			Version:  pluginTestVersion,
			IsActive: true,
		},
		Spec: PluginSpec{
			Endpoint:            endpoint,
			TimeoutMilliseconds: 250,
		},
	}
}

func validPluginBindingForTest() PluginBinding {
	return PluginBinding{
		APIVersion: "erpbridge.io/v1",
		Kind:       "PluginBinding",
		Metadata: PluginBindingMetadata{
			Name:     "transform-orders",
			IsActive: true,
		},
		Spec: PluginBindingSpec{
			PluginRef:     PluginRef{Name: "response-transformer", Version: pluginTestVersion},
			ToolRef:       ToolRef{Name: pluginTestToolName, Version: pluginTestVersion},
			Phase:         PluginPhaseAfterResponse,
			Priority:      10,
			FailurePolicy: PluginFailurePolicyContinue,
			Config: map[string]any{
				pluginTestModeKey: pluginTestMode,
			},
		},
	}
}

func TestPlugin_Validate(t *testing.T) {
	tests := map[string]struct {
		mutate  func(*Plugin)
		wantErr string
	}{
		"accepts valid resource": {
			mutate: func(plugin *Plugin) {
				plugin.Spec.Endpoint = "https://plugins.example.test/transformer"
			},
			wantErr: "",
		},
		"requires api version": {
			mutate:  func(plugin *Plugin) { plugin.APIVersion = "" },
			wantErr: "apiVersion",
		},
		"requires kind": {
			mutate:  func(plugin *Plugin) { plugin.Kind = "WrongKind" },
			wantErr: "kind",
		},
		"requires name": {
			mutate:  func(plugin *Plugin) { plugin.Metadata.Name = "" },
			wantErr: "metadata.name",
		},
		"requires version": {
			mutate:  func(plugin *Plugin) { plugin.Metadata.Version = "" },
			wantErr: pluginVersionError,
		},
		"rejects invalid version": {
			mutate:  func(plugin *Plugin) { plugin.Metadata.Version = "not-semver" },
			wantErr: pluginVersionError,
		},
		"rejects missing endpoint": {
			mutate:  func(plugin *Plugin) { plugin.Spec.Endpoint = "" },
			wantErr: "endpoint",
		},
		"rejects non-http endpoint": {
			mutate:  func(plugin *Plugin) { plugin.Spec.Endpoint = "ftp://plugins.example.test" },
			wantErr: pluginHTTPValidationText,
		},
		"rejects endpoint userinfo": {
			mutate:  func(plugin *Plugin) { plugin.Spec.Endpoint = "https://user:secret@plugins.example.test" },
			wantErr: "userinfo",
		},
		"rejects endpoint query": {
			mutate:  func(plugin *Plugin) { plugin.Spec.Endpoint = "https://plugins.example.test?token=secret" },
			wantErr: "query",
		},
		"rejects endpoint fragment": {
			mutate:  func(plugin *Plugin) { plugin.Spec.Endpoint = "https://plugins.example.test#fragment" },
			wantErr: "fragment",
		},
		"canonicalizes omitted plugin type": {
			mutate:  func(_ *Plugin) {},
			wantErr: "",
		},
		"accepts docker plugin type": {
			mutate:  func(plugin *Plugin) { plugin.Metadata.Type = PluginTypeDocker },
			wantErr: "",
		},
		"rejects unsupported plugin type": {
			mutate:  func(plugin *Plugin) { plugin.Metadata.Type = "binary" },
			wantErr: "metadata.type",
		},
		"accepts bearer auth": {
			mutate: func(plugin *Plugin) {
				plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginTestCredentialRef}
			},
			wantErr: "",
		},
		"rejects plugin auth outside plugin prefix": {
			mutate: func(plugin *Plugin) {
				// #nosec G101 -- this is an environment-variable reference used by the test.
				plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: "API_AUTH_TOKEN"}
			},
			wantErr: "credentialRef",
		},
		"rejects bearer custom header": {
			mutate: func(plugin *Plugin) {
				plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginTestCredentialRef, Header: pluginDefaultAPIKeyHeader}
			},
			wantErr: pluginAuthHeaderError,
		},
		"rejects reserved API key header": {
			mutate: func(plugin *Plugin) {
				plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeAPIKey, CredentialRef: pluginTestCredentialRef, Header: pluginAuthorizationHeader}
			},
			wantErr: pluginAuthHeaderError,
		},
		"rejects malformed API key header": {
			mutate: func(plugin *Plugin) {
				plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeAPIKey, CredentialRef: pluginTestCredentialRef, Header: "X API Key"}
			},
			wantErr: pluginAuthHeaderError,
		},
		"rejects hop by hop API key header": {
			mutate: func(plugin *Plugin) {
				plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeAPIKey, CredentialRef: pluginTestCredentialRef, Header: "Upgrade"}
			},
			wantErr: pluginAuthHeaderError,
		},
		"rejects zero timeout": {
			mutate:  func(plugin *Plugin) { plugin.Spec.TimeoutMilliseconds = 0 },
			wantErr: pluginTimeoutError,
		},
		"rejects negative timeout": {
			mutate:  func(plugin *Plugin) { plugin.Spec.TimeoutMilliseconds = -1 },
			wantErr: pluginTimeoutError,
		},
		"rejects excessive timeout": {
			mutate:  func(plugin *Plugin) { plugin.Spec.TimeoutMilliseconds = 10 * 60 * 1000 },
			wantErr: pluginTimeoutError,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plugin := validPluginForTest("http://plugins.example.test")
			test.mutate(&plugin)
			err := plugin.Validate()
			if test.wantErr == "" {
				require.NoError(t, err)
				if name == "canonicalizes omitted plugin type" {
					require.Equal(t, PluginTypeAPI, plugin.Metadata.Type)
				}
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestPluginAuthRejectsReservedHeaders(t *testing.T) {
	for header := range reservedPluginAuthHeaders {
		t.Run(header, func(t *testing.T) {
			plugin := validPluginForTest("https://plugins.example.test")
			plugin.Spec.Auth = &PluginAuth{
				Type:          PluginAuthTypeAPIKey,
				CredentialRef: pluginTestCredentialRef,
				Header:        header,
			}
			require.ErrorContains(t, plugin.Validate(), pluginAuthHeaderError)
		})
	}
}

func TestPluginBinding_Validate(t *testing.T) {
	tests := map[string]struct {
		mutate  func(*PluginBinding)
		wantErr bool
	}{
		"accepts valid binding resource":     {},
		"requires name":                      {mutate: func(binding *PluginBinding) { binding.Metadata.Name = "" }, wantErr: true},
		"requires plugin name":               {mutate: func(binding *PluginBinding) { binding.Spec.PluginRef.Name = "" }, wantErr: true},
		"requires plugin version":            {mutate: func(binding *PluginBinding) { binding.Spec.PluginRef.Version = "" }, wantErr: true},
		"requires tool name":                 {mutate: func(binding *PluginBinding) { binding.Spec.ToolRef.Name = "" }, wantErr: true},
		"requires tool version":              {mutate: func(binding *PluginBinding) { binding.Spec.ToolRef.Version = "" }, wantErr: true},
		"accepts raw response phase":         {mutate: func(binding *PluginBinding) { binding.Spec.Phase = PluginPhaseRawResponse }},
		"rejects unsupported phase":          {mutate: func(binding *PluginBinding) { binding.Spec.Phase = "before_request" }, wantErr: true},
		"rejects unsupported failure policy": {mutate: func(binding *PluginBinding) { binding.Spec.FailurePolicy = "retry" }, wantErr: true},
		"rejects negative priority":          {mutate: func(binding *PluginBinding) { binding.Spec.Priority = -1 }, wantErr: true},
		"rejects non-json config":            {mutate: func(binding *PluginBinding) { binding.Spec.Config = map[string]any{"bad": make(chan int)} }, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			binding := validPluginBindingForTest()
			if test.mutate != nil {
				test.mutate(&binding)
			}
			err := binding.Validate()
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPluginInvocation_MarshalPreservesLegacyResultNull(t *testing.T) {
	invocation := validPluginInvocationForTest()
	invocation.Result = nil

	encoded, err := json.Marshal(invocation)
	require.NoError(t, err)
	require.JSONEq(t, `{"protocolVersion":"v1","invocationId":"invocation-123","tool":{"name":"list-orders","version":"1.0.0"},"result":null,"config":{"mode":"safe"}}`, string(encoded))
}

func TestPluginInvocation_MarshalOmitsResultForRawResponse(t *testing.T) {
	invocation := validPluginInvocationForTest()
	invocation.Result = nil
	invocation.RawResponse = &PluginRawResponse{
		Status:      http.StatusOK,
		ContentType: "image/png",
		Body: PluginRawBody{
			Encoding: PluginRawBodyEncodingBase64,
			Value:    "aGVsbG8=",
		},
	}

	encoded, err := json.Marshal(invocation)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	require.NotContains(t, payload, "result")
	require.Contains(t, payload, "rawResponse")
}

func TestPluginInvocation_ValidateRawResponse(t *testing.T) {
	valid := validPluginInvocationForTest()
	valid.Result = nil
	valid.RawResponse = &PluginRawResponse{
		Status:      http.StatusOK,
		ContentType: "application/json",
		Body: PluginRawBody{
			Encoding: PluginRawBodyEncodingJSON,
			Value:    map[string]any{"ok": true},
		},
	}
	require.NoError(t, valid.Validate())

	tests := []struct {
		name   string
		mutate func(*PluginInvocation)
	}{
		{
			name: "rejects result alongside raw response",
			mutate: func(invocation *PluginInvocation) {
				invocation.Result = map[string]any{"unexpected": true}
				invocation.RawResponse = valid.RawResponse
			},
		},
		{
			name: "rejects invalid status",
			mutate: func(invocation *PluginInvocation) {
				invocation.RawResponse = &PluginRawResponse{Status: 99, ContentType: "application/json", Body: PluginRawBody{Encoding: PluginRawBodyEncodingJSON, Value: true}}
			},
		},
		{
			name: "rejects invalid encoding",
			mutate: func(invocation *PluginInvocation) {
				invocation.RawResponse = &PluginRawResponse{Status: http.StatusOK, ContentType: "application/json", Body: PluginRawBody{Encoding: "xml", Value: "body"}}
			},
		},
		{
			name: "rejects malformed base64",
			mutate: func(invocation *PluginInvocation) {
				invocation.RawResponse = &PluginRawResponse{Status: http.StatusOK, ContentType: "application/octet-stream", Body: PluginRawBody{Encoding: PluginRawBodyEncodingBase64, Value: "not base64"}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation := validPluginInvocationForTest()
			invocation.Result = nil
			test.mutate(&invocation)
			require.Error(t, invocation.Validate())
		})
	}
}

func TestValidatePluginEndpoint(t *testing.T) {
	valid := []string{
		"http://localhost:8080",
		"https://plugins.example.test/v1/process",
	}
	for _, endpoint := range valid {
		t.Run(endpoint, func(t *testing.T) {
			u, err := url.Parse(endpoint)
			require.NoError(t, err)
			require.NoError(t, validatePluginEndpoint(u))
		})
	}
}
