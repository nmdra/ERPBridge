package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/credentials"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/metrics"
	"golang.org/x/time/rate"
)

// RateLimitMiddleware provides per-session rate limiting for tool execution.
type RateLimitMiddleware struct {
	limiters map[string]*limiterEntry
	mutex    sync.Mutex
	rate     rate.Limit
	burst    int
	now      func() time.Time
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const (
	maxLimiterEntries = 10000
	limiterIdleTTL    = 15 * time.Minute
)

// NewRateLimitMiddleware initializes a new RateLimitMiddleware with the given rate and burst.
func NewRateLimitMiddleware(requestsPerSecond float64, burst int) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(requestsPerSecond),
		burst:    burst,
		now:      time.Now,
	}
}

func (m *RateLimitMiddleware) getLimiter(sessionID string) *rate.Limiter {
	if sessionID == "" {
		sessionID = "anonymous"
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := m.now()
	if entry, exists := m.limiters[sessionID]; exists {
		entry.lastSeen = now
		return entry.limiter
	}

	if len(m.limiters) >= maxLimiterEntries {
		for key, entry := range m.limiters {
			if now.Sub(entry.lastSeen) > limiterIdleTTL {
				delete(m.limiters, key)
			}
		}
	}

	limiter := rate.NewLimiter(m.rate, m.burst)
	m.limiters[sessionID] = &limiterEntry{limiter: limiter, lastSeen: now}
	return limiter
}

type rateLimitPrincipalKey struct{}

// WithRateLimitPrincipal attaches an authenticated identity to the limiter context.
func WithRateLimitPrincipal(ctx context.Context, principal string) context.Context {
	return context.WithValue(ctx, rateLimitPrincipalKey{}, principal)
}

func rateLimitPrincipal(ctx context.Context) string {
	principal, _ := ctx.Value(rateLimitPrincipalKey{}).(string)
	return principal
}

// Handle returns a server.ToolHandlerMiddleware that enforces rate limits.
func (m *RateLimitMiddleware) Handle() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sessionID := rateLimitPrincipal(ctx)
			if sess := server.ClientSessionFromContext(ctx); sess != nil {
				if sessionID == "" {
					sessionID = sess.SessionID()
				}
			}
			limiter := m.getLimiter(sessionID)

			if !limiter.Allow() {
				return mcp.NewToolResultError(fmt.Sprintf("rate limit exceeded for session %s", sessionID)), nil
			}

			return next(ctx, req)
		}
	}
}

// LoggingMiddleware audits tool execution by logging start, completion, and failure events.
func LoggingMiddleware(log *slog.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			sessionID := ""
			if sess := server.ClientSessionFromContext(ctx); sess != nil {
				sessionID = sess.SessionID()
			}
			reqID := logger.NewRequestID()

			toolLog := log.With(
				slog.String("session_id", sessionID),
				slog.String("request_id", reqID),
				slog.String("tool_name", req.Params.Name),
			)

			ctx = logger.WithLogger(ctx, toolLog)

			toolLog.InfoContext(ctx, "tool execution started", slog.Any("arguments", logger.RedactArgs(req.Params.Arguments)))

			result, err := next(ctx, req)

			duration := time.Since(start)
			if err != nil {
				toolLog.ErrorContext(ctx, "tool execution failed",
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
			} else {
				toolLog.InfoContext(ctx, "tool execution completed",
					slog.Duration("duration", duration),
				)
			}

			return result, err
		}
	}
}

// MetricsMiddleware records execution latency and invocation counts for Prometheus.
func MetricsMiddleware() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			result, err := next(ctx, req)
			duration := time.Since(start)

			status := "SUCCESS"
			if err != nil || (result != nil && result.IsError) {
				status = "ERROR"
			}

			metrics.ToolInvocationsTotal.WithLabelValues(req.Params.Name, status).Inc()
			metrics.ToolLatency.WithLabelValues(req.Params.Name).Observe(duration.Seconds())

			return result, err
		}
	}
}

// CacheMiddleware handles exact matching cache for tool results.
func (s *Server) CacheMiddleware(t *Tool) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if s.cache == nil || t.Spec.Cache == nil {
				return next(ctx, req)
			}
			if s.cacheBypassedForCredential(t) {
				return next(ctx, req)
			}

			args, _ := req.Params.Arguments.(map[string]any)
			role := ""
			if len(t.Spec.Security.AllowedRoles) > 0 {
				role, _ = CallerRoleFromContext(ctx)
			}
			cacheGeneration := uint64(0)

			if t.Spec.Cache.Enabled {
				// Read the cache and lifecycle generation as one short critical section.
				s.pluginLifecycleMu.RLock()
				cacheGeneration = s.lifecycleGeneration
				entry, err := s.cache.Get(ctx, t.Metadata.Name, role, args, *t.Spec.Cache)
				s.pluginLifecycleMu.RUnlock()
				if err == nil && entry != nil && entry.HitType != "miss" {
					var cachedResult mcp.CallToolResult
					if unmarshalErr := json.Unmarshal(entry.Response, &cachedResult); unmarshalErr == nil {
						metrics.CacheHitsTotal.WithLabelValues(entry.HitType).Inc()
						s.log.Debug("cache hit", slog.String("tool", t.Metadata.Name), slog.String("type", entry.HitType))
						return &cachedResult, nil
					}
				}

				metrics.CacheMissesTotal.Inc()
			}

			// Execute next outside the lifecycle lock. A generation check below
			// prevents an in-flight pre-change result from repopulating the cache.
			result, err := next(ctx, req)
			if err != nil || result == nil || result.IsError {
				return result, err
			}

			// WRITE to cache only for enabled read caching and an unchanged lifecycle.
			if t.Spec.Cache.Enabled {
				respJSON, marshalErr := json.Marshal(result)
				if marshalErr != nil {
					s.log.Warn("failed to marshal cache result", slog.String("error", marshalErr.Error()))
				} else {
					s.pluginLifecycleMu.RLock()
					if cacheGeneration == s.lifecycleGeneration {
						if cacheErr := s.cache.Set(ctx, t.Metadata.Name, role, args, respJSON, *t.Spec.Cache); cacheErr != nil {
							s.log.Warn("failed to cache result", slog.String("error", cacheErr.Error()))
						}
					}
					s.pluginLifecycleMu.RUnlock()
				}
			}

			// Invalidation (Auto-Flush)
			if len(t.Spec.Cache.FlushOn) > 0 {
				if err := s.cache.AutoFlush(ctx, t.Spec.Cache.FlushOn); err != nil {
					s.log.Warn("auto-flush failed", slog.String("error", err.Error()))
				}
			}

			return result, err
		}
	}
}

// cacheBypassedForCredential uses the current registered tool metadata before
// cache access. This avoids stale middleware closures serving a result after a
// tool switches from environment to file-backed credentials.
func (s *Server) cacheBypassedForCredential(fallback *Tool) bool {
	current := fallback
	if s != nil && s.registry != nil && fallback != nil {
		s.mu.RLock()
		resolved, err := s.registry.Resolve(fallback.Metadata.Name, "")
		s.mu.RUnlock()
		if err == nil && resolved != nil {
			current = resolved
		}
	}
	if current != nil && credentials.IsFileBacked(current.Spec.Security.CredentialSource) {
		return true
	}
	if s == nil || s.pluginRegistry == nil || current == nil {
		return false
	}
	for _, binding := range s.pluginRegistry.RuntimeBindingsForTool(current.Metadata.Name, current.Metadata.Version) {
		if binding != nil && binding.Plugin != nil && binding.Plugin.Spec.Auth != nil &&
			credentials.IsFileBacked(binding.Plugin.Spec.Auth.CredentialSource) {
			return true
		}
	}
	return false
}
