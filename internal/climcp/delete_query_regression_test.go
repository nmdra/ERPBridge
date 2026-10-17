package climcp

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

// Exercise confirmation, adapter arguments, and the compiled CLI together. The
// HTTP fixture records the exact identity and deletion mode received downstream.
func TestRealChildToolDeletePreservesQueryValues(t *testing.T) {
	requests := make(chan *url.URL, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		requests <- r.URL
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	session := realSessionWithGroups(t, &deploymentFixture{server: server}, []WriteGroup{GroupDestructive})

	for _, hard := range []bool{false, true} {
		for _, value := range []string{"1.0.0&hard=true", "literal&value", "literal#value", "literal%value", "literal+value", "literal?value"} {
			for _, field := range []string{"name", "version"} {
				t.Run(fmt.Sprintf("hard=%t/%s=%s", hard, field, value), func(t *testing.T) {
					name, version := "list_items", "1.0.0"
					if field == "name" {
						name = value
					} else {
						version = value
					}
					result := dispatchTool(t, session, "bridgectl_tool_delete", map[string]any{
						"name": name, "version": version, "hard": hard,
						"confirmContext": "test", "confirmTarget": name + "@" + version,
					})
					if !result.Success {
						t.Fatalf("delete failed: %+v", result.Error)
					}
					select {
					case requestURL := <-requests:
						if requestURL.Path != "/apis/erpbridge.io/v1/tools" {
							t.Errorf("path = %q", requestURL.Path)
						}
						query, err := url.ParseQuery(requestURL.RawQuery)
						if err != nil {
							t.Fatalf("parse query: %v", err)
						}
						want := url.Values{"name": {name}, "version": {version}}
						if hard {
							want.Set("hard", "true")
						}
						if !reflect.DeepEqual(query, want) {
							t.Errorf("query = %v, want %v", query, want)
						}
					default:
						t.Fatal("delete sent no request")
					}
				})
			}
		}
	}
}
