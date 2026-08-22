package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTokenCreateCmdUsesBridgeTokenAndReturnsValueOnce(t *testing.T) {
	var receivedAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthorization = r.Header.Get("Authorization")
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "cli-client", payload[cliNameField])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"erpbt_id","name":"cli-client","token":"erpbt_secret"}`))
	}))
	defer server.Close()

	setupTest()
	cfg.Contexts[testContextName] = config.Context{Server: server.URL}
	tokenOverride = "cli-auth"
	t.Cleanup(func() { tokenOverride = "" })
	tokenName = "cli-client"
	tokenScopes = []string{cliMCPScope}
	tokenRoles = []string{"operator"}
	tokenExpires = ""
	formatter.Out = io.Discard
	tokenCreateCmd.SetContext(context.Background())

	require.NoError(t, tokenCreateCmd.RunE(tokenCreateCmd, nil))
	require.Equal(t, "Bearer cli-auth", receivedAuthorization)
}

func TestParseTokenExpiry(t *testing.T) {
	absolute, err := parseTokenExpiry("2030-01-01T00:00:00Z")
	require.NoError(t, err)
	require.Equal(t, 2030, absolute.Year())

	duration, err := parseTokenExpiry("1h")
	require.NoError(t, err)
	require.True(t, duration.After(time.Now()))

	_, err = parseTokenExpiry("0s")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "invalid token expiry"))
}
