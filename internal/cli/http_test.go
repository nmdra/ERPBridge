package cli

import (
	"context"
	"net/http"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestDoBridgeRequestReturnsActionableUnreachableError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	_, err := doBridgeRequest(cmd, http.MethodGet, "http://127.0.0.1:1")
	require.Error(t, err)
	var actionable *AgentActionableError
	require.ErrorAs(t, err, &actionable)
	require.Equal(t, "UPSTREAM_UNREACHABLE", actionable.ErrorCode)
}

func TestNewBridgeRequestTokenPrecedence(t *testing.T) {
	setupTest()
	cfg.Contexts[testContextName] = config.Context{Server: testServerURL, APIToken: "context-token"}
	cmd := RootCmd
	cmd.SetContext(context.Background())
	t.Setenv("BRIDGE_API_TOKEN", "environment-token")
	tokenOverride = ""
	t.Cleanup(func() { tokenOverride = "" })

	req, err := newBridgeRequest(cmd, http.MethodGet, testServerURL, nil)
	require.NoError(t, err)
	require.Equal(t, "Bearer environment-token", req.Header.Get("Authorization"))

	tokenOverride = "flag-token"
	req, err = newBridgeRequest(cmd, http.MethodGet, testServerURL, nil)
	require.NoError(t, err)
	require.Equal(t, "Bearer flag-token", req.Header.Get("Authorization"))

	tokenOverride = ""
	t.Setenv("BRIDGE_API_TOKEN", "")
	req, err = newBridgeRequest(cmd, http.MethodGet, testServerURL, nil)
	require.NoError(t, err)
	require.Equal(t, "Bearer context-token", req.Header.Get("Authorization"))
}
