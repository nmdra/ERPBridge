package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/stretchr/testify/require"

	"github.com/nmdra/ERPBridge/internal/testutil"
)

// emptyAnswerServer answers any request with the JSON null that an empty stored
// collection produces on the wire.
func emptyAnswerServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`null`))
	}))
	t.Cleanup(server.Close)
	return server
}

// useTestLogger installs a discard logger, because the registry commands need
// one and the test process has none.
func useTestLogger(t *testing.T) {
	t.Helper()
	previous := RootLog
	RootLog = slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Cleanup(func() { RootLog = previous })
}

// setControlPlaneContext points both the management and the MCP endpoint at one
// test server.
func setControlPlaneContext(t *testing.T, endpoint string) {
	t.Helper()
	cfg = &config.Config{
		CurrentContext: testContextName,
		Contexts: map[string]config.Context{
			testContextName: {Server: endpoint, MCPServer: endpoint},
		},
	}
}

// controlPlaneJSON runs one list command with the JSON format and returns what
// it printed. The previous shared state is restored afterwards, because this
// package's tests mutate package-level configuration.
func controlPlaneJSON(t *testing.T, run func() error) string {
	t.Helper()
	var buffer bytes.Buffer
	previousFormatter, previousFormat, previousConfig := formatter, outputFormat, cfg
	formatter = &output.Formatter{Format: output.FormatJSON, Out: &buffer}
	outputFormat = string(output.FormatJSON)
	toolGetCmd.SetOut(&buffer)
	pluginGetCmd.SetOut(&buffer)
	pluginBindingGetCmd.SetOut(&buffer)
	t.Cleanup(func() {
		formatter, outputFormat, cfg = previousFormatter, previousFormat, previousConfig
		toolGetCmd.SetOut(nil)
		pluginGetCmd.SetOut(nil)
		pluginBindingGetCmd.SetOut(nil)
		_ = toolGetCmd.Flags().Set("output", "table")
		_ = pluginGetCmd.Flags().Set("output", "table")
		_ = pluginBindingGetCmd.Flags().Set("output", "table")
	})
	require.NoError(t, toolGetCmd.Flags().Set("output", "json"))
	require.NoError(t, pluginGetCmd.Flags().Set("output", "json"))
	require.NoError(t, pluginBindingGetCmd.Flags().Set("output", "json"))
	require.NoError(t, run())
	return buffer.String()
}

// TestEmptyCollectionSerializesAsEmptyArray covers the coupling that the issue
// #18 fix would otherwise break: a nil slice marshals to null, and the coupled
// onboarding consumer refuses that shape, so the first run against an empty
// registry would stop working.
func TestEmptyCollectionSerializesAsEmptyArray(t *testing.T) {
	cases := []struct {
		name  string
		field string
		run   func(t *testing.T, endpoint string) string
	}{
		{
			name:  "tool get",
			field: "tools",
			run: func(t *testing.T, endpoint string) string {
				setControlPlaneContext(t, endpoint)
				return controlPlaneJSON(t, func() error { return toolGetCmd.RunE(toolGetCmd, nil) })
			},
		},
		{
			name:  "plugin get",
			field: "plugins",
			run: func(t *testing.T, endpoint string) string {
				setControlPlaneContext(t, endpoint)
				return controlPlaneJSON(t, func() error { return pluginGetCmd.RunE(pluginGetCmd, nil) })
			},
		},
		{
			name:  "plugin binding get",
			field: "bindings",
			run: func(t *testing.T, endpoint string) string {
				setControlPlaneContext(t, endpoint)
				return controlPlaneJSON(t, func() error { return pluginBindingGetCmd.RunE(pluginBindingGetCmd, nil) })
			},
		},
		{
			name:  "token list",
			field: "items",
			run: func(t *testing.T, endpoint string) string {
				setControlPlaneContext(t, endpoint)
				return controlPlaneJSON(t, func() error { return tokenListCmd.RunE(tokenListCmd, nil) })
			},
		},
		{
			name:  "context list",
			field: "items",
			run: func(t *testing.T, _ string) string {
				cfg = &config.Config{CurrentContext: testContextName, Contexts: map[string]config.Context{}}
				return controlPlaneJSON(t, func() error { return contextListCmd.RunE(contextListCmd, nil) })
			},
		},
		{
			name:  "api list",
			field: "items",
			run: func(t *testing.T, _ string) string {
				useTestLogger(t)
				// api list reads the local registry, not the HTTP fixture, so it
				// needs its own empty home.
				testutil.SetHome(t, t.TempDir())
				cfg = &config.Config{
					CurrentContext: testContextName,
					Contexts:       map[string]config.Context{testContextName: {}},
				}
				return controlPlaneJSON(t, func() error { return apiListCmd.RunE(apiListCmd, nil) })
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := testCase.run(t, emptyAnswerServer(t).URL)

			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(payload), &fields), "payload = %q", payload)
			raw, present := fields[testCase.field]
			require.True(t, present, "field %q is missing from %q", testCase.field, payload)
			require.Equal(t, "[]", strings.TrimSpace(string(raw)),
				"an empty collection must be a list, not null: %q", payload)
		})
	}
}
