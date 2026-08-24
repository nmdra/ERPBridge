package mcp

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	pluginTestVersion  = testVersion100
	pluginTestModeKey  = "mode"
	pluginTestMode     = "safe"
	pluginTestResultID = "order-1"
	pluginTestToolName = "list-orders"
	pluginTimeoutError = "timeout"
	pluginVersionError = "metadata.version"
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
			wantErr: "http",
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
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestPluginBinding_Validate(t *testing.T) {
	tests := map[string]struct {
		mutate func(*PluginBinding)
	}{
		"accepts valid binding resource":     {},
		"requires name":                      {mutate: func(binding *PluginBinding) { binding.Metadata.Name = "" }},
		"requires plugin name":               {mutate: func(binding *PluginBinding) { binding.Spec.PluginRef.Name = "" }},
		"requires plugin version":            {mutate: func(binding *PluginBinding) { binding.Spec.PluginRef.Version = "" }},
		"requires tool name":                 {mutate: func(binding *PluginBinding) { binding.Spec.ToolRef.Name = "" }},
		"requires tool version":              {mutate: func(binding *PluginBinding) { binding.Spec.ToolRef.Version = "" }},
		"rejects unsupported phase":          {mutate: func(binding *PluginBinding) { binding.Spec.Phase = "before_request" }},
		"rejects unsupported failure policy": {mutate: func(binding *PluginBinding) { binding.Spec.FailurePolicy = "retry" }},
		"rejects negative priority":          {mutate: func(binding *PluginBinding) { binding.Spec.Priority = -1 }},
		"rejects non-json config":            {mutate: func(binding *PluginBinding) { binding.Spec.Config = map[string]any{"bad": make(chan int)} }},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			binding := validPluginBindingForTest()
			if test.mutate != nil {
				test.mutate(&binding)
			}
			err := binding.Validate()
			if test.mutate == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
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
