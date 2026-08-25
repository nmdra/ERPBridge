package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	"github.com/nmdra/ERPBridge/internal/mcp"
)

// PluginProjection is the safe console view of an external plugin.
type PluginProjection struct {
	Name                 string `json:"name"`
	Version              string `json:"version"`
	Type                 string `json:"type,omitempty"`
	Active               bool   `json:"active"`
	EndpointConfigured   bool   `json:"endpointConfigured"`
	TimeoutMilliseconds  int    `json:"timeoutMilliseconds"`
	ConfigurationPresent bool   `json:"configurationPresent"`
}

// PluginBindingProjection is the safe console view of a plugin binding.
type PluginBindingProjection struct {
	Name                 string            `json:"name"`
	Active               bool              `json:"active"`
	PluginRef            PluginResourceRef `json:"pluginRef"`
	ToolRef              ToolResourceRef   `json:"toolRef"`
	Phase                string            `json:"phase"`
	Priority             int               `json:"priority"`
	FailurePolicy        string            `json:"failurePolicy"`
	ConfigurationPresent bool              `json:"configurationPresent"`
}

// PluginResourceRef identifies an exact plugin version without exposing its endpoint.
type PluginResourceRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolResourceRef identifies an exact MCP tool version.
type ToolResourceRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// PluginListResponse contains safe plugin projections and a feature state.
type PluginListResponse struct {
	State string             `json:"state"`
	Items []PluginProjection `json:"items"`
}

// PluginBindingListResponse contains safe binding projections and a feature state.
type PluginBindingListResponse struct {
	State string                    `json:"state"`
	Items []PluginBindingProjection `json:"items"`
}

func (h *consoleHandler) plugins(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/apis/erpbridge.io/v1/plugins")
	if err != nil {
		writeJSON(w, http.StatusOK, PluginListResponse{State: stateUnavailable, Items: []PluginProjection{}})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, PluginListResponse{State: upstreamState(response.StatusCode), Items: []PluginProjection{}})
		return
	}
	var plugins []mcp.Plugin
	if err := json.NewDecoder(response.Body).Decode(&plugins); err != nil {
		writeJSON(w, http.StatusOK, PluginListResponse{State: stateUnavailable, Items: []PluginProjection{}})
		return
	}
	items := make([]PluginProjection, 0, len(plugins))
	for _, plugin := range plugins {
		items = append(items, projectPlugin(plugin))
	}
	writeJSON(w, http.StatusOK, PluginListResponse{State: stateAvailable, Items: items})
}

func (h *consoleHandler) pluginBindings(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/apis/erpbridge.io/v1/pluginbindings")
	if err != nil {
		writeJSON(w, http.StatusOK, PluginBindingListResponse{State: stateUnavailable, Items: []PluginBindingProjection{}})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, PluginBindingListResponse{State: upstreamState(response.StatusCode), Items: []PluginBindingProjection{}})
		return
	}
	var bindings []mcp.PluginBinding
	if err := json.NewDecoder(response.Body).Decode(&bindings); err != nil {
		writeJSON(w, http.StatusOK, PluginBindingListResponse{State: stateUnavailable, Items: []PluginBindingProjection{}})
		return
	}
	items := make([]PluginBindingProjection, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, projectPluginBinding(binding))
	}
	writeJSON(w, http.StatusOK, PluginBindingListResponse{State: stateAvailable, Items: items})
}

func projectPlugin(plugin mcp.Plugin) PluginProjection {
	return PluginProjection{
		Name:                 plugin.Metadata.Name,
		Version:              plugin.Metadata.Version,
		Type:                 plugin.Metadata.Type,
		Active:               plugin.Metadata.IsActive,
		EndpointConfigured:   strings.TrimSpace(plugin.Spec.Endpoint) != "",
		TimeoutMilliseconds:  plugin.Spec.TimeoutMilliseconds,
		ConfigurationPresent: plugin.Spec.Auth != nil,
	}
}

func projectPluginBinding(binding mcp.PluginBinding) PluginBindingProjection {
	return PluginBindingProjection{
		Name:   binding.Metadata.Name,
		Active: binding.Metadata.IsActive,
		PluginRef: PluginResourceRef{
			Name:    binding.Spec.PluginRef.Name,
			Version: binding.Spec.PluginRef.Version,
		},
		ToolRef: ToolResourceRef{
			Name:    binding.Spec.ToolRef.Name,
			Version: binding.Spec.ToolRef.Version,
		},
		Phase:                binding.Spec.Phase,
		Priority:             binding.Spec.Priority,
		FailurePolicy:        binding.Spec.FailurePolicy,
		ConfigurationPresent: len(binding.Spec.Config) > 0,
	}
}
