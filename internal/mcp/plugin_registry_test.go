package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginRegistry_RuntimeBindingsForToolPhase(t *testing.T) {
	t.Setenv(authTokenEnv, "admin-token")
	t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:80")

	schema := any(map[string]any{"type": schemaTypeObject, "properties": map[string]any{"text": map[string]any{"type": schemaTypeString}}})
	tool := &Tool{
		Metadata: Metadata{Name: "phase-tool", Version: testVersion100, IsActive: true},
		Spec: ToolSpec{
			Execution:    Execution{Type: "http", Method: "GET", Endpoint: "/invoice"},
			OutputSchema: &schema,
		},
	}
	plugin := validPluginForTest("http://plugin.example.test")
	raw := validPluginBindingForTest()
	raw.Spec.ToolRef.Name = tool.Metadata.Name
	raw.Metadata.Name = "raw-second"
	raw.Spec.Phase = PluginPhaseRawResponse
	raw.Spec.Priority = 20
	after := validPluginBindingForTest()
	after.Spec.ToolRef.Name = tool.Metadata.Name
	after.Metadata.Name = "after-first"
	after.Spec.Priority = 10

	registry := NewPluginRegistry()
	registry.Replace(buildPluginBindingSnapshot([]*Plugin{&plugin}, []*PluginBinding{&raw, &after}, []*Tool{tool}))

	require.Len(t, registry.RuntimeBindingsForTool("phase-tool", testVersion100), 2)
	rawBindings := registry.RuntimeBindingsForToolPhase("phase-tool", testVersion100, PluginPhaseRawResponse)
	require.Len(t, rawBindings, 1)
	require.Equal(t, "raw-second", rawBindings[0].Binding.Metadata.Name)
	afterBindings := registry.RuntimeBindingsForToolPhase("phase-tool", testVersion100, PluginPhaseAfterResponse)
	require.Len(t, afterBindings, 1)
	require.Equal(t, "after-first", afterBindings[0].Binding.Metadata.Name)
}

func TestBuildPluginBindingSnapshot_DeactivatesRawBindingWithoutAdmission(t *testing.T) {
	t.Setenv(authTokenEnv, "")
	t.Setenv("PLUGIN_ENDPOINT_ALLOWLIST", "plugin.example.test:80")
	schema := any(map[string]any{"type": schemaTypeObject})
	tool := &Tool{
		Metadata: Metadata{Name: "raw-tool", Version: testVersion100, IsActive: true},
		Spec:     ToolSpec{Execution: Execution{Type: "http", Endpoint: "/raw"}, OutputSchema: &schema},
	}
	plugin := validPluginForTest("http://plugin.example.test")
	binding := validPluginBindingForTest()
	binding.Spec.ToolRef.Name = tool.Metadata.Name
	binding.Spec.Phase = PluginPhaseRawResponse

	snapshot := buildPluginBindingSnapshot([]*Plugin{&plugin}, []*PluginBinding{&binding}, []*Tool{tool})
	require.Empty(t, snapshot)
}
