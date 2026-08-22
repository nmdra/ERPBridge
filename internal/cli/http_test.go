package cli

import (
	"context"
	"net/http"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/stretchr/testify/require"
)

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
