package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/stretchr/testify/require"
)

func TestToolDeletePreservesQueryValues(t *testing.T) {
	previousConfig := cfg
	previousHard, _ := toolDeleteCmd.Flags().GetBool("hard")
	previousYes, _ := toolDeleteCmd.Flags().GetBool("yes")
	t.Cleanup(func() {
		cfg = previousConfig
		_ = toolDeleteCmd.Flags().Set("hard", fmt.Sprint(previousHard))
		_ = toolDeleteCmd.Flags().Set("yes", fmt.Sprint(previousYes))
	})

	for _, hard := range []bool{false, true} {
		for _, value := range []string{"1.0.0&hard=true", "literal&value", "literal#value", "literal%value", "literal+value", "literal?value"} {
			for _, field := range []string{"name", "version"} {
				t.Run(fmt.Sprintf("hard=%t/%s=%s", hard, field, value), func(t *testing.T) {
					requests := make(chan *url.URL, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodDelete {
							t.Errorf("method = %s, want DELETE", r.Method)
						}
						requests <- r.URL
						w.WriteHeader(http.StatusOK)
					}))
					t.Cleanup(server.Close)
					cfg = &config.Config{
						CurrentContext: testContextName,
						Contexts: map[string]config.Context{
							testContextName: {MCPServer: server.URL},
						},
					}
					name, version := "list_items", "1.0.0"
					if field == "name" {
						name = value
					} else {
						version = value
					}
					require.NoError(t, toolDeleteCmd.Flags().Set("hard", fmt.Sprint(hard)))
					require.NoError(t, toolDeleteCmd.Flags().Set("yes", "true"))
					require.NoError(t, toolDeleteCmd.RunE(toolDeleteCmd, []string{name, version}))
					select {
					case requestURL := <-requests:
						require.Equal(t, "/apis/erpbridge.io/v1/tools", requestURL.Path)
						query, err := url.ParseQuery(requestURL.RawQuery)
						require.NoError(t, err)
						want := url.Values{"name": {name}, "version": {version}}
						if hard {
							want.Set("hard", "true")
						}
						require.Equal(t, want, query)
					default:
						t.Fatal("delete sent no request")
					}
				})
			}
		}
	}
}
