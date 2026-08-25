package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/stretchr/testify/require"
)

const (
	testPluginName        = "response-transformer"
	testPluginVersion     = "1.0.0"
	testPluginBindingName = "transform-orders"
)

func testCLIPlugin() mcp.Plugin {
	return mcp.Plugin{
		APIVersion: mcp.PluginAPIVersion,
		Kind:       mcp.PluginKind,
		Metadata: mcp.PluginMetadata{
			Name:     testPluginName,
			Version:  testPluginVersion,
			IsActive: true,
		},
		Spec: mcp.PluginSpec{Endpoint: "http://plugin.example.test", TimeoutMilliseconds: 1000},
	}
}

func testCLIPluginBinding() mcp.PluginBinding {
	return mcp.PluginBinding{
		APIVersion: mcp.PluginAPIVersion,
		Kind:       mcp.PluginBindingKind,
		Metadata:   mcp.PluginBindingMetadata{Name: testPluginBindingName, IsActive: true},
		Spec: mcp.PluginBindingSpec{
			PluginRef:     mcp.PluginRef{Name: testPluginName, Version: testPluginVersion},
			ToolRef:       mcp.ToolRef{Name: "list-orders", Version: testPluginVersion},
			Phase:         mcp.PluginPhaseAfterResponse,
			Priority:      10,
			FailurePolicy: mcp.PluginFailurePolicyContinue,
		},
	}
}

func TestDecodePluginDocuments_AcceptsJSONAndYAMLStreams(t *testing.T) {
	jsonData, err := json.Marshal([]mcp.Plugin{testCLIPlugin()})
	require.NoError(t, err)
	plugins, err := decodePluginDocuments(jsonData, "plugins.json")
	require.NoError(t, err)
	require.Len(t, plugins, 1)

	yamlData := []byte(`- apiVersion: erpbridge.io/v1
  kind: Plugin
  metadata:
    name: first
    version: 1.0.0
  spec:
    endpoint: http://first.example.test
    timeoutMilliseconds: 1000
---
apiVersion: erpbridge.io/v1
kind: Plugin
metadata:
  name: second
  version: 1.0.0
spec:
  endpoint: http://second.example.test
  timeoutMilliseconds: 1000
`)
	plugins, err = decodePluginDocuments(yamlData, "plugins.yaml")
	require.NoError(t, err)
	require.Len(t, plugins, 2)
	require.Equal(t, "first", plugins[0].Metadata.Name)
	require.Equal(t, "second", plugins[1].Metadata.Name)
}

func TestPluginApplyCmd_SendsAuthenticatedResource(t *testing.T) {
	var received http.Request
	var receivedPlugin mcp.Plugin
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = *r
		require.NoError(t, json.NewDecoder(r.Body).Decode(&receivedPlugin))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	tokenOverride = "cli-token"
	t.Cleanup(func() { tokenOverride = "" })
	path := filepath.Join(t.TempDir(), "plugin.json")
	data, err := json.Marshal(testCLIPlugin())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))

	var output bytes.Buffer
	pluginApplyCmd.SetOut(&output)
	pluginApplyCmd.SetContext(context.Background())
	require.NoError(t, pluginApplyCmd.Flags().Set("file", path))
	require.NoError(t, pluginApplyCmd.RunE(pluginApplyCmd, nil))
	require.Equal(t, http.MethodPost, received.Method)
	require.Equal(t, "/apis/erpbridge.io/v1/plugins", received.URL.Path)
	require.Equal(t, "Bearer cli-token", received.Header.Get("Authorization"))
	require.Equal(t, testPluginName, receivedPlugin.Metadata.Name)
	require.Contains(t, output.String(), "applied successfully")
}

func TestPluginApplyCmd_PropagatesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid plugin", http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	path := filepath.Join(t.TempDir(), "plugin.json")
	data, err := json.Marshal(testCLIPlugin())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	require.NoError(t, pluginApplyCmd.Flags().Set("file", path))
	err = pluginApplyCmd.RunE(pluginApplyCmd, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "422")
}

func TestPluginGetCmd_RequiresExactVersionIdentity(t *testing.T) {
	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: "http://server.example.test"}}}
	err := pluginGetCmd.RunE(pluginGetCmd, []string{"response-transformer"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "name@version")
}

func TestPluginApplyCmd_RecursesDirectories(t *testing.T) {
	applied := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		applied++
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	directory := t.TempDir()
	firstPath := filepath.Join(directory, "first.json")
	secondPath := filepath.Join(directory, "nested", "second.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(secondPath), 0750))
	first, err := json.Marshal(testCLIPlugin())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(firstPath, first, 0600))
	require.NoError(t, os.WriteFile(secondPath, []byte(`apiVersion: erpbridge.io/v1
kind: Plugin
metadata:
  name: nested
  version: 1.0.0
spec:
  endpoint: http://nested.example.test
  timeoutMilliseconds: 1000
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "README.txt"), []byte("ignored"), 0600))
	require.NoError(t, pluginApplyCmd.Flags().Set("file", directory))
	require.NoError(t, pluginApplyCmd.RunE(pluginApplyCmd, nil))
	require.Equal(t, 2, applied)
}

func TestPluginGetCmd_RendersRequestedFormatAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, testPluginName, r.URL.Query().Get("name"))
		require.Equal(t, testPluginVersion, r.URL.Query().Get("version"))
		plugin := testCLIPlugin()
		_ = json.NewEncoder(w).Encode([]*mcp.Plugin{&plugin})
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	var output bytes.Buffer
	pluginGetCmd.SetOut(&output)
	pluginGetCmd.SetContext(context.Background())
	require.NoError(t, pluginGetCmd.Flags().Set("output", "json"))
	require.NoError(t, pluginGetCmd.RunE(pluginGetCmd, []string{testPluginName + "@" + testPluginVersion}))
	require.Contains(t, output.String(), testPluginName)

	output.Reset()
	require.NoError(t, pluginGetCmd.Flags().Set("output", "yaml"))
	require.NoError(t, pluginGetCmd.RunE(pluginGetCmd, []string{testPluginName + "@" + testPluginVersion}))
	require.Contains(t, output.String(), "endpoint:")
}

func TestPluginGetCmd_RendersTableListAndPropagatesGetError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get(cliNameField) != "" {
			http.Error(w, "lookup failed", http.StatusBadGateway)
			return
		}
		plugin := testCLIPlugin()
		_ = json.NewEncoder(w).Encode([]*mcp.Plugin{&plugin})
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	var output bytes.Buffer
	pluginGetCmd.SetOut(&output)
	pluginGetCmd.SetContext(context.Background())
	require.NoError(t, pluginGetCmd.Flags().Set("output", "table"))
	require.NoError(t, pluginGetCmd.RunE(pluginGetCmd, nil))
	require.Contains(t, output.String(), "NAME")
	err := pluginGetCmd.RunE(pluginGetCmd, []string{testPluginName + "@" + testPluginVersion})
	require.Error(t, err)
	require.Contains(t, err.Error(), "502")
	require.Equal(t, 2, requests)
}

func TestPluginApplyCmd_RejectsInvalidInputBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	path := filepath.Join(t.TempDir(), "invalid-plugin.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"metadata":{"name":"bad","version":"1.0.0"}}`), 0600))
	require.NoError(t, pluginApplyCmd.Flags().Set("file", path))
	require.Error(t, pluginApplyCmd.RunE(pluginApplyCmd, nil))
	require.Zero(t, requests)
}

func TestPluginDeleteCmd_ConfirmationCanAbort(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	pluginDeleteCmd.SetIn(bytes.NewBufferString("n\n"))
	require.NoError(t, pluginDeleteCmd.Flags().Set("hard", "true"))
	require.NoError(t, pluginDeleteCmd.Flags().Set("yes", "false"))
	require.NoError(t, pluginDeleteCmd.RunE(pluginDeleteCmd, []string{testPluginName + "@" + testPluginVersion}))
	require.Zero(t, requests)
}

func TestPluginOutputTableAndYAML(t *testing.T) {
	plugin := testCLIPlugin()
	var table bytes.Buffer
	require.NoError(t, (&PluginListResponse{Plugins: []*mcp.Plugin{&plugin}}).RenderTable(&table))
	require.Contains(t, table.String(), testPluginName)

	var yamlOutput bytes.Buffer
	require.NoError(t, yaml.NewEncoder(&yamlOutput).Encode(plugin))
	require.Contains(t, yamlOutput.String(), testPluginName)
}

func TestPluginDeleteCmd_HardDeleteWithYes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		require.Equal(t, "true", r.URL.Query().Get("hard"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	pluginDeleteCmd.SetContext(context.Background())
	require.NoError(t, pluginDeleteCmd.Flags().Set("hard", "true"))
	require.NoError(t, pluginDeleteCmd.Flags().Set("yes", "true"))
	require.NoError(t, pluginDeleteCmd.RunE(pluginDeleteCmd, []string{"response-transformer@1.0.0"}))
}

func TestPluginValidateCmd_RejectsInvalidDefinition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"metadata":{"name":"missing-version"}}`), 0600))
	require.NoError(t, pluginValidateCmd.Flags().Set("file", path))
	err := pluginValidateCmd.RunE(pluginValidateCmd, nil)
	require.Error(t, err)
}
