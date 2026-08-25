package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
)

// HandlerOptions configures the local console BFF.
type HandlerOptions struct {
	Config        *config.Config
	TokenOverride string
	Assets        http.Handler
	Registry      *idp.Registry
}

type consoleHandler struct {
	config          *config.Config
	tokenOverride   string
	logStreams      chan struct{}
	metricsMu       sync.Mutex
	metricBaselines map[string]metricsBaseline
	registry        *idp.Registry
}

// NewConsoleHandler creates the read-only local BFF and frontend handler.
func NewConsoleHandler(options HandlerOptions) http.Handler {
	console := &consoleHandler{
		config:          options.Config,
		tokenOverride:   options.TokenOverride,
		logStreams:      make(chan struct{}, maxLogStreams),
		metricBaselines: make(map[string]metricsBaseline),
		registry:        options.Registry,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/console/v1/contexts", console.contexts)
	mux.HandleFunc("/api/console/v1/deployment", console.deployment)
	mux.HandleFunc("/api/console/v1/health", console.health)
	mux.HandleFunc("/api/console/v1/tools", console.tools)
	mux.HandleFunc("/api/console/v1/plugins", console.plugins)
	mux.HandleFunc("/api/console/v1/plugin-bindings", console.pluginBindings)
	mux.HandleFunc("/api/console/v1/cache", console.cache)
	mux.HandleFunc("/api/console/v1/logs/recent", console.logsRecent)
	mux.HandleFunc("/api/console/v1/logs/stream", console.logsStream)
	mux.HandleFunc("/api/console/v1/metrics", console.metricsSnapshot)
	mux.HandleFunc("/api/console/v1/topology", console.topology)
	mux.HandleFunc("/api/console/v1/server-info", console.serverInfo)
	mux.HandleFunc("/api/console/", func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(w, http.StatusNotFound, "route_not_found", "console route not found")
	})
	assets := options.Assets
	if assets == nil {
		assets = NewAssetHandler()
	}
	mux.Handle("/", assets)
	return mux
}

func (h *consoleHandler) contexts(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	items := make([]ContextProjection, 0)
	if h.config != nil {
		names := make([]string, 0, len(h.config.Contexts))
		for name := range h.config.Contexts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			items = append(items, projectContext(name, h.config.Contexts[name], name == h.config.CurrentContext))
		}
	}
	writeJSON(w, http.StatusOK, ContextListResponse{Items: items})
}

func (h *consoleHandler) deployment(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	name := r.URL.Query().Get("context")
	ctx, ok := h.context(name)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "context_not_found", fmt.Sprintf("context %q is not configured", name))
		return
	}
	writeJSON(w, http.StatusOK, DeploymentResponse{
		Context: projectContext(name, ctx, h.config != nil && h.config.CurrentContext == name),
		Console: ConsoleState{State: "connected"},
	})
}

func (h *consoleHandler) context(name string) (config.Context, bool) {
	if h.config == nil || name == "" {
		return config.Context{}, false
	}
	ctx, ok := h.config.Contexts[name]
	return ctx, ok
}

func projectContext(name string, ctx config.Context, current bool) ContextProjection {
	return ContextProjection{
		Name:              name,
		ServerIdentity:    endpointIdentity(ctx.Server, "server"),
		MCPServerIdentity: endpointIdentity(ctx.MCPServer, "mcp-server"),
		ServerState:       endpointState(ctx.Server),
		MCPServerState:    endpointState(ctx.MCPServer),
		Current:           current,
	}
}

func endpointIdentity(raw, label string) string {
	if endpointState(raw) == "configured" {
		return label
	}
	return ""
}

func endpointState(raw string) string {
	if raw == "" {
		return "unconfigured"
	}
	if err := bridgeclient.ValidateServerURL(raw); err != nil {
		return "invalid"
	}
	return "configured"
}

func onlyGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "the console route is read-only")
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIErrorResponse{Error: code, Message: message})
}
