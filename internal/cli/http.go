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
	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return nil, unreachableControlPlaneError(err)
	}
	return resp, nil
}

var cliHTTPClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// bridgeResponseError consumes a bounded remote error envelope and maps it to
// the CLI's stable exit-code contract. It must be called before closing resp.
func bridgeResponseError(resp *http.Response) error {
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	return mapRemoteError(bridgeclient.DecodeRemoteErrorResponse(resp))
}

func bridgeAPIToken() string {
	contextToken := ""
	if cfg != nil {
		if context, err := cfg.EffectiveContext(); err == nil {
			contextToken = context.APIToken
		}
	}
	return bridgeclient.ResolveToken(tokenOverride, os.Getenv("BRIDGE_API_TOKEN"), contextToken)
}
