package cli

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	"github.com/spf13/cobra"
)

// newBridgeRequest creates an authenticated request for the ERPBridge server.
// The explicit flag wins over the environment, which wins over the active
// context configuration.
func newBridgeRequest(cmd *cobra.Command, method, target string, body io.Reader) (*http.Request, error) {
	requestContext := cmd.Context()
	if requestContext == nil {
		requestContext = context.Background()
	}
	return bridgeclient.NewAuthenticatedRequest(requestContext, method, target, body, bridgeAPIToken())
}

func doBridgeRequest(cmd *cobra.Command, method, target string) (*http.Response, error) {
	return doBridgeRequestWithHeaders(cmd, method, target, nil, nil)
}

func doBridgeRequestWithHeaders(cmd *cobra.Command, method, target string, body io.Reader, headers http.Header) (*http.Response, error) {
	req, err := newBridgeRequest(cmd, method, target, body)
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return http.DefaultClient.Do(req)
}

func bridgeAPIToken() string {
	contextToken := ""
	if cfg != nil {
		contextToken = cfg.ActiveContext().APIToken
	}
	return bridgeclient.ResolveToken(tokenOverride, os.Getenv("BRIDGE_API_TOKEN"), contextToken)
}
