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

	"github.com/goccy/go-yaml"
	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
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

func TestToolGenerateCmdUsesSelectedContextRegistry(t *testing.T) {
	setupTest()
	home := t.TempDir()
	t.Setenv("HOME", home)

	reg, err := idp.NewRegistryForContext(testContextName, RootLog)
	require.NoError(t, err)
	require.NoError(t, reg.Register(&idp.API{
		Name:          "department-api",
		URL:           "http://mock-erp:8081/api/resource/Department",
		Method:        http.MethodGet,
		AuthType:      "api-key", // #nosec G101 -- test exercises an auth type, not a credential.
		AuthHeader:    "Authorization",
		CredentialRef: "ERP_ONBOARDING_CREDENTIAL",
		Module:        "erp",
		Description:   "List departments",
	}))

	spec := filepath.Join(t.TempDir(), "openapi.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(`openapi: 3.0.3
info:
  title: onboarding fixture
  version: 1.0.0
servers:
  - url: http://mock-erp:8081/api
paths:
  /resource/Department:
    get:
      operationId: list_departments
      summary: List departments
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        name:
                          type: string
`), 0600))

	oldAPI, oldOpenAPI := flagValue(t, toolGenerateCmd, "api"), flagValue(t, toolGenerateCmd, "openapi")
	t.Cleanup(func() {
		_ = toolGenerateCmd.Flags().Set("api", oldAPI)
		_ = toolGenerateCmd.Flags().Set("openapi", oldOpenAPI)
	})
	require.NoError(t, toolGenerateCmd.Flags().Set("api", "department-api"))
	require.NoError(t, toolGenerateCmd.Flags().Set("openapi", spec))
	var output bytes.Buffer
	toolGenerateCmd.SetOut(&output)
	toolGenerateCmd.SetContext(t.Context())
	t.Cleanup(func() { toolGenerateCmd.SetOut(nil) })

	require.NoError(t, toolGenerateCmd.RunE(toolGenerateCmd, nil))
	require.Contains(t, output.String(), "list_departments")
	require.Contains(t, output.String(), "ERP_ONBOARDING_CREDENTIAL")
}

func TestToolGenerateCmdWritesOneFilePerTool(t *testing.T) {
	setupTest()
	home := t.TempDir()
	t.Setenv("HOME", home)

	reg, err := idp.NewRegistryForContext(testContextName, RootLog)
	require.NoError(t, err)
	require.NoError(t, reg.Register(&idp.API{
		Name:        "orders-api",
		URL:         "http://mock-erp:8081",
		Method:      http.MethodGet,
		Module:      "erp",
		Description: "Orders API",
	}))

	spec := filepath.Join(t.TempDir(), "openapi.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(`openapi: 3.0.3
info:
  title: orders fixture
  version: 1.0.0
servers:
  - url: http://mock-erp:8081/api
paths:
  /orders:
    get:
      operationId: list_orders
      summary: List orders
      responses:
        '200':
          description: OK
    post:
      operationId: create_order
      summary: Create an order
      responses:
        '201':
          description: Created
`), 0600))

	oldAPI := flagValue(t, toolGenerateCmd, "api")
	oldOpenAPI := flagValue(t, toolGenerateCmd, "openapi")
	oldOutputDir := flagValue(t, toolGenerateCmd, "output-dir")
	oldOutputFormat := outputFormat
	t.Cleanup(func() {
		_ = toolGenerateCmd.Flags().Set("api", oldAPI)
		_ = toolGenerateCmd.Flags().Set("openapi", oldOpenAPI)
		_ = toolGenerateCmd.Flags().Set("output-dir", oldOutputDir)
		outputFormat = oldOutputFormat
		toolGenerateCmd.SetOut(nil)
		toolGenerateCmd.SetErr(nil)
	})
	require.NoError(t, toolGenerateCmd.Flags().Set("api", "orders-api"))
	require.NoError(t, toolGenerateCmd.Flags().Set("openapi", spec))
	outputFormat = "yaml"

	outputDir := filepath.Join(t.TempDir(), "manifests", "erp")
	require.NoError(t, toolGenerateCmd.Flags().Set("output-dir", outputDir))
	var stdout, stderr bytes.Buffer
	toolGenerateCmd.SetOut(&stdout)
	toolGenerateCmd.SetErr(&stderr)
	toolGenerateCmd.SetContext(context.Background())

	require.NoError(t, toolGenerateCmd.RunE(toolGenerateCmd, nil))
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "wrote 2 tool files")

	for _, name := range []string{"list_orders", "create_order"} {
		path := filepath.Join(outputDir, name+".yaml")
		// #nosec G304 -- path is created within the test's temporary directory.
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var tool mcp.Tool
		require.NoError(t, yaml.Unmarshal(data, &tool))
		require.Equal(t, name, tool.Metadata.Name)
	}

	jsonOutputDir := filepath.Join(t.TempDir(), "manifests", "erp")
	outputFormat = string(output.FormatJSON)
	require.NoError(t, toolGenerateCmd.Flags().Set("output-dir", jsonOutputDir))
	stdout.Reset()
	stderr.Reset()
	require.NoError(t, toolGenerateCmd.RunE(toolGenerateCmd, nil))
	require.Empty(t, stdout.String())
	for _, name := range []string{"list_orders", "create_order"} {
		path := filepath.Join(jsonOutputDir, name+".json")
		// #nosec G304 -- path is created within the test's temporary directory.
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var tool mcp.Tool
		require.NoError(t, json.Unmarshal(data, &tool))
		require.Equal(t, name, tool.Metadata.Name)
	}

	outputFormat = string(output.FormatJSON)
	require.NoError(t, toolGenerateCmd.Flags().Set("openapi", ""))
	require.NoError(t, toolGenerateCmd.Flags().Set("output-dir", ""))
	stdout.Reset()
	require.NoError(t, toolGenerateCmd.RunE(toolGenerateCmd, nil))
	var singleTool mcp.Tool
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &singleTool))
	require.Equal(t, "orders-api", singleTool.Metadata.Name)
}

func flagValue(t *testing.T, cmd *cobra.Command, name string) string {
	t.Helper()
	value, err := cmd.Flags().GetString(name)
	require.NoError(t, err)
	return value
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
