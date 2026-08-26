package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

type reconcileFailureBackend struct {
	scans int
}

func (b *reconcileFailureBackend) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("cache miss")
}

func (b *reconcileFailureBackend) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (b *reconcileFailureBackend) Delete(context.Context, ...string) (int, error) {
	return 0, nil
}

func (b *reconcileFailureBackend) Scan(context.Context, string) ([]string, error) {
	b.scans++
	if b.scans > 1 {
		return nil, errors.New("cache unavailable")
	}
	return nil, nil
}

func (b *reconcileFailureBackend) FlushAll(context.Context) (int, error) {
	return 0, nil
}

func (b *reconcileFailureBackend) Stats(context.Context) (cache.BackendStats, error) {
	return cache.BackendStats{}, nil
}

func newPluginAPITestServer(t *testing.T, cacheManager *cache.Manager) *Server {
	t.Helper()
	s := NewServer(nil, cacheManager, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: pluginTestToolName, Version: testVersion100, IsActive: true},
		Spec:     ToolSpec{Description: Description{Short: "list orders"}},
	}
	require.NoError(t, s.store.Save(tool))
	s.Reconcile(context.Background())
	return s
}

func pluginAPIRequest(t *testing.T, s *Server, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		payload = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, target, payload)
	w := httptest.NewRecorder()
	if target == "/apis/erpbridge.io/v1/plugins" || len(target) >= len("/apis/erpbridge.io/v1/plugins?") && target[:len("/apis/erpbridge.io/v1/plugins?")] == "/apis/erpbridge.io/v1/plugins?" {
		s.handlePluginAPI(w, req)
	} else {
		s.handlePluginBindingAPI(w, req)
	}
	return w
}

func TestPluginAPI_ApplyListFilterAndLifecycle(t *testing.T) {
	s := newPluginAPITestServer(t, nil)
	plugin := validPluginForTest("http://plugin.example.test")

	response := pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/plugins", plugin)
	require.Equal(t, http.StatusCreated, response.Code)
	storedPlugin, err := s.store.GetPlugin(plugin.Metadata.Name, plugin.Metadata.Version)
	require.NoError(t, err)
	require.Equal(t, PluginTypeAPI, storedPlugin.Metadata.Type)

	response = pluginAPIRequest(t, s, http.MethodGet, "/apis/erpbridge.io/v1/plugins?name=response-transformer&version=1.0.0", nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "response-transformer")

	binding := validPluginBindingForTest()
	response = pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusCreated, response.Code)
	require.Len(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100), 1)

	response = pluginAPIRequest(t, s, http.MethodGet, "/apis/erpbridge.io/v1/pluginbindings?toolName="+pluginTestToolName+"&toolVersion=1.0.0", nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "transform-orders")

	response = pluginAPIRequest(t, s, http.MethodDelete, "/apis/erpbridge.io/v1/pluginbindings?name=transform-orders", nil)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Empty(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100))

	response = pluginAPIRequest(t, s, http.MethodDelete, "/apis/erpbridge.io/v1/plugins?name=response-transformer&version=1.0.0", nil)
	require.Equal(t, http.StatusNoContent, response.Code)
	storedPlugin, err = s.store.GetPlugin(plugin.Metadata.Name, plugin.Metadata.Version)
	require.NoError(t, err)
	require.False(t, storedPlugin.Metadata.IsActive)
}

func TestPluginAPI_RejectsUnknownPluginFields(t *testing.T) {
	s := newPluginAPITestServer(t, nil)
	body := []byte(`{
		"apiVersion":"erpbridge.io/v1",
		"kind":"Plugin",
		"metadata":{"name":"strict-plugin","version":"1.0.0"},
		"spec":{"endpoint":"https://plugin.example.test","timeoutMilliseconds":1000,"token":"raw-secret"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/plugins", bytes.NewReader(body))
	response := httptest.NewRecorder()
	s.handlePluginAPI(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "unknown field")
}

func TestPluginBindingAPI_RejectsUnknownFields(t *testing.T) {
	s := newPluginAPITestServer(t, nil)
	body := []byte(`{
		"apiVersion":"erpbridge.io/v1",
		"kind":"PluginBinding",
		"metadata":{"name":"strict-binding"},
		"spec":{"pluginRef":{"name":"response-transformer","version":"1.0.0"},"toolRef":{"name":"list-orders","version":"1.0.0"},"phase":"after_response","token":"raw-secret"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", bytes.NewReader(body))
	response := httptest.NewRecorder()
	s.handlePluginBindingAPI(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "unknown field")
}

func TestPluginAPI_RejectsUnknownPluginAuthFields(t *testing.T) {
	s := newPluginAPITestServer(t, nil)
	body := []byte(`{
		"apiVersion":"erpbridge.io/v1",
		"kind":"Plugin",
		"metadata":{"name":"strict-auth-plugin","version":"1.0.0"},
		"spec":{"endpoint":"https://plugin.example.test","timeoutMilliseconds":1000,"auth":{"type":"bearer","credentialRef":"PLUGIN_TEST_TOKEN","token":"raw-secret"}}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/plugins", bytes.NewReader(body))
	response := httptest.NewRecorder()
	s.handlePluginAPI(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "unknown field")
}

func TestPluginAPI_RequiresProtectedAdmissionForCredentialedPlugin(t *testing.T) {
	plugin := validPluginForTest("https://plugin.example.test")
	plugin.Spec.Auth = &PluginAuth{Type: PluginAuthTypeBearer, CredentialRef: pluginTestCredentialRef}

	t.Run("rejects open control plane", func(t *testing.T) {
		t.Setenv(authTokenEnv, "")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:443")
		s := newPluginAPITestServer(t, nil)
		response := pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/plugins", plugin)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	})

	t.Run("rejects missing endpoint allowlist", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "")
		s := newPluginAPITestServer(t, nil)
		response := applyCredentialedPlugin(t, s, plugin, true)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	})

	t.Run("rejects a non-admin caller", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:443")
		s := newPluginAPITestServer(t, nil)
		response := applyCredentialedPlugin(t, s, plugin, false)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	})

	t.Run("accepts authenticated admin and allowed endpoint", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:443")
		s := newPluginAPITestServer(t, nil)
		response := applyCredentialedPlugin(t, s, plugin, true)
		require.Equal(t, http.StatusCreated, response.Code)
	})
}

func applyCredentialedPlugin(t *testing.T, s *Server, plugin Plugin, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(plugin)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/plugins", bytes.NewReader(body))
	req = req.WithContext(WithCallerIdentity(req.Context(), CallerIdentity{IsAdmin: admin}))
	response := httptest.NewRecorder()
	s.handlePluginAPI(response, req)
	return response
}

func TestPluginAPI_RejectsInvalidAndMissingReferences(t *testing.T) {
	s := newPluginAPITestServer(t, nil)

	malformed := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/plugins", bytes.NewBufferString("{bad"))
	malformedRecorder := httptest.NewRecorder()
	s.handlePluginAPI(malformedRecorder, malformed)
	require.Equal(t, http.StatusBadRequest, malformedRecorder.Code)

	invalidPlugin := validPluginForTest("ftp://plugin.example.test")
	response := pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/plugins", invalidPlugin)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)

	binding := validPluginBindingForTest()
	response = pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)

	plugin := validPluginForTest("http://plugin.example.test")
	require.Equal(t, http.StatusCreated, pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/plugins", plugin).Code)
	require.NoError(t, s.store.DeletePlugin(plugin.Metadata.Name, plugin.Metadata.Version))
	binding = validPluginBindingForTest()
	response = pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)

	plugin.Metadata.IsActive = true
	require.NoError(t, s.store.SavePlugin(&plugin))
	binding.Spec.ToolRef.Name = "missing-tool"
	response = pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
}

func TestPluginRegistry_ReconcileRefreshesDirectStoreChanges(t *testing.T) {
	s := newPluginAPITestServer(t, nil)
	plugin := validPluginForTest("http://plugin.example.test")
	binding := validPluginBindingForTest()
	require.NoError(t, s.store.SavePlugin(&plugin))
	require.NoError(t, s.store.SavePluginBinding(&binding))

	s.Reconcile(context.Background())
	require.Len(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100), 1)
	runtimeBindings := s.pluginRegistry.RuntimeBindingsForTool(pluginTestToolName, testVersion100)
	require.Len(t, runtimeBindings, 1)
	require.Equal(t, plugin.Spec.Endpoint, runtimeBindings[0].Plugin.Spec.Endpoint)
	runtimeBindings[0].Plugin.Spec.Endpoint = "http://mutated.example.test"
	require.Equal(t, plugin.Spec.Endpoint, s.pluginRegistry.RuntimeBindingsForTool(pluginTestToolName, testVersion100)[0].Plugin.Spec.Endpoint)

	require.NoError(t, s.store.DeletePlugin(plugin.Metadata.Name, plugin.Metadata.Version))
	s.Reconcile(context.Background())
	require.Empty(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100))

	plugin.Metadata.IsActive = true
	require.NoError(t, s.store.SavePlugin(&plugin))
	s.Reconcile(context.Background())
	require.Len(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100), 1)
}

func TestPluginAPI_ReconciliationFailureReturnsPending(t *testing.T) {
	cacheManager := cache.NewManagerWithBackend(&reconcileFailureBackend{}, logger.Init())
	s := newPluginAPITestServer(t, cacheManager)
	plugin := validPluginForTest("http://plugin.example.test")
	require.Equal(t, http.StatusCreated, pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/plugins", plugin).Code)

	binding := validPluginBindingForTest()
	response := pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusAccepted, response.Code)
	require.Contains(t, response.Body.String(), `"status":"pending"`)
	stored, err := s.store.GetPluginBinding(binding.Metadata.Name)
	require.NoError(t, err)
	require.True(t, stored.Metadata.IsActive)
	require.Empty(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100))
}

func TestPluginBindingAPI_RawAdmissionRequiresSecureHTTPContract(t *testing.T) {
	newServer := func(t *testing.T) *Server {
		t.Helper()
		s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
		schema := any(map[string]any{"type": schemaTypeObject, "properties": map[string]any{"text": map[string]any{"type": schemaTypeString}}})
		tool := &Tool{
			Metadata: Metadata{Name: pluginTestToolName, Version: testVersion100, IsActive: true},
			Spec:     ToolSpec{Execution: Execution{Type: "http", Method: http.MethodGet, Endpoint: "/invoice"}, OutputSchema: &schema},
		}
		require.NoError(t, s.store.Save(tool))
		plugin := validPluginForTest("http://plugin.example.test")
		require.NoError(t, s.store.SavePlugin(&plugin))
		s.Reconcile(context.Background())
		return s
	}
	newBinding := func() PluginBinding {
		binding := validPluginBindingForTest()
		binding.Spec.Phase = PluginPhaseRawResponse
		return binding
	}
	apply := func(t *testing.T, s *Server, binding PluginBinding, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(binding)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", bytes.NewReader(body))
		req = req.WithContext(WithCallerIdentity(req.Context(), CallerIdentity{IsAdmin: admin}))
		recorder := httptest.NewRecorder()
		s.handlePluginBindingAPI(recorder, req)
		return recorder
	}

	t.Run("requires configured API token", func(t *testing.T) {
		t.Setenv(authTokenEnv, "")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:80")
		s := newServer(t)
		require.Equal(t, http.StatusUnprocessableEntity, apply(t, s, newBinding(), true).Code)
	})
	t.Run("requires authenticated admin", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:80")
		s := newServer(t)
		require.Equal(t, http.StatusUnprocessableEntity, apply(t, s, newBinding(), false).Code)
	})
	t.Run("requires endpoint allowlist", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "")
		s := newServer(t)
		require.Equal(t, http.StatusUnprocessableEntity, apply(t, s, newBinding(), true).Code)
	})
	t.Run("accepts authenticated allowlisted raw binding", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:80")
		s := newServer(t)
		response := apply(t, s, newBinding(), true)
		require.Equal(t, http.StatusCreated, response.Code)
		require.Len(t, s.pluginRegistry.RuntimeBindingsForToolPhase(pluginTestToolName, testVersion100, PluginPhaseRawResponse), 1)
	})
	t.Run("rejects non-http tool", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:80")
		s := newServer(t)
		tool, err := s.store.Get(pluginTestToolName, testVersion100)
		require.NoError(t, err)
		tool.Spec.Execution.Type = "native"
		require.NoError(t, s.store.Save(tool))
		require.Equal(t, http.StatusUnprocessableEntity, apply(t, s, newBinding(), true).Code)
	})
	t.Run("rejects missing output schema", func(t *testing.T) {
		t.Setenv(authTokenEnv, "admin-token")
		t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:80")
		s := newServer(t)
		tool, err := s.store.Get(pluginTestToolName, testVersion100)
		require.NoError(t, err)
		tool.Spec.OutputSchema = nil
		require.NoError(t, s.store.Save(tool))
		require.Equal(t, http.StatusUnprocessableEntity, apply(t, s, newBinding(), true).Code)
	})
}

func TestPluginAPI_HardDeleteConflictAndStoreErrors(t *testing.T) {
	s := newPluginAPITestServer(t, nil)
	plugin := validPluginForTest("http://plugin.example.test")
	require.Equal(t, http.StatusCreated, pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/plugins", plugin).Code)
	binding := validPluginBindingForTest()
	require.Equal(t, http.StatusCreated, pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding).Code)

	response := pluginAPIRequest(t, s, http.MethodDelete, "/apis/erpbridge.io/v1/plugins?name=response-transformer&version=1.0.0&hard=true", nil)
	require.Equal(t, http.StatusConflict, response.Code)

	require.NoError(t, s.store.Close())
	response = pluginAPIRequest(t, s, http.MethodGet, "/apis/erpbridge.io/v1/plugins", nil)
	require.Equal(t, http.StatusInternalServerError, response.Code)
}

func TestPluginAPI_AdminOnlyRoute(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "admin-token")
	s := newPluginAPITestServer(t, nil)
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "http://localhost:8080")
	plugin := validPluginForTest("http://plugin.example.test")
	body, err := json.Marshal(plugin)
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/plugins", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	request = httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/plugins", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin-token")
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusCreated, recorder.Code)

	binding := validPluginBindingForTest()
	bindingBody, err := json.Marshal(binding)
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", bytes.NewReader(bindingBody))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	request = httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", bytes.NewReader(bindingBody))
	request.Header.Set("Authorization", "Bearer admin-token")
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusCreated, recorder.Code)
}

func TestPluginAPI_ReconcileAndFlushAffectedToolCache(t *testing.T) {
	cacheManager := cache.NewMemoryManager(10, logger.Init())
	s := newPluginAPITestServer(t, cacheManager)
	plugin := validPluginForTest("http://plugin.example.test")
	require.NoError(t, s.store.SavePlugin(&plugin))
	s.Reconcile(context.Background())

	toolCache := cache.Config{Enabled: true, TTLSeconds: 60}
	seedCache := func() {
		require.NoError(t, cacheManager.Set(context.Background(), pluginTestToolName, "", nil, []byte(`{"result":true}`), toolCache))
	}
	assertCacheFlushed := func() {
		stats, err := cacheManager.Stats(context.Background())
		require.NoError(t, err)
		require.Equal(t, int64(0), stats.ExactKeys)
	}

	seedCache()
	stats, err := cacheManager.Stats(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.ExactKeys)

	binding := validPluginBindingForTest()
	response := pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusCreated, response.Code)
	require.Len(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100), 1)
	assertCacheFlushed()

	seedCache()
	binding.Spec.Config[pluginTestModeKey] = "strict"
	response = pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusCreated, response.Code)
	assertCacheFlushed()

	seedCache()
	plugin.Spec.Endpoint = "http://plugin-updated.example.test"
	response = pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/plugins", plugin)
	require.Equal(t, http.StatusCreated, response.Code)
	assertCacheFlushed()

	seedCache()
	response = pluginAPIRequest(t, s, http.MethodDelete, "/apis/erpbridge.io/v1/pluginbindings?name=transform-orders", nil)
	require.Equal(t, http.StatusNoContent, response.Code)
	assertCacheFlushed()

	binding.Spec.Config[pluginTestModeKey] = pluginTestMode
	response = pluginAPIRequest(t, s, http.MethodPost, "/apis/erpbridge.io/v1/pluginbindings", binding)
	require.Equal(t, http.StatusCreated, response.Code)
	seedCache()
	response = pluginAPIRequest(t, s, http.MethodDelete, "/apis/erpbridge.io/v1/plugins?name=response-transformer&version=1.0.0", nil)
	require.Equal(t, http.StatusNoContent, response.Code)
	assertCacheFlushed()
	require.Empty(t, s.pluginRegistry.BindingsForTool(pluginTestToolName, testVersion100))
}
