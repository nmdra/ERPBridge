// Package main is the entry point for the ERPBridge server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mcp_server "github.com/mark3labs/mcp-go/server"
	"github.com/nmdra/ERPBridge/internal/banner"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

var version = "dev"

func main() {
	// Print startup banner
	banner.Print(os.Stdout, "ERPBridge Server", version)

	stdioFlag := flag.Bool("stdio", false, "Run in STDIO transport mode")
	flag.Parse()

	transport := os.Getenv("MCP_TRANSPORT")
	useStdio := *stdioFlag || transport == "stdio"

	if useStdio {
		_ = os.Setenv("LOG_TO_STDERR", "true")
	}

	// Initialize Logger
	rootLog := logger.Init()

	slog.Info("Starting ERPBridge Server", slog.String("version", version), slog.Bool("stdio", useStdio))

	mcpPort := os.Getenv("MCP_PORT")
	if mcpPort == "" {
		mcpPort = "8080"
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "data/erpbridge.db"
	}

	redisURL := os.Getenv("REDIS_URL")

	rateRPS := 5.0
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		if _, err := fmt.Sscanf(v, "%f", &rateRPS); err != nil {
			slog.Warn("failed to parse RATE_LIMIT_RPS", slog.String("value", v), slog.String("error", err.Error()))
		}
	}
	rateBurst := 10
	if v := os.Getenv("RATE_LIMIT_BURST"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &rateBurst); err != nil {
			slog.Warn("failed to parse RATE_LIMIT_BURST", slog.String("value", v), slog.String("error", err.Error()))
		}
	}

	// In a real scenario, this should be the public URL of the server
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s", mcpPort)
	}

	// Initialize Cache
	var cacheMgr *cache.Manager
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			slog.Error("failed to parse redis url", slog.String("error", err.Error()))
		} else {
			rdb := redis.NewClient(opt)
			cacheMgr = cache.NewManager(rdb, rootLog)
			slog.Info("cache initialized", slog.String("backend", "redis"))
		}
	} else {
		maxEntries := 10000
		if value := os.Getenv("CACHE_MEMORY_MAX_ENTRIES"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				slog.Warn("invalid CACHE_MEMORY_MAX_ENTRIES; using default", slog.String("value", value), slog.Int("default", maxEntries))
			} else {
				maxEntries = parsed
			}
		}
		cacheMgr = cache.NewMemoryManager(maxEntries, rootLog)
		slog.Info("cache initialized", slog.String("backend", "memory"), slog.Int("max_entries", maxEntries))
	}

	conn := connector.NewClient(rootLog)
	server := mcp.NewServer(conn, cacheMgr, rootLog, mcp.RateLimitConfig{
		RequestsPerSecond: rateRPS,
		Burst:             rateBurst,
	}, dbPath)

	// Setup signal-aware context for background workers
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start Reconciliation Loop
	go server.StartController(ctx)

	// Trigger Server Start Hook
	server.TelemetryHooks.OnServerStart()
	defer server.TelemetryHooks.OnServerStop()

	if useStdio {
		slog.Info("ERPBridge Server running in STDIO mode")
		stdioServer := mcp_server.NewStdioServer(server.MCPServer())
		filteredWriter := mcp.NewToolListFilterWriter(os.Stdout, server.FilterToolsList)
		if err := stdioServer.Listen(context.Background(), os.Stdin, filteredWriter); err != nil {
			slog.Error("stdio server failed", slog.String("error", err.Error()))
		}
		return
	}

	mux := http.NewServeMux()
	server.ServeHTTP(mux, baseURL)

	// Metrics endpoint
	mux.Handle("/metrics", server.AuthHandler(promhttp.Handler(), "metrics", false))

	slog.Info("ERPBridge Server listening",
		slog.String("port", mcpPort),
		slog.String("mcp_http", baseURL+"/mcp/"),
	)
	httpServer := &http.Server{
		Addr:              ":" + mcpPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server stopped with error", slog.String("error", err.Error()))
	}
}
