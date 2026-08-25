package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

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
	State string         `json:"state"`
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// TopologyNode represents a transport, tool, API, plugin, binding, or unresolved endpoint.
type TopologyNode struct {
	ID           string                   `json:"id"`
	Kind         string                   `json:"kind"`
	Label        string                   `json:"label"`
	ContextState string                   `json:"contextState,omitempty"`
	Tool         *ToolProjection          `json:"tool,omitempty"`
	API          *TopologyAPIDetails      `json:"api,omitempty"`
	Plugin       *PluginProjection        `json:"plugin,omitempty"`
	Binding      *PluginBindingProjection `json:"binding,omitempty"`
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
	ID            string `json:"id"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	MatchKind     string `json:"matchKind"`
	ContextState  string `json:"contextState,omitempty"`
	Authoritative bool   `json:"authoritative"`
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
	ctxName := r.URL.Query().Get("context")
	ctx, ok := h.context(ctxName)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "context_not_found", "the selected context is not configured")
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

	registry := h.registry
	if registry == nil {
		registry, err = idp.NewRegistry("", slog.Default())
		if err != nil {
			writeJSON(w, http.StatusOK, TopologyResponse{State: stateUnavailable, Nodes: []TopologyNode{}, Edges: []TopologyEdge{}})
			return
		}
	}
	apis := registry.List()
	sort.Slice(apis, func(i, j int) bool { return apis[i].Name < apis[j].Name })

	graph := TopologyResponse{State: stateAvailable, Nodes: make([]TopologyNode, 0), Edges: make([]TopologyEdge, 0)}
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
		kind, matched := matchAPI(tool, apis, ctx.ERPBase)
		if matched != nil {
			apiID := "api:" + stableID(matched.ID, matched.Name)
			if len(graph.Edges) < maxTopologyEdges {
				graph.Edges = append(graph.Edges, TopologyEdge{ID: "edge:" + toolID + ":" + apiID, Source: toolID, Target: apiID, MatchKind: kind, ContextState: apiContextState(matched.URL, ctx.ERPBase), Authoritative: kind == matchExact})
			}
		} else {
			unresolvedID := "unresolved:" + stableID(tool.Metadata.Name, tool.Metadata.Version)
			if len(graph.Nodes) < maxTopologyNodes {
				graph.Nodes = append(graph.Nodes, TopologyNode{ID: unresolvedID, Kind: "unresolved-endpoint", Label: projection.EndpointPath})
			}
			if len(graph.Edges) < maxTopologyEdges {
				graph.Edges = append(graph.Edges, TopologyEdge{ID: "edge:" + toolID + ":" + unresolvedID, Source: toolID, Target: unresolvedID, MatchKind: kind, Authoritative: false})
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
	writeJSON(w, http.StatusOK, graph)
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

func matchAPI(tool mcp.Tool, apis []idp.API, base string) (string, *idp.API) {
	target := normalizeEndpoint(tool.Spec.Execution.Method, tool.Spec.Execution.Endpoint, base)
	if target.Path == "" {
		return "unresolved", nil
	}
	exact := make([]*idp.API, 0)
	prefix := make([]*idp.API, 0)
	for index := range apis {
		api := &apis[index]
		candidate := normalizeEndpoint(api.Method, api.URL, base)
		if candidate.Host != target.Host {
			continue
		}
		if candidate.Method != target.Method {
			// A root API registration describes a base URL, so it can infer
			// paths for generated tools with other HTTP methods. Keep this
			// relationship non-authoritative by returning base-prefix below.
			if candidate.Path == "/" && target.Path != "/" {
				prefix = append(prefix, api)
			}
			continue
		}
		if candidate.Path == target.Path {
			exact = append(exact, api)
			continue
		}
		if strings.HasPrefix(target.Path, strings.TrimRight(candidate.Path, "/")+"/") {
			prefix = append(prefix, api)
		}
	}
	if len(exact) == 1 {
		return matchExact, exact[0]
	}
	if len(exact) > 1 || len(prefix) > 1 {
		return "ambiguous", nil
	}
	if len(prefix) == 1 {
		return "base-prefix", prefix[0]
	}
	return "unresolved", nil
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
