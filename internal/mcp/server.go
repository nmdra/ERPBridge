// Package mcp implements the Model Context Protocol (MCP) server,
// allowing AI agents to interact with ERP systems through tools,
// resources, and prompts.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/credentials"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/metrics"
)

// Server is the primary MCP server implementation for ERPBridge.
type Server struct {
	mcpServer           *server.MCPServer
	connector           ERPConnector
	cache               *cache.Manager
	log                 *slog.Logger
	mu                  sync.RWMutex
	pluginLifecycleMu   sync.RWMutex
	store               *Store
	registry            *ToolRegistry
	pluginRegistry      *PluginRegistry
	pluginClient        PluginProcessor
	lifecycleGeneration uint64
	lastDesiredHash     string
	resources           map[string]*Resource
	prompts             map[string]*Prompt
	Notifier            *CustomNotifier
	TelemetryHooks      *TelemetryHooks
	BusinessHooks       *BusinessHooks
	toolMiddlewares     []server.ToolHandlerMiddleware
	authWarnOnce        sync.Once
	serverInfo          ServerInfo
}

const (
	statusKey           = "status"
	textContentType     = "text"
	toolNameQueryParam  = "name"
	testToolsEnv        = "MCP_ENABLE_TEST_TOOLS"
	dataClassPublic     = "public"
	dataClassInternal   = "internal"
	dataClassPII        = "pii"
	dataClassRestricted = "restricted"
)

// RateLimitConfig defines the configuration for the tool rate limiter.
type RateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
}

// Validate checks that rate limiting can make progress without accepting
// values that would disable or destabilize the limiter.
func (c RateLimitConfig) Validate() error {
	if c.RequestsPerSecond <= 0 || math.IsNaN(c.RequestsPerSecond) || math.IsInf(c.RequestsPerSecond, 0) {
		return errors.New("rate limit requests per second must be positive and finite")
	}
	if c.Burst <= 0 {
		return errors.New("rate limit burst must be positive")
	}
	return nil
}

// NewServer creates a new Server instance with the provided connector, cache manager, and logger.
func NewServer(connector ERPConnector, cacheMgr *cache.Manager, rootLog *slog.Logger, rateCfg RateLimitConfig, dbPath string) *Server {
	var bridgeServer *Server
	s := server.NewMCPServer("ERPBridge", "1.0.0",
		server.WithLogging(),
		server.WithResourceCompletionProvider(&ResourceCompletionProvider{}),
		server.WithPromptCompletionProvider(&PromptCompletionProvider{}),
		server.WithOutputSchemaValidation(),
		server.WithHooks(&server.Hooks{}),
		server.WithToolFilter(func(_ context.Context, tools []mcp.Tool) []mcp.Tool {
			if bridgeServer == nil {
				return tools
			}
			return bridgeServer.filterToolsList(tools)
		}),
	)

	mcpHandler := logger.NewMCPHandler(s, "mcp")
	mcpLog := slog.New(logger.MultiHandler{
		logger.Component(rootLog, "mcp").Handler(),
		mcpHandler,
	})

	store, err := NewStore(dbPath)
	if err != nil {
		mcpLog.Error("failed to initialize store", slog.String("error", err.Error()))
	}

	srv := &Server{
		mcpServer:      s,
		connector:      connector,
		cache:          cacheMgr,
		log:            mcpLog,
		store:          store,
		registry:       NewToolRegistry(),
		pluginRegistry: NewPluginRegistry(),
		pluginClient:   NewPluginClientWithLogger(mcpLog),
		resources:      make(map[string]*Resource),
		prompts:        make(map[string]*Prompt),
		Notifier:       NewCustomNotifier(s),
	}
	bridgeServer = srv

	srv.TelemetryHooks = NewTelemetryHooks(mcpLog)
	srv.TelemetryHooks.Register(s)

	srv.BusinessHooks = NewBusinessHooks(srv.Notifier, mcpLog)
	srv.BusinessHooks.Register(s)
	srv.warnUnresolvedCredentials()

	// Initialize global tool middlewares
	rateLimiter := NewRateLimitMiddleware(rateCfg.RequestsPerSecond, rateCfg.Burst)
	srv.toolMiddlewares = []server.ToolHandlerMiddleware{
		rateLimiter.Handle(),
		LoggingMiddleware(srv.log),
		MetricsMiddleware(),
	}

	srv.RegisterBuiltinTools()

	return srv
}

// RegisterBuiltinTools registers internal system tools using structured handlers.
func (s *Server) RegisterBuiltinTools() {
	if !testToolsEnabled() {
		return
	}

	// system.progress_test
	type ProgressTestInput struct {
		Steps int `json:"steps" jsonschema:"description=Number of steps to simulate (max 100),default=10"`
	}

	progressTool := mcp.NewTool("system.progress_test",
		mcp.WithDescription("A demonstration tool that sends real-time progress notifications."),
		mcp.WithInputSchema[ProgressTestInput](),
		mcp.WithToolIcons(mcp.Icon{
			Src:      "https://erpbridge.io/icons/progress.png",
			MIMEType: "image/png",
		}),
	)

	progressHandler := mcp.NewStructuredToolHandler(func(ctx context.Context, _ mcp.CallToolRequest, input ProgressTestInput) (*mcp.CallToolResult, error) {
		steps := min(input.Steps, 100)

		for i := 1; i <= steps; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
				s.Notifier.SendProgress(ctx, i, steps, fmt.Sprintf("Processing step %d/%d...", i, steps))
			}
		}

		return mcp.NewToolResultText(fmt.Sprintf("Finished %d steps successfully.", steps)), nil
	})

	// Apply global middlewares
	for _, v := range slices.Backward(s.toolMiddlewares) {
		progressHandler = v(progressHandler)
	}

	s.mcpServer.AddTool(progressTool, progressHandler)
	metrics.InitializeToolMetrics(progressTool.Name)

	// system.sensitive_log_test
	type SensitiveLogTestInput struct {
		Token   string `json:"token" jsonschema:"description=A sensitive token that should be redacted"`
		Message string `json:"message" jsonschema:"description=A normal message"`
	}

	sensitiveLogTool := mcp.NewTool("system.sensitive_log_test",
		mcp.WithDescription("A demonstration tool that logs sensitive data to verify redaction."),
		mcp.WithInputSchema[SensitiveLogTestInput](),
		mcp.WithToolIcons(mcp.Icon{
			Src:      "https://erpbridge.io/icons/security.png",
			MIMEType: "image/png",
		}),
	)

	sensitiveLogHandler := mcp.NewStructuredToolHandler(func(ctx context.Context, _ mcp.CallToolRequest, input SensitiveLogTestInput) (*mcp.CallToolResult, error) {
		// Log using the composite logger which includes MCPHandler
		s.log.InfoContext(ctx, "Sensitive data received",
			slog.String("token", input.Token),
			slog.String("message", input.Message),
		)

		return mcp.NewToolResultText("Logs emitted. Check your MCP client logs."), nil
	})

	// Apply global middlewares
	for _, v := range slices.Backward(s.toolMiddlewares) {
		sensitiveLogHandler = v(sensitiveLogHandler)
	}

	s.mcpServer.AddTool(sensitiveLogTool, sensitiveLogHandler)
	metrics.InitializeToolMetrics(sensitiveLogTool.Name)
}

// ResourceCompletionProvider implements the mcp-go completion provider for resources.
type ResourceCompletionProvider struct{}

// CompleteResourceArgument provides suggestions for resource URIs.
func (p *ResourceCompletionProvider) CompleteResourceArgument(_ context.Context, _ string, _ mcp.CompleteArgument, _ mcp.CompleteContext) (*mcp.Completion, error) {
	return &mcp.Completion{
		Values: []string{"recent-item-1", "recent-item-2"},
	}, nil
}

// PromptCompletionProvider implements the mcp-go completion provider for prompts.
type PromptCompletionProvider struct{}

// CompletePromptArgument provides suggestions for prompt arguments.
func (p *PromptCompletionProvider) CompletePromptArgument(_ context.Context, _ string, _ mcp.CompleteArgument, _ mcp.CompleteContext) (*mcp.Completion, error) {
	return &mcp.Completion{
		Values: []string{"suggested-value-A", "suggested-value-B"},
	}, nil
}

// RegisterResource adds a resource to the server.
func (s *Server) RegisterResource(r *Resource) {
	s.mu.Lock()
	s.resources[r.URITemplate] = r
	s.mu.Unlock()

	mcpResource := mcp.NewResource(r.URITemplate, r.Name,
		mcp.WithResourceDescription(r.Description),
		mcp.WithMIMEType(r.MimeType),
	)
	s.mcpServer.AddResource(mcpResource, s.handleMCPResourceRead)
	s.log.Info("registered MCP resource", slog.String("name", r.Name), slog.String("uri", r.URITemplate))
	s.warnUnresolvedCredentials()
}

func (s *Server) warnUnresolvedCredentials() {
	s.mu.RLock()
	resources := make([]*Resource, 0, len(s.resources))
	for _, resource := range s.resources {
		resources = append(resources, resource)
	}
	s.mu.RUnlock()

	if s.store != nil {
		tools, err := s.store.List()
		if err != nil {
			s.log.Warn("could not inspect tool credentials", slog.String("error", err.Error()))
		} else {
			for _, tool := range tools {
				if !credentialConfigured(tool.Spec.Security.CredentialRef, tool.Spec.Security.CredentialSource) {
					s.log.Warn("tool credential is unresolved", slog.String("tool_name", tool.Metadata.Name), slog.String("credential_source", string(tool.Spec.Security.CredentialSource)))
				}
			}
		}
	}

	for _, resource := range resources {
		if !credentialConfigured(resource.Security.CredentialRef, resource.Security.CredentialSource) {
			s.log.Warn("resource credential is unresolved", slog.String("resource_name", resource.Name), slog.String("credential_source", string(resource.Security.CredentialSource)))
		}
	}
}

func credentialConfigured(ref string, source credentials.CredentialSource) bool {
	if ref == "" {
		return true
	}
	if err := credentials.ValidateCredentialSource(source); err != nil {
		return false
	}
	if credentials.IsFileBacked(source) {
		dir, ok := os.LookupEnv(credentials.CredentialsDirEnv)
		return ok && dir != ""
	}
	value, ok := os.LookupEnv(ref)
	return ok && value != ""
}

// RegisterPrompt adds a prompt template to the server.
func (s *Server) RegisterPrompt(p *Prompt) {
	s.mu.Lock()
	s.prompts[p.Name] = p
	s.mu.Unlock()
	mcpPrompt := mcp.NewPrompt(p.Name,
		mcp.WithPromptDescription(p.Description),
	)
	for _, a := range p.Arguments {
		mcpPrompt.Arguments = append(mcpPrompt.Arguments, mcp.PromptArgument{
			Name:        a.Name,
			Description: a.Description,
			Required:    a.Required,
		})
	}
	s.mcpServer.AddPrompt(mcpPrompt, s.handleMCPPromptGet)
	s.log.Info("registered MCP prompt", slog.String("name", p.Name))
}

func (s *Server) handleMCPResourceRead(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	// Find resource by URI matching (simplistic for template)
	// In a real implementation, we'd use a regex or template matcher
	s.mu.RLock()
	r, ok := s.resources[request.Params.URI]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("resource not found: %s", request.Params.URI)
	}

	content, err := r.Execute(ctx, request.Params.URI, s.connector)
	if err != nil {
		return nil, err
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: r.MimeType,
			Text:     content,
		},
	}, nil
}

func (s *Server) handleMCPPromptGet(_ context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	s.mu.RLock()
	p, ok := s.prompts[request.Params.Name]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("prompt not found: %s", request.Params.Name)
	}

	// Simple template expansion (naive)
	text := p.Template
	if request.Params.Arguments != nil {
		for k, v := range request.Params.Arguments {
			text = fmt.Sprintf("%s\n\n%s: %v", text, k, v)
		}
	}

	return &mcp.GetPromptResult{
		Description: p.Description,
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: textContentType,
					Text: text,
				},
			},
		},
	}, nil
}

// StartController runs the background reconciliation loop.
func (s *Server) StartController(ctx context.Context) {
	s.log.Info("starting reconciliation controller")
	s.Reconcile(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Reconcile(ctx)
		}
	}
}

// Reconcile ensures the MCP runtime matches the desired state in the SQLite store.
func (s *Server) Reconcile(ctx context.Context) {
	s.pluginLifecycleMu.Lock()
	defer s.pluginLifecycleMu.Unlock()
	_ = s.reconcileLocked(ctx)
}

func (s *Server) reconcileLocked(ctx context.Context) error {
	if s.store == nil {
		return nil
	}

	// Check if state has changed
	currentHash, err := s.store.GetStateHash()
	if err != nil {
		s.log.Error("failed to get state hash", slog.String("error", err.Error()))
		return fmt.Errorf("get state hash: %w", err)
	}

	s.mu.RLock()
	lastHash := s.lastDesiredHash
	s.mu.RUnlock()

	if currentHash == lastHash {
		return nil
	}

	s.log.Info("reconciling tools", slog.String("old_hash", lastHash), slog.String("new_hash", currentHash))

	desiredTools, err := s.store.List()
	if err != nil {
		s.log.Error("failed to list desired tools", slog.String("error", err.Error()))
		return fmt.Errorf("list desired tools: %w", err)
	}
	desiredPlugins, err := s.store.ListPlugins()
	if err != nil {
		s.log.Error("failed to list desired plugins", slog.String("error", err.Error()))
		return fmt.Errorf("list desired plugins: %w", err)
	}
	desiredBindings, err := s.store.ListPluginBindings()
	if err != nil {
		s.log.Error("failed to list desired plugin bindings", slog.String("error", err.Error()))
		return fmt.Errorf("list desired plugin bindings: %w", err)
	}
	if s.pluginRegistry == nil {
		s.pluginRegistry = NewPluginRegistry()
	}
	s.lifecycleGeneration++
	oldBindings := s.pluginRegistry.SnapshotBindings()
	nextSnapshot := buildPluginBindingSnapshot(desiredPlugins, desiredBindings, desiredTools)
	var nextBindings []*PluginBinding
	for _, values := range nextSnapshot {
		for _, value := range values {
			if value != nil {
				nextBindings = append(nextBindings, value.Binding)
			}
		}
	}
	if err := s.flushPluginBindingTargets(ctx, oldBindings, nextBindings); err != nil {
		s.log.Error("failed to invalidate plugin binding caches", slog.String("error", err.Error()))
		return fmt.Errorf("invalidate plugin binding caches: %w", err)
	}
	s.pluginRegistry.Replace(nextSnapshot)
	s.warnUnresolvedCredentials()

	desiredMap := make(map[string]bool)
	for _, dt := range desiredTools {
		if !dt.Metadata.IsActive {
			continue
		}
		key := fmt.Sprintf("%s@%s", dt.Metadata.Name, dt.Metadata.Version)
		desiredMap[key] = true

		// Existing logic: Register any tool that is in the store but not in registry
		existing, err := s.registry.Resolve(dt.Metadata.Name, dt.Metadata.Version)
		if err != nil || existing == nil {
			s.log.Info("reconciling tool (adding/updating)", slog.String("name", dt.Metadata.Name), slog.String("version", dt.Metadata.Version))
			s.RegisterTool(dt)
		}
	}

	// Deletion Reconciliation: Identify tools in actual state that are missing from desired state
	actualTools := s.registry.ListActive() // Use ListActive to find currently active tools
	for _, at := range actualTools {
		// Builtin system tools are not managed by SQLite
		if at.Metadata.Module == "system" {
			continue
		}

		key := fmt.Sprintf("%s@%s", at.Metadata.Name, at.Metadata.Version)
		if !desiredMap[key] {
			s.log.Info("reconciling tool (deactivating stale)", slog.String("name", at.Metadata.Name), slog.String("version", at.Metadata.Version))
			s.DeregisterTool(at.Metadata.Name, at.Metadata.Version)
		}
	}

	s.mu.Lock()
	s.lastDesiredHash = currentHash
	s.mu.Unlock()
	return nil
}

// DeregisterTool removes a tool from the server's registry and active MCP server.
func (s *Server) DeregisterTool(name, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.registry.Remove(name, version)

	s.log.Info("tool removed from active registry", slog.String("tool_name", name), slog.String("version", version))

	// Notify clients that tools have changed
	s.mcpServer.SendNotificationToAllClients("notifications/tools/list_changed", nil)

	// Notify clients specifically about this deletion
	if s.Notifier != nil {
		s.Notifier.SendToolDeleted(name, version)
	}
}

func (s *Server) filterToolsList(tools []mcp.Tool) []mcp.Tool {
	filtered := make([]mcp.Tool, 0, len(tools))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, tool := range tools {
		if strings.HasPrefix(tool.Name, "system.") {
			filtered = append(filtered, tool)
			continue
		}
		entry, err := s.registry.Resolve(tool.Name, "")
		if err == nil && entry.Metadata.IsActive {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// FilterToolsList exposes the active-tool filter to the Stdio transport adapter.
func (s *Server) FilterToolsList(tools []mcp.Tool) []mcp.Tool {
	return s.filterToolsList(tools)
}

// RegisterTool adds a tool to the server's registry and active MCP server.
func (s *Server) RegisterTool(t *Tool) {
	if err := s.validateTool(t); err != nil {
		if s.log != nil {
			s.log.Error("rejected invalid tool", slog.String("tool_name", t.Metadata.Name), slog.String("error", err.Error()))
		}
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t.Metadata.IsActive = true // Ensure it is marked as active when registered
	if err := s.registry.Add(t); err != nil {
		s.log.Error("failed to add tool to registry", slog.String("tool", t.Metadata.Name), slog.String("error", err.Error()))
		return
	}

	// Serialize the input schema to JSON.RawMessage
	inputSchema, err := schemaForMCP(t)
	if err != nil {
		s.log.Error("failed to prepare input schema", slog.String("tool_name", t.Metadata.Name), slog.String("error", err.Error()))
		return
	}
	schemaJSON, err := json.Marshal(inputSchema)
	if err != nil {
		s.log.Error("failed to marshal input schema", slog.String("tool_name", t.Metadata.Name), slog.String("error", err.Error()))
		return
	}

	// Use versioned name internally for MCP registration if it's not the default?
	// Actually, the spec says we should use stable aliases.
	// For now, we register with the base name and let the handler resolve.
	mcpTool := mcp.NewToolWithRawSchema(t.Metadata.Name, t.Spec.Description.Short, json.RawMessage(schemaJSON))

	// Explicitly clear structured input fields because RawInputSchema is used.
	mcpTool.InputSchema = mcp.ToolInputSchema{}
	if t.Spec.OutputSchema != nil {
		outputSchemaJSON, err := json.Marshal(*t.Spec.OutputSchema)
		if err != nil {
			s.log.Error("failed to prepare output schema", slog.String("tool_name", t.Metadata.Name), slog.String("error", err.Error()))
			return
		}
		mcpTool.RawOutputSchema = json.RawMessage(outputSchemaJSON)
	}

	// Add tool to server with the shared middleware and execution seam.
	handler := s.applyToolMiddlewares(t, s.handleMCPToolCall(t.Metadata.Name))

	s.mcpServer.AddTool(mcpTool, handler)
	metrics.InitializeToolMetrics(t.Metadata.Name)
	s.log.Info("registered MCP tool", slog.String("tool_name", t.Metadata.Name), slog.String("version", t.Metadata.Version))

	// Notify clients that tools have changed
	s.mcpServer.SendNotificationToAllClients("notifications/tools/list_changed", nil)
}

func (s *Server) handleMCPToolCall(name string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		s.mu.RLock()
		// Resolve the latest stable version for this tool name.
		t, err := s.registry.Resolve(name, "")
		s.mu.RUnlock()

		if err != nil {
			return nil, fmt.Errorf("tool not found: %s (%w)", name, err)
		}
		args, err := toolArguments(request)
		if err != nil {
			return nil, err
		}
		result, err := s.executeToolCall(ctx, t, args)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result, nil
	}
}

func (s *Server) executeToolRequest(ctx context.Context, tool *Tool, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := toolArguments(request)
	if err != nil {
		return nil, err
	}
	return s.executeToolCall(ctx, tool, args)
}

func toolArguments(request mcp.CallToolRequest) (map[string]any, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok && request.Params.Arguments != nil {
		return nil, fmt.Errorf("invalid arguments format")
	}
	return args, nil
}

func (s *Server) applyToolMiddlewares(tool *Tool, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	handler = s.CacheMiddleware(tool)(handler)
	handler = s.RoleAuthzMiddleware(tool)(handler)
	for _, middleware := range slices.Backward(s.toolMiddlewares) {
		handler = middleware(handler)
	}
	return handler
}

// MCPServer returns the underlying mcp-go MCPServer instance.
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcpServer
}

// ServeHTTP handles the various MCP transports and management endpoints.
func (s *Server) ServeHTTP(mux *http.ServeMux, _ string) {
	corsOrigins := configuredCORSOrigins()
	// 1. Streamable HTTP Transport (Modern clients, Postman)
	// MUST strip prefix so the server sees "/" internally
	streamableServer := server.NewStreamableHTTPServer(s.mcpServer,
		server.WithStateful(true),
		server.WithSessionIdleTTL(30*time.Minute),
		server.WithEndpointPath("/"), // Tell the server it is mounted at /
		server.WithStreamableHTTPCORS(
			server.WithCORSAllowedOrigins(corsOrigins...),
			server.WithCORSAllowedMethods("POST", "GET", "OPTIONS"),
			server.WithCORSAllowedHeaders("Content-Type", "Mcp-Session-Id", "Last-Event-ID", "Authorization", "MCP-Protocol-Version"),
			server.WithCORSExposedHeaders("Mcp-Session-Id"),
		),
	)

	// Wrap streamableServer with filtering middleware to hide inactive tools
	filteredStreamable := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only intercept JSON-RPC POST requests
		if r.Method != http.MethodPost {
			streamableServer.ServeHTTP(w, r)
			return
		}

		// Use a buffer to capture the response
		buf := &bytes.Buffer{}
		iw := &interceptingResponseWriter{
			ResponseWriter: w,
			body:           buf,
		}

		streamableServer.ServeHTTP(iw, r)

		// mcp-go uses SSE format even in POST responses for streamable HTTP.
		// We need to process each line and filter any 'data: ' blocks that contain tool lists.
		lines := bytes.Split(buf.Bytes(), []byte("\n"))
		var finalBody bytes.Buffer

		for _, line := range lines {
			if after, ok := bytes.CutPrefix(line, []byte("data: ")); ok {
				jsonData := after

				// Try to parse the JSON to see if it's a tools/list result
				var jsonResp struct {
					JSONRPC string `json:"jsonrpc"`
					ID      any    `json:"id"`
					Result  struct {
						Tools []mcp.Tool `json:"tools"`
					} `json:"result"`
				}

				if err := json.Unmarshal(jsonData, &jsonResp); err == nil && len(jsonResp.Result.Tools) > 0 {
					filteredTools := s.filterToolsList(jsonResp.Result.Tools)

					// Update the result and re-marshal
					jsonResp.Result.Tools = filteredTools
					newJSON, _ := json.Marshal(jsonResp)
					finalBody.Write([]byte("data: "))
					finalBody.Write(newJSON)
					finalBody.Write([]byte("\n"))
					continue
				}
			}
			// Not a tool list or not a data line, keep as is
			finalBody.Write(line)
			finalBody.Write([]byte("\n"))
		}

		// Update Content-Length header if it was set
		if w.Header().Get("Content-Length") != "" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", finalBody.Len()))
		}
		_, _ = w.Write(finalBody.Bytes())
	})

	mcpHandler := http.StripPrefix("/mcp", filteredStreamable)
	authenticatedMCP := s.AuthHandler(mcpHandler, scopeMCP, false)
	mux.Handle("/mcp/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAllowedMCPPreflight(r) {
			mcpHandler.ServeHTTP(w, r)
			return
		}
		authenticatedMCP.ServeHTTP(w, r)
	}))

	// 3. Management & Utility Endpoints
	mux.HandleFunc("/mcp/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.Handle("/api/auth/tokens", http.HandlerFunc(s.handleTokenAPI))
	mux.Handle("/api/auth/tokens/", http.HandlerFunc(s.handleTokenAPI))
	mux.Handle("/api/tools/invoke", s.AuthHandler(http.HandlerFunc(s.handleDirectInvoke), "", true))
	mux.Handle("/api/cache/stats", s.AuthHandler(http.HandlerFunc(s.handleCacheStats), "", true))
	mux.Handle("/api/info", s.AuthHandler(http.HandlerFunc(s.handleInfo), "", true))
	mux.Handle("/api/cache/flush", s.AuthHandler(http.HandlerFunc(s.handleCacheFlush), "", true))
	mux.Handle("/api/logs/stream", s.AuthHandler(http.HandlerFunc(s.handleLogStream), "logs", false))
	mux.Handle("/api/logs/recent", s.AuthHandler(http.HandlerFunc(s.handleLogRecent), "logs", false))

	// 4. Kubernetes-Style Tool API
	mux.Handle("/apis/erpbridge.io/v1/tools", s.AuthHandler(http.HandlerFunc(s.handleToolAPI), "", true))
	apiProbeHandler := s.AuthHandler(http.HandlerFunc(s.handleAPIProbe), "", true)
	mux.Handle(apiProbePath, apiProbeHandler)
	// Keep the versioned resource namespace available for clients that group
	// all declarative control-plane operations below /apis/erpbridge.io/v1.
	mux.Handle(apiProbeResourcePath, apiProbeHandler)
	mux.Handle("/apis/erpbridge.io/v1/plugins", s.AuthHandler(http.HandlerFunc(s.handlePluginAPI), "", true))
	mux.Handle("/apis/erpbridge.io/v1/pluginbindings", s.AuthHandler(http.HandlerFunc(s.handlePluginBindingAPI), "", true))
}

func (s *Server) handleToolAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleToolApply(w, r)
	case http.MethodGet:
		s.handleToolList(w, r)
	case http.MethodDelete:
		s.handleToolDelete(w, r)
	default:
		writeControlPlaneError(w, http.StatusMethodNotAllowed, ErrorMethodNotAllowed, "the requested control-plane method is not supported", "use GET, POST, or DELETE for this resource")
	}
}

func (s *Server) handleToolApply(w http.ResponseWriter, r *http.Request) {
	var t Tool
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeControlPlaneError(w, http.StatusBadRequest, ErrorValidationFailed, "the tool resource is not valid JSON", "submit one valid tool resource")
		return
	}

	// Admission Controller
	if err := s.validateTool(&t); err != nil {
		writeControlPlaneError(w, http.StatusUnprocessableEntity, ErrorValidationFailed, "the tool resource failed validation", "review the tool metadata, execution, and security fields")
		return
	}

	if s.store == nil {
		writeControlPlaneError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "store not available", "check ERPBridge storage configuration")
		return
	}
	s.pluginLifecycleMu.Lock()
	defer s.pluginLifecycleMu.Unlock()

	affected, err := s.store.ListPluginBindingsForTool(t.Metadata.Name, t.Metadata.Version)
	if err != nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage and retry")
		return
	}
	if err := s.flushPluginBindingTargets(r.Context(), affected); err != nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check cache health and retry")
		return
	}

	t.Metadata.IsActive = true // Mark as active before saving

	if err := s.store.Save(&t); err != nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage and retry")
		return
	}

	// Immediate reconciliation for responsiveness
	s.RegisterTool(&t)
	if err := s.reconcileLocked(r.Context()); err != nil {
		writeReconciliationPending(w)
		return
	}
	s.warnUnresolvedCredentials()

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		statusKey:          "applied",
		toolNameQueryParam: t.Metadata.Name,
		"version":          t.Metadata.Version,
	})
}

func (s *Server) handleToolList(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage configuration")
		return
	}
	tools, err := s.store.List()
	if err != nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage and retry")
		return
	}
	nameFilter := r.URL.Query().Get(toolNameQueryParam)
	versionFilter := r.URL.Query().Get("version")
	if nameFilter != "" || versionFilter != "" {
		filtered := make([]*Tool, 0, len(tools))
		for _, tool := range tools {
			if nameFilter != "" && tool.Metadata.Name != nameFilter {
				continue
			}
			if versionFilter != "" && tool.Metadata.Version != versionFilter {
				continue
			}
			filtered = append(filtered, tool)
		}
		tools = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tools)
}

func (s *Server) handleToolDelete(w http.ResponseWriter, r *http.Request) {
	s.pluginLifecycleMu.Lock()
	defer s.pluginLifecycleMu.Unlock()

	name := r.URL.Query().Get(toolNameQueryParam)
	version := r.URL.Query().Get("version")
	hard := r.URL.Query().Get("hard") == queryTrueValue

	if name == "" || version == "" {
		writeControlPlaneError(w, http.StatusBadRequest, ErrorValidationFailed, "tool name and version are required", "provide both name and version")
		return
	}
	if s.store == nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage configuration")
		return
	}
	affected, err := s.store.ListPluginBindingsForTool(name, version)
	if err != nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage and retry")
		return
	}
	if err := s.flushPluginBindingTargets(r.Context(), affected); err != nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check cache health and retry")
		return
	}

	// Immediate deactivation for responsiveness and client notification
	s.DeregisterTool(name, version)

	if hard {
		if err := s.store.HardDelete(name, version); err != nil {
			writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage and retry")
			return
		}
	} else {
		if err := s.store.Delete(name, version); err != nil {
			writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage and retry")
			return
		}
	}

	if err := s.reconcileLocked(r.Context()); err != nil {
		writeReconciliationPending(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func testToolsEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(testToolsEnv)), "true")
}

// Admission Controller
func (s *Server) validateTool(t *Tool) error {
	if err := credentials.ValidateCredentialSource(t.Spec.Security.CredentialSource); err != nil {
		return fmt.Errorf("invalid security.credentialSource: %w", err)
	}
	if credentials.IsFileBacked(t.Spec.Security.CredentialSource) && t.Spec.Security.CredentialRef == "" {
		return fmt.Errorf("security.credentialRef is required for file credentials")
	}
	if t.Spec.Security.CredentialRef != "" {
		if err := credentials.ValidateReference(t.Spec.Security.CredentialRef); err != nil {
			return fmt.Errorf("invalid security.credentialRef")
		}
	}
	if t.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if t.Metadata.Version == "" {
		return fmt.Errorf("metadata.version is required")
	}
	if strings.Contains(strings.ToLower(t.Metadata.Name), "get-") ||
		strings.Contains(strings.ToLower(t.Metadata.Name), "post-") {
		return fmt.Errorf("tool name should be intent-based, not include HTTP verbs")
	}

	// Check for embedded secrets in Execution path (simplified check)
	if strings.Contains(t.Spec.Execution.Endpoint, "token ") ||
		strings.Contains(t.Spec.Execution.Endpoint, "key=") {
		return fmt.Errorf("endpoint should not contain raw secrets, use credentialRef instead")
	}

	switch t.Spec.Security.DataClass {
	case "", dataClassPublic, dataClassInternal:
	case dataClassPII, dataClassRestricted:
		if len(t.Spec.Security.AllowedRoles) == 0 {
			return fmt.Errorf("security.allowedRoles is required for dataClass %q", t.Spec.Security.DataClass)
		}
	default:
		return fmt.Errorf("invalid security.dataClass %q: must be one of public, internal, pii, or restricted", t.Spec.Security.DataClass)
	}

	roles, err := NormalizeRoles(t.Spec.Security.AllowedRoles)
	if err != nil {
		return fmt.Errorf("invalid security.allowedRoles: %w", err)
	}
	if len(roles) > 0 {
		if _, exists := t.Spec.InputSchema.Properties["role"]; exists || slices.Contains(t.Spec.InputSchema.Required, "role") {
			return fmt.Errorf("guarded tools must reserve role selector input for authorization")
		}
		t.Spec.Security.AllowedRoles = roles
	}

	return nil
}

func schemaForMCP(t *Tool) (InputSchema, error) {
	var schema InputSchema
	encoded, err := json.Marshal(t.Spec.InputSchema)
	if err != nil {
		return InputSchema{}, fmt.Errorf("copy input schema: %w", err)
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		return InputSchema{}, fmt.Errorf("copy input schema: %w", err)
	}
	if len(t.Spec.Security.AllowedRoles) == 0 {
		return schema, nil
	}
	if schema.Properties == nil {
		schema.Properties = make(map[string]Property)
	}
	roles, err := NormalizeRoles(t.Spec.Security.AllowedRoles)
	if err != nil {
		return InputSchema{}, fmt.Errorf("prepare role selector: %w", err)
	}
	schema.Properties["role"] = Property{
		Type:        schemaTypeString,
		Description: "Authorization role used for this tool call",
		Enum:        roles,
	}
	return schema, nil
}

func (s *Server) handleDirectInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeControlPlaneError(w, http.StatusMethodNotAllowed, ErrorMethodNotAllowed, "the requested control-plane method is not supported", "use POST to invoke a tool")
		return
	}

	var req ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if s.log != nil {
			s.log.Warn("direct invoke request was invalid")
		}
		writeControlPlaneError(w, http.StatusBadRequest, ErrorValidationFailed, "the tool invocation is not valid JSON", "submit one valid invocation request")
		return
	}

	s.mu.RLock()
	// Resolve tool by name (latest stable)
	t, err := s.registry.Resolve(req.Name, "")
	s.mu.RUnlock()

	if err != nil {
		writeControlPlaneError(w, http.StatusNotFound, ErrorResourceNotFound, "the requested tool was not found", "check the tool name and version")
		return
	}

	args := req.Arguments
	if args == nil {
		args = map[string]any{}
	}
	directContext, authorizedArgs, authzErr := s.authorizeDirectInvoke(r.Context(), t, args, r.Header.Get("X-ERPBridge-Role"))
	if authzErr != nil {
		writeRoleAuthorizationError(w, authzErr)
		return
	}

	// Create a bridge to the MCP middleware chain
	mcpReq := mcp.CallToolRequest{}
	mcpReq.Params.Name = req.Name
	mcpReq.Params.Arguments = authorizedArgs

	// Use the same execution and middleware seam as MCP tools/call.
	handler := s.applyToolMiddlewares(t, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.executeToolRequest(ctx, t, request)
	})

	// Execute through the middleware chain. The outcome pointer lets the
	// direct compatibility endpoint classify rate limiting without inspecting
	// the MCP error text returned to protocol clients.
	rateContext, rateOutcome := withRateLimitOutcome(directContext)
	mcpResult, err := handler(rateContext, mcpReq)
	if rateOutcome.limited {
		w.Header().Set("Retry-After", strconv.FormatInt(rateLimitRetryAfterSeconds(rateOutcome.retryAfter), 10))
		writeControlPlaneError(w, http.StatusTooManyRequests, ErrorRateLimited, "the tool request rate limit was exceeded", "retry after the indicated delay")
		return
	}
	if err != nil {
		// Keep the established direct-invoke compatibility response for plugin
		// processing failures. It contains only a fixed safe message; MCP
		// tools/call continues to use isError=true in its JSON-RPC result.
		if errors.Is(err, ErrPluginProcessingFailed) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{pluginErrorField: ErrPluginProcessingFailed.Error()})
			return
		}
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check tool and plugin health, then retry")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if mcpResult.IsError {
		writeControlPlaneError(w, http.StatusBadGateway, ErrorUpstreamUnreachable, "tool execution failed", "check the upstream ERP service and tool configuration")
		return
	}

	// Convert back to internal ToolResult for compatibility if needed,
	// or just send the MCP result content.
	// For bridgectl compatibility, we send ToolResult structure.
	var result any
	if mcpResult.StructuredContent != nil {
		result = mcpResult.StructuredContent
	} else if len(mcpResult.Content) > 0 {
		if text, ok := mcpResult.Content[0].(mcp.TextContent); ok {
			_ = json.Unmarshal([]byte(text.Text), &result)
		}
	}

	_ = json.NewEncoder(w).Encode(ToolResult{Result: result})
}

func writeRoleAuthorizationError(w http.ResponseWriter, err error) {
	status := http.StatusForbidden
	var roleErr *RoleAuthorizationError
	if errors.As(err, &roleErr) && roleErr.Status != 0 {
		status = roleErr.Status
	}
	writeControlPlaneError(w, status, ErrorAuthorizationDenied, "the caller is not authorized to invoke this tool", "use a token with the required role")
}

func (s *Server) handleCacheFlush(w http.ResponseWriter, r *http.Request) {
	if s.cache == nil {
		writeControlPlaneError(w, http.StatusServiceUnavailable, ErrorHealthCheckFailed, "cache is not enabled", "check cache configuration before retrying")
		return
	}

	tool := r.URL.Query().Get("tool")
	module := r.URL.Query().Get("module")
	all := r.URL.Query().Get("all") == "true"

	var count int
	var err error

	switch {
	case all:
		count, err = s.cache.FlushModule(r.Context(), "") // Empty matches all exact
	case tool != "":
		count, err = s.cache.FlushTool(r.Context(), tool)
	case module != "":
		if s.store == nil {
			writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check ERPBridge storage configuration")
			return
		}
		var tools []*Tool
		tools, err = s.store.ListByModule(module)
		if err == nil {
			for _, tool := range tools {
				var deleted int
				deleted, err = s.cache.FlushToolInternal(r.Context(), tool.Metadata.Name)
				count += deleted
				if err != nil {
					break
				}
			}
		}
	default:
		writeControlPlaneError(w, http.StatusBadRequest, ErrorValidationFailed, "one of tool, module, or all is required", "provide exactly one cache flush selector")
		return
	}

	if err != nil {
		writeControlPlaneInternalError(w, http.StatusInternalServerError, ErrorHealthCheckFailed, "check cache health and retry")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"deleted": count,
		statusKey: "ok",
	})
}

func (s *Server) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	if s.cache == nil {
		writeControlPlaneError(w, http.StatusServiceUnavailable, ErrorHealthCheckFailed, "cache is not enabled", "check cache configuration before retrying")
		return
	}

	statsContext, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	stats, err := s.cache.Stats(statsContext)
	if err != nil {
		writeControlPlaneInternalError(w, http.StatusServiceUnavailable, ErrorHealthCheckFailed, "check cache health and retry")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"apiVersion": "v1",
		"kind":       "CacheStats",
		statusKey:    "active",
		"stats":      stats,
	})
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	ch := logger.Subscribe()
	defer logger.Unsubscribe(ch)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", string(msg)); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleLogRecent(w http.ResponseWriter, _ *http.Request) {
	logs := logger.GetRecentLogs()
	w.Header().Set("Content-Type", "application/json")

	// Logs are already JSON strings
	_, _ = fmt.Fprintf(w, "[")
	for i, l := range logs {
		if i > 0 {
			_, _ = fmt.Fprintf(w, ",")
		}
		_, _ = fmt.Fprintf(w, "%s", string(l))
	}
	_, _ = fmt.Fprintf(w, "]")
}

// interceptingResponseWriter wraps http.ResponseWriter to capture the body.
type interceptingResponseWriter struct {
	http.ResponseWriter
	body *bytes.Buffer
}

func (iw *interceptingResponseWriter) Write(b []byte) (int, error) {
	return iw.body.Write(b)
}

func (iw *interceptingResponseWriter) Flush() {
	if flusher, ok := iw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
