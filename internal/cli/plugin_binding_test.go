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

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestDecodePluginBindingDocuments_AcceptsJSON(t *testing.T) {
	data, err := json.Marshal([]mcp.PluginBinding{testCLIPluginBinding()})
	require.NoError(t, err)
	bindings, err := decodePluginBindingDocuments(data, "bindings.json")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, testPluginBindingName, bindings[0].Metadata.Name)
}

func TestDecodePluginBindingDocuments_AcceptsYAMLSequence(t *testing.T) {
	data := []byte(`- apiVersion: erpbridge.io/v1
  kind: PluginBinding
  metadata:
    name: first
  spec:
    pluginRef:
      name: response-transformer
      version: 1.0.0
    toolRef:
      name: list-orders
      version: 1.0.0
    phase: after_response
---
apiVersion: erpbridge.io/v1
kind: PluginBinding
metadata:
  name: second
spec:
  pluginRef:
    name: response-transformer
    version: 1.0.0
  toolRef:
    name: list-orders
    version: 1.0.0
  phase: after_response
`)
	bindings, err := decodePluginBindingDocuments(data, "bindings.yaml")
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.Equal(t, "first", bindings[0].Metadata.Name)
	require.Equal(t, "second", bindings[1].Metadata.Name)
}

func TestPluginBindingApplyCmd_SendsResourceAndAuth(t *testing.T) {
	var received http.Request
	var receivedBinding mcp.PluginBinding
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = *r
		require.NoError(t, json.NewDecoder(r.Body).Decode(&receivedBinding))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	tokenOverride = "binding-token"
	t.Cleanup(func() { tokenOverride = "" })
	path := filepath.Join(t.TempDir(), "binding.json")
	data, err := json.Marshal(testCLIPluginBinding())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))

	var output bytes.Buffer
	pluginBindingApplyCmd.SetOut(&output)
	pluginBindingApplyCmd.SetContext(context.Background())
	require.NoError(t, pluginBindingApplyCmd.Flags().Set("file", path))
	require.NoError(t, pluginBindingApplyCmd.RunE(pluginBindingApplyCmd, nil))
	require.Equal(t, http.MethodPost, received.Method)
	require.Equal(t, "/apis/erpbridge.io/v1/pluginbindings", received.URL.Path)
	require.Equal(t, "Bearer binding-token", received.Header.Get("Authorization"))
	require.Equal(t, testPluginBindingName, receivedBinding.Metadata.Name)
	require.Contains(t, output.String(), "applied successfully")
}

func TestPluginBindingApplyCmd_RecursesDirectories(t *testing.T) {
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
	first, err := json.Marshal(testCLIPluginBinding())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(firstPath, first, 0600))
	require.NoError(t, os.WriteFile(secondPath, []byte(`apiVersion: erpbridge.io/v1
kind: PluginBinding
metadata:
  name: nested-binding
spec:
  pluginRef:
    name: response-transformer
    version: 1.0.0
  toolRef:
    name: list-orders
    version: 1.0.0
  phase: after_response
`), 0600))
	require.NoError(t, pluginBindingApplyCmd.Flags().Set("file", directory))
	require.NoError(t, pluginBindingApplyCmd.RunE(pluginBindingApplyCmd, nil))
	require.Equal(t, 2, applied)
}

func TestPluginBindingGetCmd_RendersJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testPluginBindingName, r.URL.Query().Get("name"))
		binding := testCLIPluginBinding()
		_ = json.NewEncoder(w).Encode([]*mcp.PluginBinding{&binding})
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	var output bytes.Buffer
	pluginBindingGetCmd.SetOut(&output)
	pluginBindingGetCmd.SetContext(context.Background())
	require.NoError(t, pluginBindingGetCmd.Flags().Set("output", "json"))
	require.NoError(t, pluginBindingGetCmd.RunE(pluginBindingGetCmd, []string{testPluginBindingName}))
	require.Contains(t, output.String(), testPluginBindingName)
	output.Reset()
	require.NoError(t, pluginBindingGetCmd.Flags().Set("output", "yaml"))
	require.NoError(t, pluginBindingGetCmd.RunE(pluginBindingGetCmd, []string{testPluginBindingName}))
	require.Contains(t, output.String(), "pluginRef:")
}

func TestPluginBindingGetCmd_RendersTableListAndPropagatesGetError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get(cliNameField) != "" {
			http.Error(w, "lookup failed", http.StatusBadGateway)
			return
		}
		binding := testCLIPluginBinding()
		_ = json.NewEncoder(w).Encode([]*mcp.PluginBinding{&binding})
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	var output bytes.Buffer
	pluginBindingGetCmd.SetOut(&output)
	pluginBindingGetCmd.SetContext(context.Background())
	require.NoError(t, pluginBindingGetCmd.Flags().Set("output", "table"))
	require.NoError(t, pluginBindingGetCmd.RunE(pluginBindingGetCmd, nil))
	require.Contains(t, output.String(), "NAME")
	err := pluginBindingGetCmd.RunE(pluginBindingGetCmd, []string{testPluginBindingName})
	require.Error(t, err)
	require.Contains(t, err.Error(), "502")
	require.Equal(t, 2, requests)
}

func TestPluginBindingDeleteCmd_ConfirmationCanAbort(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	pluginBindingDeleteCmd.SetIn(bytes.NewBufferString("n\n"))
	require.NoError(t, pluginBindingDeleteCmd.Flags().Set("hard", "true"))
	require.NoError(t, pluginBindingDeleteCmd.Flags().Set("yes", "false"))
	require.NoError(t, pluginBindingDeleteCmd.RunE(pluginBindingDeleteCmd, []string{testPluginBindingName}))
	require.Zero(t, requests)
}

func TestPluginBindingApplyCmd_PropagatesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid binding", http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	path := filepath.Join(t.TempDir(), "binding.json")
	data, err := json.Marshal(testCLIPluginBinding())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	require.NoError(t, pluginBindingApplyCmd.Flags().Set("file", path))
	err = pluginBindingApplyCmd.RunE(pluginBindingApplyCmd, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "422")
}

func TestPluginBindingDeleteCmd_HardDeleteWithYes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testPluginBindingName, r.URL.Query().Get("name"))
		require.Equal(t, "true", r.URL.Query().Get("hard"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{testContextName: {MCPServer: server.URL}}}
	pluginBindingDeleteCmd.SetContext(context.Background())
	require.NoError(t, pluginBindingDeleteCmd.Flags().Set("hard", "true"))
	require.NoError(t, pluginBindingDeleteCmd.Flags().Set("yes", "true"))
	require.NoError(t, pluginBindingDeleteCmd.RunE(pluginBindingDeleteCmd, []string{testPluginBindingName}))
}

func TestPluginBindingValidateCmd_RejectsMissingReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-binding.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"metadata":{"name":"bad"}}`), 0600))
	require.NoError(t, pluginBindingValidateCmd.Flags().Set("file", path))
	err := pluginBindingValidateCmd.RunE(pluginBindingValidateCmd, nil)
	require.Error(t, err)
}
