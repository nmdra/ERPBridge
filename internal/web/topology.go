package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
	"github.com/nmdra/ERPBridge/internal/mcp"
)

const (
	maxTopologyNodes = 500
	maxTopologyEdges = 1000
)

// TopologyResponse is the bounded, safe API-to-MCP graph.
type TopologyResponse struct {
	State      string           `json:"state"`
	Nodes      []TopologyNode   `json:"nodes"`
	Edges      []TopologyEdge   `json:"edges"`
	Truncated  bool             `json:"truncated"`
	Omitted    *TopologyOmitted `json:"omitted,omitempty"`
	ObservedAt time.Time        `json:"observedAt"`
}

// TopologyOmitted reports safe counts omitted by the graph safety caps.
type TopologyOmitted struct {
	Nodes int `json:"nodes"`
	Edges int `json:"edges"`
}

// TopologyNode represents a transport, tool, API, plugin, binding, unresolved endpoint, or ambiguous endpoint.
type TopologyNode struct {
	ID               string                   `json:"id"`
	Kind             string                   `json:"kind"`
	Label            string                   `json:"label"`
	DiagnosticReason string                   `json:"diagnosticReason,omitempty"`
	ContextState     string                   `json:"contextState,omitempty"`
	Tool             *ToolProjection          `json:"tool,omitempty"`
	API              *TopologyAPIDetails      `json:"api,omitempty"`
	Plugin           *PluginProjection        `json:"plugin,omitempty"`
	Binding          *PluginBindingProjection `json:"binding,omitempty"`
}

// TopologyAPIDetails contains an API registry projection without credentials.
type TopologyAPIDetails struct {
	Name         string `json:"name"`
	Module       string `json:"module,omitempty"`
	Method       string `json:"method"`
	EndpointPath string `json:"endpointPath"`
	Status       string `json:"status,omitempty"`
}

// TopologyEdge represents one graph relationship and its match confidence.
type TopologyEdge struct {
	ID               string `json:"id"`
	Source           string `json:"source"`
	Target           string `json:"target"`
	MatchKind        string `json:"matchKind"`
	DiagnosticReason string `json:"diagnosticReason,omitempty"`
	ContextState     string `json:"contextState,omitempty"`
	Authoritative    bool   `json:"authoritative"`
}

type normalizedEndpoint struct {
	Method string
	Host   string
	Path   string
}

func (h *consoleHandler) topology(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctxName, ctx, ok := h.contextNameForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, targetMCPServer(), "/apis/erpbridge.io/v1/tools")
	if err != nil {
		writeJSON(w, http.StatusOK, TopologyResponse{State: stateUnavailable, Nodes: []TopologyNode{}, Edges: []TopologyEdge{}})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, TopologyResponse{State: upstreamState(response.StatusCode), Nodes: []TopologyNode{}, Edges: []TopologyEdge{}})
		return
	}
	var tools []mcp.Tool
	if err := jsonDecoder(response.Body, &tools); err != nil {
		writeJSON(w, http.StatusOK, TopologyResponse{State: stateUnavailable, Nodes: []TopologyNode{}, Edges: []TopologyEdge{}})
		return
	}

	plugins, bindings, pluginsAvailable := h.fetchPluginResources(r, ctx)

	registry, err := h.registryForContext(ctxName)
	if err != nil {
		writeJSON(w, http.StatusOK, TopologyResponse{State: stateUnavailable, Nodes: []TopologyNode{}, Edges: []TopologyEdge{}})
		return
	}
	apis := registry.List()
	sort.Slice(apis, func(i, j int) bool { return apis[i].Name < apis[j].Name })
	candidateNodes, candidateEdges := topologyCandidateCounts(tools, apis, plugins, bindings, pluginsAvailable, ctx.ERPBase)

	graph := TopologyResponse{State: stateAvailable, Nodes: make([]TopologyNode, 0), Edges: make([]TopologyEdge, 0), ObservedAt: time.Now().UTC()}
	transportID := "transport:" + ctxName
	graph.Nodes = append(graph.Nodes, TopologyNode{ID: transportID, Kind: "mcp-transport", Label: "MCP transport"})
	for _, api := range apis {
		if len(graph.Nodes) >= maxTopologyNodes {
			break
		}
		graph.Nodes = append(graph.Nodes, TopologyNode{
			ID:           "api:" + stableID(api.ID, api.Name),
			Kind:         "erp-api",
			Label:        api.Name,
			ContextState: apiContextState(api.URL, ctx.ERPBase),
			API: &TopologyAPIDetails{
				Name:         api.Name,
				Module:       api.Module,
				Method:       strings.ToUpper(api.Method),
				EndpointPath: safeEndpointPath(api.URL),
				Status:       api.Status,
			},
		})
	}
	toolIDs := make(map[string]string, len(tools))
	for _, tool := range tools {
		if len(graph.Nodes) >= maxTopologyNodes {
			break
		}
		toolID := "tool:" + stableID(tool.Metadata.Name, tool.Metadata.Version)
		toolIDs[resourceIdentity(tool.Metadata.Name, tool.Metadata.Version)] = toolID
		projection := projectTool(tool)
		graph.Nodes = append(graph.Nodes, TopologyNode{ID: toolID, Kind: "mcp-tool", Label: tool.Metadata.Name, Tool: &projection})
		if len(graph.Edges) < maxTopologyEdges {
			graph.Edges = append(graph.Edges, TopologyEdge{ID: "edge:" + transportID + ":" + toolID, Source: transportID, Target: toolID, MatchKind: matchExact, Authoritative: true})
		}
		kind, matched, diagnosticReason := matchAPI(tool, apis, ctx.ERPBase)
		if matched != nil {
			apiID := "api:" + stableID(matched.ID, matched.Name)
			if len(graph.Edges) < maxTopologyEdges {
				graph.Edges = append(graph.Edges, TopologyEdge{ID: "edge:" + toolID + ":" + apiID, Source: toolID, Target: apiID, MatchKind: kind, ContextState: apiContextState(matched.URL, ctx.ERPBase), Authoritative: kind == matchExact})
			}
		} else {
			endpointKind := "unresolved-endpoint"
			endpointID := "unresolved:" + stableID(tool.Metadata.Name, tool.Metadata.Version)
			if kind == "ambiguous" {
				endpointKind = "ambiguous-endpoint"
				endpointID = "ambiguous:" + stableID(tool.Metadata.Name, tool.Metadata.Version)
			}
			if len(graph.Nodes) < maxTopologyNodes {
				graph.Nodes = append(graph.Nodes, TopologyNode{ID: endpointID, Kind: endpointKind, Label: projection.EndpointPath, DiagnosticReason: diagnosticReason})
			}
			if len(graph.Edges) < maxTopologyEdges {
				graph.Edges = append(graph.Edges, TopologyEdge{ID: "edge:" + toolID + ":" + endpointID, Source: toolID, Target: endpointID, MatchKind: kind, DiagnosticReason: diagnosticReason, Authoritative: false})
			}
		}
	}

	if pluginsAvailable {
		pluginIDs := make(map[string]string, len(plugins))
		for _, plugin := range plugins {
			if len(graph.Nodes) >= maxTopologyNodes {
				break
			}
			projection := projectPlugin(plugin)
			pluginID := "plugin:" + stableID(plugin.Metadata.Name, plugin.Metadata.Version)
			pluginIDs[resourceIdentity(plugin.Metadata.Name, plugin.Metadata.Version)] = pluginID
			graph.Nodes = append(graph.Nodes, TopologyNode{
				ID:     pluginID,
				Kind:   "external-plugin",
				Label:  plugin.Metadata.Name,
				Plugin: &projection,
			})
		}

		bindingIDs := make(map[string]string, len(bindings))
		for _, binding := range bindings {
			if len(graph.Nodes) >= maxTopologyNodes {
				break
			}
			projection := projectPluginBinding(binding)
			bindingID := "binding:" + stableID(binding.Metadata.Name, binding.Metadata.Name)
			bindingIDs[binding.Metadata.Name] = bindingID
			graph.Nodes = append(graph.Nodes, TopologyNode{
				ID:      bindingID,
				Kind:    "plugin-binding",
				Label:   binding.Metadata.Name,
				Binding: &projection,
			})
		}

		for _, binding := range bindings {
			bindingID, bindingOK := bindingIDs[binding.Metadata.Name]
			toolID, toolOK := toolIDs[resourceIdentity(binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)]
			pluginID, pluginOK := pluginIDs[resourceIdentity(binding.Spec.PluginRef.Name, binding.Spec.PluginRef.Version)]
			if bindingOK && toolOK && len(graph.Edges) < maxTopologyEdges {
				graph.Edges = append(graph.Edges, TopologyEdge{
					ID:            "edge:" + toolID + ":" + bindingID,
					Source:        toolID,
					Target:        bindingID,
					MatchKind:     matchExact,
					Authoritative: true,
				})
			}
			if bindingOK && pluginOK && len(graph.Edges) < maxTopologyEdges {
				graph.Edges = append(graph.Edges, TopologyEdge{
					ID:            "edge:" + bindingID + ":" + pluginID,
					Source:        bindingID,
					Target:        pluginID,
					MatchKind:     matchExact,
					Authoritative: true,
				})
			}
		}
	}

	admittedNodeIDs := make(map[string]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		admittedNodeIDs[node.ID] = struct{}{}
	}
	closedEdges := graph.Edges[:0]
	for _, edge := range graph.Edges {
		if _, sourceOK := admittedNodeIDs[edge.Source]; !sourceOK {
			continue
		}
		if _, targetOK := admittedNodeIDs[edge.Target]; !targetOK {
			continue
		}
		closedEdges = append(closedEdges, edge)
	}
	graph.Edges = closedEdges

	if candidateNodes > len(graph.Nodes) || candidateEdges > len(graph.Edges) {
		graph.Truncated = true
		graph.Omitted = &TopologyOmitted{
			Nodes: maxInt(0, candidateNodes-len(graph.Nodes)),
			Edges: maxInt(0, candidateEdges-len(graph.Edges)),
		}
	}
	writeJSON(w, http.StatusOK, graph)
}

func (h *consoleHandler) registryForContext(contextName string) (*idp.Registry, error) {
	if h.registryProvider != nil {
		return h.registryProvider(contextName)
	}
	if h.registry != nil {
		cfg, _, _, _ := h.configSnapshot(false)
		if cfg != nil && cfg.CurrentContext == contextName {
			return h.registry, nil
		}
	}
	return idp.NewRegistryForContext(contextName, slog.Default())
}

func topologyCandidateCounts(tools []mcp.Tool, apis []idp.API, plugins []mcp.Plugin, bindings []mcp.PluginBinding, pluginsAvailable bool, base string) (int, int) {
	unresolved := 0
	for _, tool := range tools {
		_, matched, _ := matchAPI(tool, apis, base)
		if matched == nil {
			unresolved++
		}
	}
	nodes := 1 + len(apis) + len(tools) + unresolved
	edges := len(tools) * 2
	if !pluginsAvailable {
		return nodes, edges
	}
	nodes += len(plugins) + len(bindings)
	pluginIDs := make(map[string]struct{}, len(plugins))
	for _, plugin := range plugins {
		pluginIDs[resourceIdentity(plugin.Metadata.Name, plugin.Metadata.Version)] = struct{}{}
	}
	toolIDs := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		toolIDs[resourceIdentity(tool.Metadata.Name, tool.Metadata.Version)] = struct{}{}
	}
	for _, binding := range bindings {
		if _, ok := toolIDs[resourceIdentity(binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)]; !ok {
			continue
		}
		if _, ok := pluginIDs[resourceIdentity(binding.Spec.PluginRef.Name, binding.Spec.PluginRef.Version)]; ok {
			edges += 2
		}
	}
	return nodes, edges
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func targetMCPServer() bridgeclient.Target { return bridgeclient.TargetMCPServer }

func resourceIdentity(name, version string) string {
	return name + "@" + version
}

func (h *consoleHandler) fetchPluginResources(r *http.Request, ctx config.Context) ([]mcp.Plugin, []mcp.PluginBinding, bool) {
	pluginResponse, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/apis/erpbridge.io/v1/plugins")
	if err != nil {
		return nil, nil, false
	}
	defer func() { _ = pluginResponse.Body.Close() }()
	if pluginResponse.StatusCode < http.StatusOK || pluginResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, false
	}
	var plugins []mcp.Plugin
	if err := jsonDecoder(pluginResponse.Body, &plugins); err != nil {
		return nil, nil, false
	}

	bindingResponse, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/apis/erpbridge.io/v1/pluginbindings")
	if err != nil {
		return nil, nil, false
	}
	defer func() { _ = bindingResponse.Body.Close() }()
	if bindingResponse.StatusCode < http.StatusOK || bindingResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, false
	}
	var bindings []mcp.PluginBinding
	if err := jsonDecoder(bindingResponse.Body, &bindings); err != nil {
		return nil, nil, false
	}
	return plugins, bindings, true
}

func jsonDecoder(body io.Reader, value any) error {
	return json.NewDecoder(body).Decode(value)
}

func stableID(primary, fallback string) string {
	value := primary
	if value == "" {
		value = fallback
	}
	return strings.NewReplacer("/", "_", " ", "_", "@", "_").Replace(value)
}

const (
	diagnosticMissingEndpoint = "The tool has no endpoint."
	diagnosticEmptyRegistry   = "No ERP APIs are registered."
	diagnosticHostMismatch    = "No registered ERP API matches the endpoint host."
	diagnosticMethodMismatch  = "Registered ERP APIs use a different method."
	diagnosticNoCandidate     = "No registered ERP API matches this endpoint."
	diagnosticAmbiguous       = "More than one registered ERP API matches this endpoint."
	matchKindUnresolved       = "unresolved"
)

func matchAPI(tool mcp.Tool, apis []idp.API, base string) (string, *idp.API, string) {
	if strings.TrimSpace(tool.Spec.Execution.Endpoint) == "" {
		return matchKindUnresolved, nil, diagnosticMissingEndpoint
	}
	if len(apis) == 0 {
		return matchKindUnresolved, nil, diagnosticEmptyRegistry
	}
	target := normalizeEndpoint(tool.Spec.Execution.Method, tool.Spec.Execution.Endpoint, base)
	if target.Path == "" {
		return matchKindUnresolved, nil, diagnosticMissingEndpoint
	}
	exact := make([]*idp.API, 0)
	prefix := make([]*idp.API, 0)
	hostMatches := false
	methodMatches := false
	for index := range apis {
		api := &apis[index]
		candidate := normalizeEndpoint(api.Method, api.URL, base)
		if candidate.Host != target.Host {
			continue
		}
		hostMatches = true
		if candidate.Method != target.Method {
			// A root API registration describes a base URL, so it can infer
			// paths for generated tools with other HTTP methods. Keep this
			// relationship non-authoritative by returning base-prefix below.
			if candidate.Path == "/" && target.Path != "/" {
				prefix = append(prefix, api)
			}
			continue
		}
		methodMatches = true
		if candidate.Path == target.Path {
			exact = append(exact, api)
			continue
		}
		if strings.HasPrefix(target.Path, strings.TrimRight(candidate.Path, "/")+"/") {
			prefix = append(prefix, api)
		}
	}
	if len(exact) == 1 {
		return matchExact, exact[0], ""
	}
	if len(exact) > 1 || len(prefix) > 1 {
		return "ambiguous", nil, diagnosticAmbiguous
	}
	if len(prefix) == 1 {
		return "base-prefix", prefix[0], ""
	}
	if !hostMatches {
		return matchKindUnresolved, nil, diagnosticHostMismatch
	}
	if !methodMatches {
		return matchKindUnresolved, nil, diagnosticMethodMismatch
	}
	return matchKindUnresolved, nil, diagnosticNoCandidate
}

func normalizeEndpoint(method, raw, base string) normalizedEndpoint {
	parsed, err := url.Parse(raw)
	if err != nil {
		return normalizedEndpoint{}
	}
	if !parsed.IsAbs() && base != "" {
		baseURL, baseErr := url.Parse(base)
		if baseErr == nil {
			parsed = baseURL.ResolveReference(parsed)
		}
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	if port != "" {
		host += ":" + port
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/"
	}
	return normalizedEndpoint{Method: strings.ToUpper(method), Host: host, Path: path}
}

func apiContextState(apiURL, erpBase string) string {
	if erpBase == "" {
		return "unassigned"
	}
	api := normalizeEndpoint("GET", apiURL, "")
	base := normalizeEndpoint("GET", erpBase, "")
	if api.Host != "" && api.Host == base.Host {
		return "context matched"
	}
	return "unassigned"
}
