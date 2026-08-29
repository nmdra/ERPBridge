package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/metrics"
)

// TelemetryHooks implements lifecycle callbacks for telemetry and logging.
type TelemetryHooks struct {
	logger         *slog.Logger
	callStartTimes sync.Map
	activeSessions atomic.Int32
}

// NewTelemetryHooks creates a new TelemetryHooks instance.
func NewTelemetryHooks(logger *slog.Logger) *TelemetryHooks {
	return &TelemetryHooks{
		logger: logger,
	}
}

// Register attaches the telemetry hooks to the given MCP server.
func (h *TelemetryHooks) Register(s *server.MCPServer) {
	hooks := s.GetHooks()
	if hooks == nil {
		return
	}

	hooks.AddOnRegisterSession(h.OnSessionStart)
	hooks.AddOnUnregisterSession(h.OnSessionEnd)
	hooks.AddBeforeCallTool(h.OnBeforeCallTool)
	hooks.AddAfterCallTool(h.OnAfterCallTool)
	hooks.AddOnSuccess(h.OnRequestSuccess)
	hooks.AddOnError(h.OnRequestError)
}

// OnServerStart records telemetry when the MCP server starts.
func (h *TelemetryHooks) OnServerStart() {
	h.logger.Info("MCP Server starting")
	metrics.ServerStartsTotal.Inc()
}

// OnServerStop records telemetry when the MCP server stops.
func (h *TelemetryHooks) OnServerStop() {
	h.logger.Info("MCP Server stopping")
	metrics.ServerStopsTotal.Inc()
}

// OnSessionStart tracks session creation metrics and active session counts.
func (h *TelemetryHooks) OnSessionStart(_ context.Context, session server.ClientSession) {
	h.logger.Info("Session started", slog.String("session_id", session.SessionID()))
	metrics.SessionsStartedTotal.Inc()
	h.activeSessions.Add(1)
	metrics.SessionsActive.Set(float64(h.activeSessions.Load()))
}

// OnSessionEnd tracks session termination metrics and active session counts.
func (h *TelemetryHooks) OnSessionEnd(_ context.Context, session server.ClientSession) {
	h.logger.Info("Session ended", slog.String("session_id", session.SessionID()))
	metrics.SessionsEndedTotal.Inc()
	h.activeSessions.Add(-1)
	metrics.SessionsActive.Set(float64(h.activeSessions.Load()))
}

// OnBeforeCallTool starts timing a tool invocation.
func (h *TelemetryHooks) OnBeforeCallTool(_ context.Context, id any, _ *mcp.CallToolRequest) {
	h.callStartTimes.Store(id, time.Now())
}

// OnAfterCallTool completes timing a tool invocation and emits duration log.
func (h *TelemetryHooks) OnAfterCallTool(_ context.Context, id any, message *mcp.CallToolRequest, _ any) {
	start, ok := h.callStartTimes.LoadAndDelete(id)
	if !ok {
		return
	}
	duration := time.Since(start.(time.Time))

	h.logger.Info("Tool call completed",
		slog.String("tool", message.Params.Name),
		slog.Duration("duration", duration),
	)
}

// OnRequestSuccess records protocol requests that produced a JSON-RPC result.
func (h *TelemetryHooks) OnRequestSuccess(_ context.Context, _ any, method mcp.MCPMethod, _ any, _ any) {
	metrics.MCPRequestsTotal.WithLabelValues(string(method), "success").Inc()
}

// OnRequestError records protocol requests that produced a JSON-RPC error.
func (h *TelemetryHooks) OnRequestError(_ context.Context, _ any, method mcp.MCPMethod, _ any, _ error) {
	metrics.MCPRequestsTotal.WithLabelValues(string(method), "error").Inc()
}

// BusinessHooks implements custom business logic callbacks.
type BusinessHooks struct {
	notifier *CustomNotifier
	logger   *slog.Logger
}

// NewBusinessHooks creates a new BusinessHooks instance.
func NewBusinessHooks(notifier *CustomNotifier, logger *slog.Logger) *BusinessHooks {
	return &BusinessHooks{
		notifier: notifier,
		logger:   logger,
	}
}

// Register attaches the business logic hooks to the given MCP server.
func (h *BusinessHooks) Register(s *server.MCPServer) {
	hooks := s.GetHooks()
	if hooks == nil {
		return
	}

	hooks.AddOnError(h.OnError)
	hooks.AddOnRegisterSession(h.OnSessionStart)
}

// OnSessionStart sends welcome notifications when a new client session connects.
func (h *BusinessHooks) OnSessionStart(ctx context.Context, session server.ClientSession) {
	h.logger.Info("Business logic initialized for session", slog.String("session_id", session.SessionID()))
	// Send a welcome progress message
	h.notifier.SendProgress(ctx, 100, 100, "Connected to ERPBridge V2. Ready for declarative tool management.")
}

// OnError handles errors during MCP operations and dispatches alerts if needed.
func (h *BusinessHooks) OnError(ctx context.Context, _ any, method mcp.MCPMethod, _ any, err error) {
	h.logger.Error("Operation failed", slog.Any("method", method), slog.Any("error", err))

	// Send an alert on tool failure
	if string(method) == "tools/call" {
		h.notifier.SendAlert(ctx, fmt.Sprintf("Tool execution failed: %v", err), "error")
	}
}
