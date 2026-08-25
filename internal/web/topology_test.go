package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
	"github.com/nmdra/ERPBridge/internal/mcp"
)

func TestRootAPIInfersBasePrefixAcrossHTTPMethods(t *testing.T) {
	tool := mcp.Tool{Spec: mcp.ToolSpec{Execution: mcp.Execution{Method: "POST", Endpoint: "http://erp.local/api/resource/Employee"}}}
	apis := []idp.API{{Name: "mockerp", URL: "http://erp.local", Method: "GET"}}
	kind, matched := matchAPI(tool, apis, "http://erp.local")
	if kind != "base-prefix" || matched == nil || matched.Name != "mockerp" {
		t.Fatalf("match = %q, %+v; want base-prefix mockerp", kind, matched)
	}
}

func TestTopologyResolvesExactAmbiguousAndUnresolvedTools(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"metadata":{"name":"exact","version":"1.0.0","module":"finance"},"spec":{"execution":{"method":"GET","endpoint":"http://erp.local/api/invoices","responsePath":"data"},"security":{"credentialRef":"SECRET"}}},{"metadata":{"name":"missing","version":"1.0.0"},"spec":{"execution":{"method":"POST","endpoint":"/not-registered"}}},{"metadata":{"name":"ambiguous","version":"1.0.0"},"spec":{"execution":{"method":"GET","endpoint":"http://erp.local/api/shared"}}}]`))
	}))
	defer upstream.Close()
	registry, err := idp.NewRegistry(t.TempDir()+"/registry.json", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	for _, api := range []*idp.API{
		{ID: "exact-api", Name: "invoices", URL: "http://erp.local/api/invoices", Method: "GET", Module: "finance", AuthKey: "secret"},
		{ID: "shared-a", Name: "shared-a", URL: "http://erp.local/api/shared", Method: "GET"},
		{ID: "shared-b", Name: "shared-b", URL: "http://erp.local/api/shared/", Method: "GET"},
	} {
		registry.APIs[api.Name] = *api
	}
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {MCPServer: upstream.URL, ERPBase: "http://erp.local"}}}
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler:       NewConsoleHandler(HandlerOptions{Config: cfg, Registry: registry}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/topology?context=local", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "SECRET") || strings.Contains(recorder.Body.String(), "AuthKey") {
		t.Fatalf("topology contains credentials: %s", recorder.Body.String())
	}
	var topology TopologyResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &topology); err != nil {
		t.Fatal(err)
	}
	if topology.State != "available" || len(topology.Nodes) < 5 {
		t.Fatalf("topology = %+v", topology)
	}
	matches := map[string]bool{}
	for _, edge := range topology.Edges {
		matches[edge.MatchKind] = true
	}
	for _, kind := range []string{"exact", "ambiguous", "unresolved"} {
		if !matches[kind] {
			t.Fatalf("missing %s edge in %+v", kind, topology.Edges)
		}
	}
}
