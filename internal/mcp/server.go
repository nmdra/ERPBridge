// Package mcp implements the Model Context Protocol (MCP) server,
// allowing AI agents to interact with ERP systems through tools,
// resources, and prompts.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
)

// Server is the primary MCP server implementation for ERPBridge.
type Server struct {
	mcpServer       *server.MCPServer
	connector       ERPConnector
	cache           *cache.Manager
	log             *slog.Logger
	mu              sync.RWMutex
	store           *Store
	registry        *ToolRegistry
	lastDesiredHash string
	resources       map[string]*Resource
	prompts         map[string]*Prompt
	Notifier        *CustomNotifier
	TelemetryHooks  *TelemetryHooks
	BusinessHooks   *BusinessHooks
	toolMiddlewares []server.ToolHandlerMiddleware
	authWarnOnce    sync.Once
}

const (
	statusKey          = "status"
	toolNameQueryParam = "name"
)

// RateLimitConfig defines the configuration for the tool rate limiter.
type RateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
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
		mcpServer: s,
		connector: connector,
		cache:     cacheMgr,
		log:       mcpLog,
		store:     store,
		registry:  NewToolRegistry(),
		resources: make(map[string]*Resource),
		prompts:   make(map[string]*Prompt),
		Notifier:  NewCustomNotifier(s),
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
				if !credentialConfigured(tool.Spec.Security.CredentialRef) {
					s.log.Warn("tool credential reference is unresolved", slog.String("tool_name", tool.Metadata.Name), slog.String("credential_ref", tool.Spec.Security.CredentialRef))
				}
			}
		}
	}

	for _, resource := range resources {
		if !credentialConfigured(resource.Security.CredentialRef) {
			s.log.Warn("resource credential reference is unresolved", slog.String("resource_name", resource.Name), slog.String("credential_ref", resource.Security.CredentialRef))
		}
	}
}

func credentialConfigured(ref string) bool {
	if ref == "" {
		return true
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
					Type: "text",
					Text: text,
				},
			},
		},
	}, nil
}

// StartController runs the background reconciliation loop.
func (s *Server) StartController(ctx context.Context) {
	s.log.Info("starting reconciliation controller")
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
func (s *Server) Reconcile(_ context.Context) {
	if s.store == nil {
		return
	}

	// Check if state has changed
	currentHash, err := s.store.GetStateHash()
	if err != nil {
		s.log.Error("failed to get state hash", slog.String("error", err.Error()))
		return
	}

	s.mu.RLock()
	lastHash := s.lastDesiredHash
	s.mu.RUnlock()

	if currentHash == lastHash {
		return
	}

	s.log.Info("reconciling tools", slog.String("old_hash", lastHash), slog.String("new_hash", currentHash))

	desiredTools, err := s.store.List()
	if err != nil {
		s.log.Error("failed to list desired tools", slog.String("error", err.Error()))
		return
	}
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
	s.mu.Lock()
	defer s.mu.Unlock()

	t.Metadata.IsActive = true // Ensure it is marked as active when registered
	if err := s.registry.Add(t); err != nil {
		s.log.Error("failed to add tool to registry", slog.String("tool", t.Metadata.Name), slog.String("error", err.Error()))
		return
	}

	// Serialize the input schema to JSON.RawMessage
	schemaJSON, err := json.Marshal(t.Spec.InputSchema)
	if err != nil {
		s.log.Error("failed to marshal input schema", slog.String("tool_name", t.Metadata.Name), slog.String("error", err.Error()))
		return
	}

	// Use versioned name internally for MCP registration if it's not the default?
	// Actually, the spec says we should use stable aliases.
	// For now, we register with the base name and let the handler resolve.
	mcpTool := mcp.NewToolWithRawSchema(t.Metadata.Name, t.Spec.Description.Short, json.RawMessage(schemaJSON))

	// Explicitly clear structured schema fields to avoid conflict during marshaling
	mcpTool.InputSchema = mcp.ToolInputSchema{}
	mcpTool.OutputSchema = mcp.ToolOutputSchema{}

	// Add tool to server with handler
	handler := s.handleMCPToolCall(t.Metadata.Name)

	// Apply tool-specific middlewares
	handler = s.CacheMiddleware(t)(handler)

	// Apply global middlewares
	for _, v := range slices.Backward(s.toolMiddlewares) {
		handler = v(handler)
	}

	s.mcpServer.AddTool(mcpTool, handler)
	s.log.Info("registered MCP tool", slog.String("tool_name", t.Metadata.Name), slog.String("version", t.Metadata.Version))

	// Notify clients that tools have changed
	s.mcpServer.SendNotificationToAllClients("notifications/tools/list_changed", nil)
}

func (s *Server) handleMCPToolCall(name string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		s.mu.RLock()
		// Resolve the latest stable version for this tool name
		t, err := s.registry.Resolve(name, "")
		s.mu.RUnlock()

		if err != nil {
			return nil, fmt.Errorf("tool not found: %s (%w)", name, err)
		}

		// Type assertion for arguments
		args, ok := request.Params.Arguments.(map[string]any)
		if !ok && request.Params.Arguments != nil {
			return nil, fmt.Errorf("invalid arguments format")
		}

		result, err := t.Execute(ctx, args, s.connector)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Convert result to mcp-go format
		resultJSON, _ := json.Marshal(result.Result)
		return mcp.NewToolResultText(string(resultJSON)), nil
	}
}

// MCPServer returns the underlying mcp-go MCPServer instance.
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcpServer
}

// ServeHTTP handles the various MCP transports and management endpoints.
func (s *Server) ServeHTTP(mux *http.ServeMux, _ string) {
	// 1. Streamable HTTP Transport (Modern clients, Postman)
	// MUST strip prefix so the server sees "/" internally
	streamableServer := server.NewStreamableHTTPServer(s.mcpServer,
		server.WithStateful(true),
		server.WithSessionIdleTTL(30*time.Minute),
		server.WithEndpointPath("/"), // Tell the server it is mounted at /
		server.WithStreamableHTTPCORS(
			server.WithCORSAllowedOrigins("*"),
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

	mux.Handle("/mcp/", s.AuthHandler(http.StripPrefix("/mcp", filteredStreamable), scopeMCP, false))

	// 3. Management & Utility Endpoints
	mux.HandleFunc("/mcp/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.Handle("/api/auth/tokens", http.HandlerFunc(s.handleTokenAPI))
	mux.Handle("/api/auth/tokens/", http.HandlerFunc(s.handleTokenAPI))
	mux.Handle("/api/tools/invoke", s.AuthHandler(http.HandlerFunc(s.handleDirectInvoke), "", true))
	mux.Handle("/api/cache/stats", s.AuthHandler(http.HandlerFunc(s.handleCacheStats), "", true))
	mux.Handle("/api/cache/flush", s.AuthHandler(http.HandlerFunc(s.handleCacheFlush), "", true))
	mux.Handle("/api/logs/stream", s.AuthHandler(http.HandlerFunc(s.handleLogStream), "logs", false))
	mux.Handle("/api/logs/recent", s.AuthHandler(http.HandlerFunc(s.handleLogRecent), "logs", false))

	// 4. Kubernetes-Style Tool API
	mux.Handle("/apis/erpbridge.io/v1/tools", s.AuthHandler(http.HandlerFunc(s.handleToolAPI), "", true))
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolApply(w http.ResponseWriter, r *http.Request) {
	var t Tool
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Admission Controller
	if err := s.validateTool(&t); err != nil {
		http.Error(w, "invalid tool: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	if s.store == nil {
		http.Error(w, "store not available", http.StatusInternalServerError)
		return
	}

	t.Metadata.IsActive = true // Mark as active before saving

	if err := s.store.Save(&t); err != nil {
		http.Error(w, "failed to save tool: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Immediate reconciliation for responsiveness
	s.RegisterTool(&t)
	s.warnUnresolvedCredentials()

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		statusKey:          "applied",
		toolNameQueryParam: t.Metadata.Name,
		"version":          t.Metadata.Version,
	})
}

func (s *Server) handleToolList(w http.ResponseWriter, r *http.Request) {
	tools, err := s.store.List()
	if err != nil {
		http.Error(w, "failed to list tools: "+err.Error(), http.StatusInternalServerError)
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
	name := r.URL.Query().Get(toolNameQueryParam)
	version := r.URL.Query().Get("version")
	hard := r.URL.Query().Get("hard") == "true"

	if name == "" || version == "" {
		http.Error(w, "missing name or version parameter", http.StatusBadRequest)
		return
	}

	// Immediate deactivation for responsiveness and client notification
	s.DeregisterTool(name, version)

	if hard {
		if err := s.store.HardDelete(name, version); err != nil {
			http.Error(w, "failed to hard delete tool: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := s.store.Delete(name, version); err != nil {
			http.Error(w, "failed to delete tool: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// Admission Controller
func (s *Server) validateTool(t *Tool) error {
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

	return nil
}

func (s *Server) handleDirectInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Error("bad request", slog.String("error", err.Error()))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	// Resolve tool by name (latest stable)
	t, err := s.registry.Resolve(req.Name, "")
	s.mu.RUnlock()

	if err != nil {
		http.Error(w, "tool not found", http.StatusNotFound)
		return
	}

	// Create a bridge to the MCP middleware chain
	mcpReq := mcp.CallToolRequest{}
	mcpReq.Params.Name = req.Name
	mcpReq.Params.Arguments = req.Arguments

	// Base handler for direct invoke
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]any)
		if !ok && request.Params.Arguments != nil {
			return nil, fmt.Errorf("invalid arguments format")
		}
		result, err := t.Execute(ctx, args, s.connector)
		if err != nil {
			return nil, err
		}
		resultJSON, _ := json.Marshal(result.Result)
		return mcp.NewToolResultText(string(resultJSON)), nil
	}

	// Apply tool-specific middlewares
	handler = s.CacheMiddleware(t)(handler)

	// Apply global middlewares
	for _, v := range slices.Backward(s.toolMiddlewares) {
		handler = v(handler)
	}

	// Execute through the middleware chain
	mcpResult, err := handler(r.Context(), mcpReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if mcpResult.IsError {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": mcpResult.Content})
		return
	}

	// Convert back to internal ToolResult for compatibility if needed,
	// or just send the MCP result content.
	// For bridgectl compatibility, we send ToolResult structure.
	var result any
	if len(mcpResult.Content) > 0 {
		if text, ok := mcpResult.Content[0].(mcp.TextContent); ok {
			_ = json.Unmarshal([]byte(text.Text), &result)
		}
	}

	_ = json.NewEncoder(w).Encode(ToolResult{Result: result})
}

func (s *Server) handleCacheFlush(w http.ResponseWriter, r *http.Request) {
	if s.cache == nil {
		http.Error(w, "cache not enabled", http.StatusServiceUnavailable)
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
			http.Error(w, "store not available", http.StatusInternalServerError)
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
		http.Error(w, "missing tool, module or all parameter", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "cache not enabled", http.StatusServiceUnavailable)
		return
	}

	stats, err := s.cache.Stats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

	ch := logger.Subscribe()
	defer logger.Unsubscribe(ch)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(msg))
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
