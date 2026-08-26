package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/security"
	"github.com/stretchr/testify/require"
)

const (
	pluginClientTestCredentialRef = "PLUGIN_CLIENT_TEST_CREDENTIAL" // #nosec G101 -- environment-variable reference used by tests.
	pluginClientTestValue         = "test-value"
)

type pluginRoundTripFunc func(*http.Request) (*http.Response, error)

func (f pluginRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func validPluginInvocationForTest() PluginInvocation {
	return PluginInvocation{
		ProtocolVersion: PluginProtocolVersion,
		InvocationID:    "invocation-123",
		Tool: ToolIdentity{
			Name:    pluginTestToolName,
			Version: pluginTestVersion,
		},
		Result: map[string]any{"id": pluginTestResultID},
		Config: map[string]any{pluginTestModeKey: pluginTestMode},
	}
}

func TestPluginClient_Process_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/process", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var request PluginInvocation
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "invocation-123", request.InvocationID)
		require.Equal(t, pluginTestToolName, request.Tool.Name)
		require.Equal(t, map[string]any{"id": pluginTestResultID}, request.Result)
		require.Equal(t, map[string]any{pluginTestModeKey: pluginTestMode}, request.Config)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"id":"` + pluginTestResultID + `","processed":true}}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	response, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.NoError(t, err)
	require.Equal(t, map[string]any{"id": pluginTestResultID, "processed": true}, response.Result)
}

func TestPluginClient_Process_SendsRawResponseWithoutLegacyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.NotContains(t, payload, "result")
		raw, ok := payload["rawResponse"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, float64(http.StatusOK), raw["status"])
		require.Equal(t, "image/png", raw["contentType"])
		require.Equal(t, "base64", raw["body"].(map[string]any)["encoding"])
		_, _ = w.Write([]byte(`{"result":{"text":"ok"}}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	invocation := validPluginInvocationForTest()
	invocation.Result = nil
	invocation.RawResponse = &PluginRawResponse{
		Status:      http.StatusOK,
		ContentType: "image/png",
		Body: PluginRawBody{
			Encoding: PluginRawBodyEncodingBase64,
			Value:    "iVBORw==",
		},
	}
	response, err := NewPluginClient().Process(context.Background(), &plugin, invocation)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"text": "ok"}, response.Result)
}

func TestPluginClient_Process_RejectsUnsupportedProtocol(t *testing.T) {
	plugin := validPluginForTest("http://plugins.example.test")
	invocation := validPluginInvocationForTest()
	invocation.ProtocolVersion = "v0"
	_, err := NewPluginClient().Process(context.Background(), &plugin, invocation)
	require.Error(t, err)
}

func TestPluginClient_Process_RequiresExactToolIdentity(t *testing.T) {
	plugin := validPluginForTest("http://plugins.example.test")
	for name, mutate := range map[string]func(*PluginInvocation){
		"missing name":    func(invocation *PluginInvocation) { invocation.Tool.Name = "" },
		"missing version": func(invocation *PluginInvocation) { invocation.Tool.Version = "" },
		"invalid version": func(invocation *PluginInvocation) { invocation.Tool.Version = "not-semver" },
	} {
		t.Run(name, func(t *testing.T) {
			invocation := validPluginInvocationForTest()
			mutate(&invocation)
			_, err := NewPluginClient().Process(context.Background(), &plugin, invocation)
			require.Error(t, err)
		})
	}
}

func TestPluginClient_Process_RejectsPayloadDataOutsideContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.NotContains(t, payload, "arguments")
		require.NotContains(t, payload, "headers")
		require.NotContains(t, payload, "credentials")
		require.NotContains(t, payload, "role")
		_, _ = w.Write([]byte(`{"result":42}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	invocation := validPluginInvocationForTest()
	response, err := NewPluginClient().Process(context.Background(), &plugin, invocation)
	require.NoError(t, err)
	require.Equal(t, float64(42), response.Result)
}

func TestPluginClient_Process_GeneratesInvocationID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload PluginInvocation
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.NotEmpty(t, payload.InvocationID)
		_, _ = w.Write([]byte(`{"result":true}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	invocation := validPluginInvocationForTest()
	invocation.InvocationID = ""
	_, err := NewPluginClient().Process(context.Background(), &plugin, invocation)
	require.NoError(t, err)
}

func TestPluginClient_Process_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"result":true}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	plugin.Spec.TimeoutMilliseconds = 10
	_, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
	require.NotContains(t, err.Error(), server.URL)
}

func TestPluginClient_Process_DisablesRedirects(t *testing.T) {
	var finalCalls int
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		finalCalls++
		_, _ = w.Write([]byte(`{"result":true}`))
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/v1/process", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	plugin := validPluginForTest(redirect.URL)
	_, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
	require.Zero(t, finalCalls)
	require.NotContains(t, err.Error(), final.URL)
}

func TestPluginClient_Process_NonSuccessDoesNotExposeBody(t *testing.T) {
	privateBody := "private-plugin-response"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(privateBody))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	_, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
	require.NotContains(t, err.Error(), privateBody)
	require.NotContains(t, err.Error(), server.URL)
}

func TestPluginClient_Process_RejectsMalformedResponse(t *testing.T) {
	body := `{"result":`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	_, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
	require.NotContains(t, err.Error(), body)
}

func TestPluginClient_Process_RejectsMissingResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	_, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
}

func TestPluginClient_Process_RejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":"` + strings.Repeat("x", maxPluginJSONBytes) + `"}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	_, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
	require.NotContains(t, err.Error(), server.URL)
}

func TestPluginClient_Process_RejectsOversizedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("oversized request must be rejected before sending")
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	invocation := validPluginInvocationForTest()
	invocation.Config = map[string]any{"data": strings.Repeat("x", maxPluginJSONBytes)}
	_, err := NewPluginClient().Process(context.Background(), &plugin, invocation)
	require.Error(t, err)
}

func TestPluginClient_Process_DoesNotRetry(t *testing.T) {
	calls := 0
	transport := pluginRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("transport failure")
	})
	plugin := validPluginForTest("http://plugins.example.test")
	client := NewPluginClient(&http.Client{Transport: transport})
	_, err := client.Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestPluginClient_Process_UsesContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Second)
		_, _ = w.Write([]byte(`{"result":true}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewPluginClient().Process(ctx, &plugin, validPluginInvocationForTest())
	require.Error(t, err)
}

func TestPluginClient_Process_RejectsNilPlugin(t *testing.T) {
	_, err := NewPluginClient().Process(context.Background(), nil, validPluginInvocationForTest())
	require.Error(t, err)
}

func TestPluginClient_Process_SendsAuthenticationHeaders(t *testing.T) {
	const credential = pluginClientTestValue

	tests := []struct {
		name              string
		auth              *PluginAuth
		expectedHeader    string
		expectedValue     string
		unexpectedHeaders []string
	}{
		{
			name:              "absent auth",
			unexpectedHeaders: []string{pluginAuthorizationHeader, pluginDefaultAPIKeyHeader},
		},
		{
			name: "bearer",
			auth: &PluginAuth{
				Type:          PluginAuthTypeBearer,
				CredentialRef: pluginClientTestCredentialRef,
			},
			expectedHeader:    pluginAuthorizationHeader,
			expectedValue:     "Bearer " + credential,
			unexpectedHeaders: []string{pluginDefaultAPIKeyHeader},
		},
		{
			name: "default api key",
			auth: &PluginAuth{
				Type:          PluginAuthTypeAPIKey,
				CredentialRef: pluginClientTestCredentialRef,
			},
			expectedHeader:    pluginDefaultAPIKeyHeader,
			expectedValue:     credential,
			unexpectedHeaders: []string{pluginAuthorizationHeader},
		},
		{
			name: "custom api key",
			auth: &PluginAuth{
				Type:          PluginAuthTypeAPIKey,
				CredentialRef: pluginClientTestCredentialRef,
				Header:        "X-Plugin-Key",
			},
			expectedHeader:    "X-Plugin-Key",
			expectedValue:     credential,
			unexpectedHeaders: []string{pluginAuthorizationHeader, pluginDefaultAPIKeyHeader},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(pluginClientTestCredentialRef, credential)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				require.Equal(t, "application/json", r.Header.Get("Accept"))
				if test.expectedHeader != "" {
					require.Equal(t, []string{test.expectedValue}, r.Header.Values(test.expectedHeader))
				}
				for _, header := range test.unexpectedHeaders {
					require.Empty(t, r.Header.Values(header))
				}
				_, _ = w.Write([]byte(`{"result":true}`))
			}))
			defer server.Close()
			allowInsecurePluginAuthHost(t, server.URL)

			plugin := validPluginForTest(server.URL)
			plugin.Spec.Auth = test.auth
			_, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
			require.NoError(t, err)
		})
	}
}

func TestPluginClient_Process_RejectsCredentialedHTTPBeforeOutboundCall(t *testing.T) {
	const credential = pluginClientTestValue
	t.Setenv(pluginClientTestCredentialRef, credential)

	calls := 0
	client := NewPluginClient(&http.Client{Transport: pluginRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("outbound call must not occur")
	})})
	plugin := validPluginForTest("http://plugins.example.test")
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginClientTestCredentialRef}

	_, err := client.Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.EqualError(t, err, "credentialed plugin endpoint is not allowed")
	require.Zero(t, calls)
}

func TestPluginClient_Process_RejectsNonmatchingCredentialedHTTPHost(t *testing.T) {
	const credential = pluginClientTestValue
	t.Setenv(pluginClientTestCredentialRef, credential)
	t.Setenv(security.InsecureAuthAllowedHostsEnv, "other.example.test:80")

	calls := 0
	client := NewPluginClient(&http.Client{Transport: pluginRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("outbound call must not occur")
	})})
	plugin := validPluginForTest("http://plugins.example.test")
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginClientTestCredentialRef}

	_, err := client.Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.EqualError(t, err, "credentialed plugin endpoint is not allowed")
	require.Zero(t, calls)
}

func TestPluginClient_Process_AllowsExactCredentialedHTTPHostWithWarning(t *testing.T) {
	const credential = pluginClientTestValue
	t.Setenv(pluginClientTestCredentialRef, credential)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+credential, r.Header.Get(pluginAuthorizationHeader))
		_, _ = w.Write([]byte(`{"result":true}`))
	}))
	defer server.Close()
	allowInsecurePluginAuthHost(t, server.URL)

	var logs bytes.Buffer
	plugin := validPluginForTest(server.URL)
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginClientTestCredentialRef}
	_, err := NewPluginClientWithLogger(slog.New(slog.NewTextHandler(&logs, nil))).Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.NoError(t, err)
	require.Contains(t, logs.String(), "credentialed outbound HTTP is allowed for development")
	require.NotContains(t, logs.String(), credential)
}

func TestPluginClient_Process_AllowsCredentialedHTTPS(t *testing.T) {
	const credential = pluginClientTestValue
	t.Setenv(pluginClientTestCredentialRef, credential)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+credential, r.Header.Get(pluginAuthorizationHeader))
		_, _ = w.Write([]byte(`{"result":true}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginClientTestCredentialRef}
	_, err := NewPluginClient(server.Client()).Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.NoError(t, err)
}

func TestPluginClient_Process_MissingCredentialMakesZeroOutboundCalls(t *testing.T) {
	const credentialRef = "PLUGIN_CLIENT_MISSING_CREDENTIAL" // #nosec G101 -- environment-variable reference used by this test.
	t.Setenv(credentialRef, "")

	calls := 0
	client := NewPluginClient(&http.Client{Transport: pluginRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("outbound call must not occur")
	})})
	plugin := validPluginForTest("https://plugins.example.test")
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: credentialRef}

	_, err := client.Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.EqualError(t, err, `plugin credential is not configured`)
	require.Zero(t, calls)
}

func TestPluginClient_Process_DoesNotExposeCredentialInErrorsOrLogs(t *testing.T) {
	const credential = pluginClientTestValue
	t.Setenv(pluginClientTestCredentialRef, credential)

	var logs bytes.Buffer
	client := NewPluginClientWithLogger(slog.New(slog.NewTextHandler(&logs, nil)), &http.Client{
		Transport: pluginRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("transport failure")
		}),
	})
	plugin := validPluginForTest("https://plugins.example.test")
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginClientTestCredentialRef}

	_, err := client.Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.EqualError(t, err, "plugin request failed")
	require.NotContains(t, err.Error(), credential)
	require.NotContains(t, logs.String(), credential)
}

func TestPluginClient_Process_RejectsInvalidAuthenticationBeforeOutboundCall(t *testing.T) {
	calls := 0
	client := NewPluginClient(&http.Client{Transport: pluginRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("outbound call must not occur")
	})})
	plugin := validPluginForTest("https://plugins.example.test")
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginClientTestCredentialRef, Header: "X-Invalid"}

	_, err := client.Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.Error(t, err)
	require.Zero(t, calls)
}

func allowInsecurePluginAuthHost(t *testing.T, rawURL string) {
	t.Helper()
	endpoint, err := url.Parse(rawURL)
	require.NoError(t, err)
	t.Setenv(security.InsecureAuthAllowedHostsEnv, endpoint.Host)
}

func TestPluginClient_Process_UsesOnlyResultEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"ok":true},"error":"ignored"}`))
	}))
	defer server.Close()

	plugin := validPluginForTest(server.URL)
	response, err := NewPluginClient().Process(context.Background(), &plugin, validPluginInvocationForTest())
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, response.Result)
}
