//go:build pluginintegration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/mcp"
)

func TestPluginSystemBlackBox(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ERPBRIDGE_TEST_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	client := &http.Client{}

	plugin := mcp.Plugin{
		APIVersion: mcp.PluginAPIVersion,
		Kind:       mcp.PluginKind,
		Metadata:   mcp.PluginMetadata{Name: "mock-plugin", Version: "0.1.0", IsActive: true},
		Spec:       mcp.PluginSpec{Endpoint: "http://mock-plugin:8080", TimeoutMilliseconds: 2000},
	}
	applyJSON(t, client, baseURL+"/apis/erpbridge.io/v1/plugins", plugin)

	boundTool := integrationTool("plugin-fixture-bound")
	ordinaryTool := integrationTool("plugin-fixture-ordinary")
	applyJSON(t, client, baseURL+"/apis/erpbridge.io/v1/tools", boundTool)
	applyJSON(t, client, baseURL+"/apis/erpbridge.io/v1/tools", ordinaryTool)

	binding := mcp.PluginBinding{
		APIVersion: mcp.PluginAPIVersion,
		Kind:       mcp.PluginBindingKind,
		Metadata:   mcp.PluginBindingMetadata{Name: "mock-plugin-fixture-binding", IsActive: true},
		Spec: mcp.PluginBindingSpec{
			PluginRef:     mcp.PluginRef{Name: plugin.Metadata.Name, Version: plugin.Metadata.Version},
			ToolRef:       mcp.ToolRef{Name: boundTool.Metadata.Name, Version: boundTool.Metadata.Version},
			Phase:         mcp.PluginPhaseAfterResponse,
			FailurePolicy: mcp.PluginFailurePolicyFail,
		},
	}
	applyJSON(t, client, baseURL+"/apis/erpbridge.io/v1/pluginbindings", binding)

	directBound := invokeDirect(t, client, baseURL, boundTool.Metadata.Name)
	directOrdinary := invokeDirect(t, client, baseURL, ordinaryTool.Metadata.Name)
	assertFixture(t, directBound, true)
	assertFixture(t, directOrdinary, false)

	sessionID, initResponse := mcpRequest(t, client, baseURL, "", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]string{"name": "plugin-integration", "version": "1.0"},
		},
	})
	if initResponse["result"] == nil {
		t.Fatalf("initialize response has no result: %#v", initResponse)
	}
	_, listResponse := mcpRequest(t, client, baseURL, sessionID, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{},
	})
	assertToolListed(t, listResponse, boundTool.Metadata.Name)
	assertToolListed(t, listResponse, ordinaryTool.Metadata.Name)

	_, boundCall := mcpRequest(t, client, baseURL, sessionID, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": boundTool.Metadata.Name, "arguments": map[string]any{}},
	})
	_, ordinaryCall := mcpRequest(t, client, baseURL, sessionID, map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{"name": ordinaryTool.Metadata.Name, "arguments": map[string]any{}},
	})
	assertFixture(t, mcpTextResult(t, boundCall), true)
	assertFixture(t, mcpTextResult(t, ordinaryCall), false)
}

func integrationTool(name string) mcp.Tool {
	return mcp.Tool{
		APIVersion: mcp.PluginAPIVersion,
		Kind:       "MCPTool",
		Metadata:   mcp.Metadata{Name: name, Version: "1.0.0", Module: "integration", IsActive: true},
		Spec: mcp.ToolSpec{
			Description: mcp.Description{Short: "Read the deterministic plugin fixture"},
			InputSchema: mcp.InputSchema{Type: "object", Properties: map[string]mcp.Property{}},
			Execution:   mcp.Execution{Type: "http", Method: http.MethodGet, Endpoint: "/api/resource/Plugin Fixture", ResponsePath: "data"},
			Security:    mcp.Security{AuthType: "api-key", CredentialRef: "ERP_PRIMARY_KEY"},
		},
	}
}

func applyJSON(t *testing.T, client *http.Client, endpoint string, resource any) {
	t.Helper()
	body, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("apply %s failed (%d): %s", endpoint, response.StatusCode, payload)
	}
}

func invokeDirect(t *testing.T, client *http.Client, baseURL, name string) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"arguments":{}}`, name)
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/tools/invoke", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("direct invoke failed (%d): %s", response.StatusCode, payload)
	}
	var result mcp.ToolResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	value, ok := result.Result.(map[string]any)
	if !ok {
		t.Fatalf("direct result is not an object: %#v", result.Result)
	}
	return value
}

func mcpRequest(t *testing.T, client *http.Client, baseURL, sessionID string, payload map[string]any) (string, map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/mcp/", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		request.Header.Set("Mcp-Session-Id", sessionID)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("MCP request failed (%d): %s", response.StatusCode, payload)
	}
	contents, _ := io.ReadAll(response.Body)
	result := decodeMCPResponse(t, contents)
	return response.Header.Get("Mcp-Session-Id"), result
}

func decodeMCPResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") {
		var response map[string]any
		if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.HasPrefix(line, "data: ") {
			var response map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &response); err == nil && response["result"] != nil {
				return response
			}
		}
	}
	t.Fatalf("MCP response did not contain JSON result: %s", body)
	return nil
}

func mcpTextResult(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP response has no result: %#v", response)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("MCP result has no content: %#v", result)
	}
	text, ok := content[0].(map[string]any)["text"].(string)
	if !ok {
		t.Fatalf("MCP result has no text: %#v", content[0])
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertFixture(t *testing.T, value map[string]any, processed bool) {
	t.Helper()
	if value["id"] != "plugin-fixture" || value["state"] != "source" {
		t.Fatalf("unexpected fixture: %#v", value)
	}
	_, hasMarker := value["processedBy"]
	if hasMarker != processed {
		t.Fatalf("processedBy=%v, want %v: %#v", hasMarker, processed, value)
	}
	if processed && value["processedBy"] != "mock-plugin" {
		t.Fatalf("unexpected marker: %#v", value)
	}
}

func assertToolListed(t *testing.T, response map[string]any, name string) {
	t.Helper()
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list has no result: %#v", response)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list has no tools: %#v", result)
	}
	for _, raw := range tools {
		if tool, ok := raw.(map[string]any); ok && tool["name"] == name {
			return
		}
	}
	t.Fatalf("tool %q was not listed: %#v", name, tools)
}
