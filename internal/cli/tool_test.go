package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/stretchr/testify/require"
)

func TestToolListResponse_RenderTable(t *testing.T) {
	resp := &ToolListResponse{
		Tools: []*mcp.Tool{
			{
				Metadata: mcp.Metadata{
					Name:    testToolName,
					Module:  "hr",
					Version: testToolVersion,
					Status:  testStatusActive,
				},
			},
		},
	}
	var buf bytes.Buffer
	err := resp.RenderTable(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte(testToolName)) {
		t.Errorf("expected output to contain 'tool1'")
	}
}

func TestToolGetCmd(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"metadata":{"name":"tool1","version":"1.0"}}]`))
	}))
	defer ts.Close()

	cfg = &config.Config{
		CurrentContext: testContextName,
		Contexts: map[string]config.Context{
			testContextName: {MCPServer: ts.URL},
		},
	}
	var buf bytes.Buffer
	formatter = &output.Formatter{Format: output.FormatJSON, Out: &buf}

	err := toolGetCmd.RunE(toolGetCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(testToolName)) {
		t.Errorf("expected output to contain tool1")
	}
}

func TestToolDeleteCmd_Errors(t *testing.T) {
	err := toolDeleteCmd.Args(toolDeleteCmd, []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing required arguments")

	err = toolDeleteCmd.Args(toolDeleteCmd, []string{testToolName})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing required arguments")
}

func TestToolDeleteCmd(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	cfg = &config.Config{
		CurrentContext: testContextName,
		Contexts: map[string]config.Context{
			testContextName: {MCPServer: ts.URL},
		},
	}

	t.Run("SoftDelete", func(t *testing.T) {
		toolDeleteCmd.SetContext(context.Background())
		_ = toolDeleteCmd.Flags().Set("hard", "false")
		err := toolDeleteCmd.RunE(toolDeleteCmd, []string{testToolName, testToolVersion})
		require.NoError(t, err)
	})

	t.Run("HardDeleteWithYes", func(t *testing.T) {
		toolDeleteCmd.SetContext(context.Background())
		_ = toolDeleteCmd.Flags().Set("hard", "true")
		_ = toolDeleteCmd.Flags().Set("yes", "true")
		err := toolDeleteCmd.RunE(toolDeleteCmd, []string{testToolName, testToolVersion})
		require.NoError(t, err)
	})
}

func TestToolApplyGeneratedYAMLFileExactlyOnce(t *testing.T) {
	var requests atomic.Int32
	var applied mcp.Tool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&applied))
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	cfg = &config.Config{
		CurrentContext: testContextName,
		Contexts:       map[string]config.Context{testContextName: {MCPServer: ts.URL}},
	}
	manifest := filepath.Join(t.TempDir(), "draft.yaml")
	contents := `apiVersion: erpbridge.io/v1
kind: MCPTool
metadata:
  name: list_orders
  version: 1.0.0
spec:
  execution:
    type: http
    method: GET
    endpoint: /orders
`
	require.NoError(t, os.WriteFile(manifest, []byte(contents), 0600))
	require.NoError(t, toolApplyCmd.Flags().Set("file", manifest))
	var output bytes.Buffer
	toolApplyCmd.SetOut(&output)
	toolApplyCmd.SetErr(&output)
	defer toolApplyCmd.SetOut(nil)

	require.NoError(t, toolApplyCmd.RunE(toolApplyCmd, nil))
	require.Equal(t, int32(1), requests.Load())
	require.Equal(t, "list_orders", applied.Metadata.Name)
	require.Contains(t, output.String(), "list_orders@1.0.0 applied successfully")
}

func TestToolValidateCmd(t *testing.T) {
	content := `{"metadata":{"name":"t","version":"1"}}`
	err := os.WriteFile("test_tool.json", []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove("test_tool.json") }()

	require.NoError(t, toolValidateCmd.Flags().Set("file", "test_tool.json"))
	err = toolValidateCmd.RunE(toolValidateCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolValidateCmd_ParseErrorsAreVisible(t *testing.T) {
	path := "invalid_tool.json"
	require.NoError(t, os.WriteFile(path, []byte(`{"metadata":`), 0600))
	defer func() { _ = os.Remove(path) }()

	require.NoError(t, toolValidateCmd.Flags().Set("file", path))
	err := toolValidateCmd.RunE(toolValidateCmd, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse JSON")
}

func TestDecodeToolDocuments_AcceptsGeneratedYAMLSequenceAndDocuments(t *testing.T) {
	data := []byte(`- apiVersion: erpbridge.io/v1
  kind: MCPTool
  metadata:
    name: first
    version: 1.0.0
---
apiVersion: erpbridge.io/v1
kind: MCPTool
metadata:
  name: second
  version: 1.0.0
`)

	tools, err := decodeToolDocuments(data, "tools.yaml")
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "first", tools[0].Metadata.Name)
	require.Equal(t, "second", tools[1].Metadata.Name)
}
