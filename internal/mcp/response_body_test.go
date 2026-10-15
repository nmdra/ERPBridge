package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/security"
	"github.com/stretchr/testify/require"
)

// Release the fixture body only after the real connector has returned headers.
// This reproduces the ownership boundary between the connector and MCP decoder.
type responseBodyConnector struct {
	next    *connector.Client
	release func()
}

func (c responseBodyConnector) Call(ctx context.Context, ep connector.EndpointConfig, query url.Values, body io.Reader) (*http.Response, error) {
	resp, err := c.next.Call(ctx, ep, query, body)
	c.release()
	return resp, err
}

func (c responseBodyConnector) CallWithOptions(ctx context.Context, ep connector.EndpointConfig, query url.Values, body io.Reader, options connector.CallOptions) (*http.Response, error) {
	resp, err := c.next.CallWithOptions(ctx, ep, query, body, options)
	c.release()
	return resp, err
}

func TestToolStreamingResponseBodyWithRealConnector(t *testing.T) {
	for _, raw := range []bool{false, true} {
		name := "json_decode"
		if raw {
			name = "raw_capture"
		}
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			const payload = `{"decision":"deny","reason":"fixture policy","conditions":[]}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Key") != "fixture-policy-key" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				select {
				case <-release:
					_, _ = io.WriteString(w, payload)
				case <-r.Context().Done():
				}
			}))
			t.Cleanup(func() {
				unblock()
				upstream.Close()
			})
			u, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			t.Setenv(security.InsecureAuthAllowedHostsEnv, u.Host)
			t.Setenv("RESPONSE_BODY_POLICY_TEST_KEY", "fixture-policy-key")
			log := logger.Init()
			conn := responseBodyConnector{next: connector.NewClient(log), release: unblock}
			tool := Tool{Spec: ToolSpec{
				Execution: Execution{Type: "http", Method: http.MethodPost, Endpoint: upstream.URL},
				Security:  Security{AuthType: "api-key", AuthHeader: "X-API-Key", CredentialRef: "RESPONSE_BODY_POLICY_TEST_KEY"},
			}}
			if raw {
				result, err := tool.CallERP(context.Background(), map[string]any{}, conn, connector.CallOptions{PreserveErrorResponses: true})
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, result.Status)
				require.JSONEq(t, payload, string(result.Body))
				return
			}
			result, err := tool.Execute(context.Background(), map[string]any{}, conn)
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.Equal(t, "deny", result.Result.(map[string]any)["decision"])
		})
	}
}
