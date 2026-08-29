package web

import (
	"encoding/json"
	"fmt"
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
	kind, matched, diagnosticReason := matchAPI(tool, apis, "http://erp.local")
	if kind != "base-prefix" || matched == nil || matched.Name != "mockerp" || diagnosticReason != "" {
		t.Fatalf("match = %q, %+v, %q; want base-prefix mockerp without diagnostic", kind, matched, diagnosticReason)
	}
}

func TestTopologyIncludesPluginBindingsWhenFeatureIsAvailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/erpbridge.io/v1/tools":
			_, _ = w.Write([]byte(`[{"metadata":{"name":"list_orders","version":"1.0.0"},"spec":{"execution":{"method":"GET","endpoint":"http://erp.local/api/orders"}}}]`))
		case "/apis/erpbridge.io/v1/plugins":
			_, _ = w.Write([]byte(`[{"apiVersion":"erpbridge.io/v1","kind":"Plugin","metadata":{"name":"transformer","version":"1.2.0","isActive":true},"spec":{"endpoint":"https://plugin.internal.example/v1/process","timeoutMilliseconds":1500}}]`))
		case "/apis/erpbridge.io/v1/pluginbindings":
			_, _ = w.Write([]byte(`[{"apiVersion":"erpbridge.io/v1","kind":"PluginBinding","metadata":{"name":"transform-orders","isActive":true},"spec":{"pluginRef":{"name":"transformer","version":"1.2.0"},"toolRef":{"name":"list_orders","version":"1.0.0"},"phase":"after_response","priority":10,"failurePolicy":"continue"}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {MCPServer: upstream.URL, ERPBase: "http://erp.local"}}}
	console, err := NewServer(Options{ListenAddress: "127.0.0.1:0", Handler: NewConsoleHandler(HandlerOptions{Config: cfg})})
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
	var topology TopologyResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &topology); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, node := range topology.Nodes {
		kinds[node.Kind]++
	}
	if kinds["external-plugin"] != 1 || kinds["plugin-binding"] != 1 {
		t.Fatalf("plugin nodes = %+v", kinds)
	}
	pluginEdges := 0
	for _, edge := range topology.Edges {
		if strings.HasPrefix(edge.Source, "binding:") || strings.HasPrefix(edge.Target, "binding:") {
			pluginEdges++
		}
	}
	if pluginEdges != 2 {
		t.Fatalf("plugin edges = %d, edges = %+v", pluginEdges, topology.Edges)
	}
}

func TestTopologyReportsTruncation(t *testing.T) {
	tools := make([]mcp.Tool, maxTopologyNodes+1)
	for index := range tools {
		tools[index] = mcp.Tool{
			Metadata: mcp.Metadata{Name: "tool-" + fmt.Sprint(index), Version: "1.0.0"},
			Spec:     mcp.ToolSpec{Execution: mcp.Execution{Method: http.MethodGet, Endpoint: "/api/resource/" + fmt.Sprint(index)}},
		}
	}
	payload, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/erpbridge.io/v1/tools" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{"local": {MCPServer: upstream.URL, ERPBase: "http://erp.local"}}}
	console, err := NewServer(Options{ListenAddress: "127.0.0.1:0", Handler: NewConsoleHandler(HandlerOptions{Config: cfg})})
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
	var topology TopologyResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &topology); err != nil {
		t.Fatal(err)
	}
	if !topology.Truncated || topology.Omitted == nil || topology.Omitted.Nodes == 0 || topology.Omitted.Edges == 0 {
		t.Fatalf("topology truncation = %+v", topology)
	}
	nodeIDs := make(map[string]struct{}, len(topology.Nodes))
	for _, node := range topology.Nodes {
		nodeIDs[node.ID] = struct{}{}
	}
	for _, edge := range topology.Edges {
		if _, ok := nodeIDs[edge.Source]; !ok {
			t.Fatalf("edge source %q is not admitted", edge.Source)
		}
		if _, ok := nodeIDs[edge.Target]; !ok {
			t.Fatalf("edge target %q is not admitted", edge.Target)
		}
	}
}

func TestTopologyLoadsSelectedContextRegistryWithoutInjectedRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	registry, err := idp.NewRegistryForContext("selected", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&idp.API{
		ID:     "selected-api",
		Name:   "selected-invoices",
		URL:    "http://erp.local/api/invoices",
		Method: http.MethodGet,
	}); err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/erpbridge.io/v1/tools" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"metadata":{"name":"list_invoices","version":"1.0.0"},"spec":{"execution":{"method":"GET","endpoint":"http://erp.local/api/invoices"}}}]`))
	}))
	defer upstream.Close()

	cfg := &config.Config{CurrentContext: "selected", Contexts: map[string]config.Context{
		"selected": {MCPServer: upstream.URL, ERPBase: "http://erp.local"},
		"other":    {MCPServer: upstream.URL, ERPBase: "http://erp.local"},
	}}
	var loadedContexts []string
	console, err := NewServer(Options{
		ListenAddress: "127.0.0.1:0",
		Handler: NewConsoleHandler(HandlerOptions{
			Config: cfg,
			RegistryProvider: func(contextName string) (*idp.Registry, error) {
				loadedContexts = append(loadedContexts, contextName)
				return idp.NewRegistryForContext(contextName, slog.Default())
			},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = console.Close() }()

	request := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/topology?context=selected", nil)
	request.Host = console.Host()
	request.Header.Set(CapabilityHeader, console.Capability())
	recorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var topology TopologyResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &topology); err != nil {
		t.Fatal(err)
	}
	if topology.State != stateAvailable {
		t.Fatalf("state = %q; want %q", topology.State, stateAvailable)
	}
	var selectedAPI bool
	for _, node := range topology.Nodes {
		if node.Kind == "erp-api" && node.Label == "selected-invoices" {
			selectedAPI = true
		}
	}
	if !selectedAPI {
		t.Fatalf("selected context API is absent from topology nodes: %+v", topology.Nodes)
	}
	var exactMatched bool
	for _, edge := range topology.Edges {
		if edge.MatchKind == matchExact && edge.Target == "api:selected-api" {
			exactMatched = true
		}
	}
	if !exactMatched {
		t.Fatalf("selected context API has no exact match: %+v", topology.Edges)
	}
	var endpointID string
	for _, node := range topology.Nodes {
		if node.Kind == "erp-endpoint" && node.Endpoint != nil &&
			node.Endpoint.Method == http.MethodGet && node.Endpoint.Path == "/api/invoices" {
			endpointID = node.ID
		}
	}
	if endpointID == "" {
		t.Fatalf("exact ERP endpoint is absent: %+v", topology.Nodes)
	}
	var apiEndpointMatched bool
	for _, edge := range topology.Edges {
		if edge.Source == "api:selected-api" && edge.Target == endpointID && edge.MatchKind == matchExact {
			apiEndpointMatched = true
		}
	}
	if !apiEndpointMatched {
		t.Fatalf("selected API has no exact endpoint relationship: %+v", topology.Edges)
	}

	omittedRequest := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/topology", nil)
	omittedRequest.Host = console.Host()
	omittedRequest.Header.Set(CapabilityHeader, console.Capability())
	omittedRecorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(omittedRecorder, omittedRequest)
	if omittedRecorder.Code != http.StatusOK {
		t.Fatalf("omitted context status = %d body = %s", omittedRecorder.Code, omittedRecorder.Body.String())
	}
	if len(loadedContexts) < 2 || loadedContexts[len(loadedContexts)-1] != "selected" {
		t.Fatalf("registry contexts = %v; want omitted context to resolve to selected", loadedContexts)
	}

	otherRequest := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/topology?context=other", nil)
	otherRequest.Host = console.Host()
	otherRequest.Header.Set(CapabilityHeader, console.Capability())
	otherRecorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(otherRecorder, otherRequest)
	if otherRecorder.Code != http.StatusOK {
		t.Fatalf("other context status = %d body = %s", otherRecorder.Code, otherRecorder.Body.String())
	}
	var otherTopology TopologyResponse
	if err := json.Unmarshal(otherRecorder.Body.Bytes(), &otherTopology); err != nil {
		t.Fatal(err)
	}
	for _, node := range otherTopology.Nodes {
		if node.Kind == "erp-api" {
			t.Fatalf("other context leaked selected API: %+v", otherTopology.Nodes)
		}
	}
	for _, edge := range otherTopology.Edges {
		if edge.MatchKind == "unresolved" && edge.DiagnosticReason == "No ERP APIs are registered." {
			return
		}
	}
	t.Fatalf("other context has no registry-empty unresolved diagnostic: %+v", otherTopology.Edges)
}

func TestTopologyResolvesExactAmbiguousAndUnresolvedTools(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"metadata":{"name":"exact","version":"1.0.0","module":"finance"},"spec":{"execution":{"method":"GET","endpoint":"https://tool-user:TOOL_URL_SECRET@erp.private.example/api/invoices?token=QUERY_SECRET","responsePath":"data"},"security":{"credentialRef":"SECRET"}}},{"metadata":{"name":"missing-endpoint","version":"1.0.0"},"spec":{"execution":{"method":"POST","endpoint":""}}},{"metadata":{"name":"method-mismatch","version":"1.0.0"},"spec":{"execution":{"method":"POST","endpoint":"/api/invoices"}}},{"metadata":{"name":"ambiguous","version":"1.0.0"},"spec":{"execution":{"method":"GET","endpoint":"https://erp.private.example/api/shared"}}}]`))
	}))
	defer upstream.Close()
	registry, err := idp.NewRegistry(t.TempDir()+"/registry.json", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	for _, api := range []*idp.API{
		// #nosec G101 -- this URL is a deliberate redaction-test fixture, not a credential.
		{ID: "exact-api", Name: "invoices", URL: "https://api-user:API_URL_SECRET@erp.private.example/api/invoices", Method: "GET", Module: "finance", CredentialRef: "ERP_SECRET"},
		{ID: "shared-a", Name: "shared-a", URL: "https://erp.private.example/api/shared", Method: "GET"},
		{ID: "shared-b", Name: "shared-b", URL: "https://erp.private.example/api/shared/", Method: "GET"},
	} {
		registry.APIs[api.Name] = *api
	}
	cfg := &config.Config{CurrentContext: "local", Contexts: map[string]config.Context{
		"local": {MCPServer: upstream.URL, ERPBase: "https://erp.private.example"},
		"other": {MCPServer: upstream.URL, ERPBase: "https://erp.private.example"},
	}}
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
	for _, sensitive := range []string{"SECRET", "erp.private.example", "tool-user", "api-user", "AuthKey"} {
		if strings.Contains(recorder.Body.String(), sensitive) {
			t.Fatalf("topology contains sensitive endpoint data %q: %s", sensitive, recorder.Body.String())
		}
	}
	var topology TopologyResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &topology); err != nil {
		t.Fatal(err)
	}
	if topology.State != "available" || len(topology.Nodes) < 5 {
		t.Fatalf("topology = %+v", topology)
	}
	diagnostics := map[string]bool{}
	for _, edge := range topology.Edges {
		diagnostics[edge.DiagnosticReason] = true
	}
	for _, diagnostic := range []string{
		"More than one registered ERP API matches this endpoint.",
		"The tool has no endpoint.",
		"Registered ERP APIs use a different method.",
	} {
		if !diagnostics[diagnostic] {
			t.Fatalf("missing diagnostic %q in %+v", diagnostic, topology.Edges)
		}
	}
	var ambiguousNode bool
	for _, node := range topology.Nodes {
		if node.Kind == "ambiguous-endpoint" && node.DiagnosticReason == "More than one registered ERP API matches this endpoint." {
			ambiguousNode = true
		}
	}
	if !ambiguousNode {
		t.Fatalf("ambiguous endpoint node is absent: %+v", topology.Nodes)
	}

	otherRequest := httptest.NewRequest(http.MethodGet, console.URL()+"/api/console/v1/topology?context=other", nil)
	otherRequest.Host = console.Host()
	otherRequest.Header.Set(CapabilityHeader, console.Capability())
	otherRecorder := httptest.NewRecorder()
	console.Handler().ServeHTTP(otherRecorder, otherRequest)
	if otherRecorder.Code != http.StatusOK {
		t.Fatalf("other context status = %d body = %s", otherRecorder.Code, otherRecorder.Body.String())
	}
	var otherTopology TopologyResponse
	if err := json.Unmarshal(otherRecorder.Body.Bytes(), &otherTopology); err != nil {
		t.Fatal(err)
	}
	for _, node := range otherTopology.Nodes {
		if node.Kind == "erp-api" {
			t.Fatalf("injected current-context registry leaked into other context: %+v", otherTopology.Nodes)
		}
	}
}

func TestMatchAPIReportsSafeDiagnosticReasons(t *testing.T) {
	base := "https://erp.private.example"
	cases := []struct {
		name       string
		tool       mcp.Tool
		apis       []idp.API
		matchKind  string
		diagnostic string
	}{
		{
			name:       "missing endpoint",
			tool:       mcp.Tool{Spec: mcp.ToolSpec{Execution: mcp.Execution{Method: http.MethodGet}}},
			matchKind:  "unresolved",
			diagnostic: "The tool has no endpoint.",
		},
		{
			name:       "empty API registry",
			tool:       mcp.Tool{Spec: mcp.ToolSpec{Execution: mcp.Execution{Method: http.MethodGet, Endpoint: "/api/invoices"}}},
			matchKind:  "unresolved",
			diagnostic: "No ERP APIs are registered.",
		},
		{
			name:       "host mismatch",
			tool:       mcp.Tool{Spec: mcp.ToolSpec{Execution: mcp.Execution{Method: http.MethodGet, Endpoint: "/api/invoices"}}},
			apis:       []idp.API{{Name: "other", Method: http.MethodGet, URL: "https://other.private.example/api/invoices"}},
			matchKind:  "unresolved",
			diagnostic: "No registered ERP API matches the endpoint host.",
		},
		{
			name:       "method mismatch",
			tool:       mcp.Tool{Spec: mcp.ToolSpec{Execution: mcp.Execution{Method: http.MethodPost, Endpoint: "/api/invoices"}}},
			apis:       []idp.API{{Name: "invoices", Method: http.MethodGet, URL: "/api/invoices"}},
			matchKind:  "unresolved",
			diagnostic: "Registered ERP APIs use a different method.",
		},
		{
			name:       "no path candidate",
			tool:       mcp.Tool{Spec: mcp.ToolSpec{Execution: mcp.Execution{Method: http.MethodGet, Endpoint: "/api/orders"}}},
			apis:       []idp.API{{Name: "invoices", Method: http.MethodGet, URL: "/api/invoices"}},
			matchKind:  "unresolved",
			diagnostic: "No registered ERP API matches this endpoint.",
		},
		{
			name: "ambiguous",
			tool: mcp.Tool{Spec: mcp.ToolSpec{Execution: mcp.Execution{Method: http.MethodGet, Endpoint: "/api/invoices"}}},
			apis: []idp.API{
				{Name: "invoices-a", Method: http.MethodGet, URL: "/api/invoices"},
				{Name: "invoices-b", Method: http.MethodGet, URL: "/api/invoices/"},
			},
			matchKind:  "ambiguous",
			diagnostic: "More than one registered ERP API matches this endpoint.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, matched, diagnostic := matchAPI(tc.tool, tc.apis, base)
			if kind != tc.matchKind || matched != nil || diagnostic != tc.diagnostic {
				t.Fatalf("match = %q, %+v, %q; want %q, nil, %q", kind, matched, diagnostic, tc.matchKind, tc.diagnostic)
			}
			for _, sensitive := range []string{"erp.private.example", "other.private.example"} {
				if strings.Contains(diagnostic, sensitive) {
					t.Fatalf("diagnostic exposes endpoint data %q: %q", sensitive, diagnostic)
				}
			}
		})
	}
}
