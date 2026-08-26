package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
)

// ConfigProvider loads the current CLI configuration for a read-only snapshot.
// Implementations must not return credentials to the caller or mutate a config
// after returning it.
type ConfigProvider func() (*config.Config, error)

const (
	defaultConfigRefreshInterval = 15 * time.Second
	maxConfigRefreshInterval     = 5 * time.Minute
)

// HandlerOptions configures the local console BFF.
type HandlerOptions struct {
	Config                *config.Config
	ConfigProvider        ConfigProvider
	ConfigRefreshInterval time.Duration
	TokenOverride         string
	Assets                http.Handler
	Registry              *idp.Registry
}

type consoleHandler struct {
	config          *config.Config
	configMu        sync.RWMutex
	configRefreshMu sync.Mutex
	configProvider  ConfigProvider
	configRefreshed time.Time
	configErr       error
	refreshInterval time.Duration
	tokenOverride   string
	logStreams      chan struct{}
	metricsMu       sync.Mutex
	metricBaselines map[string]metricsBaseline
	registry        *idp.Registry
}

// NewConsoleHandler creates the read-only local BFF and frontend handler.
func NewConsoleHandler(options HandlerOptions) http.Handler {
	provider := options.ConfigProvider
	initialConfig := options.Config
	if provider != nil {
		initialConfig = cloneConfig(initialConfig)
	}
	interval := options.ConfigRefreshInterval
	if interval <= 0 {
		interval = defaultConfigRefreshInterval
	}
	if interval > maxConfigRefreshInterval {
		interval = maxConfigRefreshInterval
	}
	console := &consoleHandler{
		config:          initialConfig,
		configProvider:  provider,
		refreshInterval: interval,
		configRefreshed: time.Now().UTC(),
		tokenOverride:   options.TokenOverride,
		logStreams:      make(chan struct{}, maxLogStreams),
		metricBaselines: make(map[string]metricsBaseline),
		registry:        options.Registry,
	}
	if provider != nil {
		console.refreshConfig(true)
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
	cfg, observedAt, stale, _ := h.configSnapshot(refreshRequested(r))
	if cfg == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "config_unavailable", "configured contexts are temporarily unavailable")
		return
	}
	items := make([]ContextProjection, 0, len(cfg.Contexts))
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		if config.ValidateContextName(name) == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	current := ""
	if _, err := cfg.EffectiveContext(); err == nil {
		current = cfg.CurrentContext
	}
	for _, name := range names {
		ctx, err := cfg.ResolveContext(name)
		if err == nil {
			items = append(items, projectContext(name, ctx, name == current))
		}
	}
	writeJSON(w, http.StatusOK, ContextListResponse{Items: items, ObservedAt: observedAt, Stale: stale})
}

func (h *consoleHandler) deployment(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	name := r.URL.Query().Get("context")
	cfg, observedAt, stale, _ := h.configSnapshot(refreshRequested(r))
	if cfg == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "config_unavailable", "configured contexts are temporarily unavailable")
		return
	}
	ctx, err := cfg.ResolveContext(name)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "context_not_found", fmt.Sprintf("context %q is not configured", name))
		return
	}
	current := cfg.CurrentContext == name
	writeJSON(w, http.StatusOK, DeploymentResponse{
		Context:    projectContext(name, ctx, current),
		Console:    ConsoleState{State: "connected"},
		ObservedAt: observedAt,
		Stale:      stale,
	})
}

func (h *consoleHandler) context(name string) (config.Context, bool) {
	cfg, _, _, _ := h.configSnapshot(false)
	if cfg == nil {
		return config.Context{}, false
	}
	ctx, err := cfg.ResolveContext(name)
	return ctx, err == nil
}

func (h *consoleHandler) configSnapshot(force bool) (*config.Config, time.Time, bool, error) {
	h.refreshConfig(force)
	h.configMu.RLock()
	defer h.configMu.RUnlock()
	return h.config, h.configRefreshed, h.configErr != nil, h.configErr
}

func (h *consoleHandler) refreshConfig(force bool) {
	if h.configProvider == nil {
		return
	}
	now := time.Now().UTC()
	h.configMu.RLock()
	due := h.config == nil || now.Sub(h.configRefreshed) >= h.refreshInterval
	h.configMu.RUnlock()
	if !force && !due {
		return
	}
	h.configRefreshMu.Lock()
	defer h.configRefreshMu.Unlock()
	now = time.Now().UTC()
	h.configMu.RLock()
	due = h.config == nil || now.Sub(h.configRefreshed) >= h.refreshInterval
	h.configMu.RUnlock()
	if !force && !due {
		return
	}
	loaded, err := h.configProvider()
	if err == nil && loaded != nil {
		if _, err = loaded.EffectiveContext(); err == nil {
			loaded = cloneConfig(loaded)
		}
	} else if err == nil {
		err = fmt.Errorf("configuration is nil")
	}
	h.configMu.Lock()
	defer h.configMu.Unlock()
	h.configRefreshed = now
	if err == nil {
		h.config = loaded
		h.configErr = nil
		return
	}
	// A failed reload must not remove the last known-good snapshot. The error
	// is retained only as an internal stale marker.
	h.configErr = err
}

func refreshRequested(r *http.Request) bool {
	value := r.URL.Query().Get("refresh")
	return value == "1" || value == "true"
}

func cloneConfig(source *config.Config) *config.Config {
	if source == nil {
		return nil
	}
	clone := &config.Config{CurrentContext: source.CurrentContext, Contexts: make(map[string]config.Context, len(source.Contexts))}
	for name, ctx := range source.Contexts {
		clone.Contexts[name] = ctx
	}
	return clone
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
