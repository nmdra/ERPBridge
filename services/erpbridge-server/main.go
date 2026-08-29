// Package main is the entry point for the ERPBridge server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
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

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func serveHTTP(ctx context.Context, server *http.Server, listener net.Listener) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return shutdownErr
	}
}

func parseRateLimitConfig() (mcp.RateLimitConfig, error) {
	config := mcp.RateLimitConfig{RequestsPerSecond: 10, Burst: 20}
	if value := os.Getenv("RATE_LIMIT_RPS"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return mcp.RateLimitConfig{}, errors.New("invalid RATE_LIMIT_RPS")
		}
		config.RequestsPerSecond = parsed
	}
	if value := os.Getenv("RATE_LIMIT_BURST"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return mcp.RateLimitConfig{}, errors.New("invalid RATE_LIMIT_BURST")
		}
		config.Burst = parsed
	}
	if err := config.Validate(); err != nil {
		return mcp.RateLimitConfig{}, errors.New("invalid rate limit configuration")
	}
	return config, nil
}

func initializeCache(rootLog *slog.Logger) (*cache.Manager, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		options, err := redis.ParseURL(redisURL)
		if err != nil {
			return nil, errors.New("invalid REDIS_URL")
		}
		if options.DialTimeout == 0 {
			options.DialTimeout = time.Second
		}
		if options.ReadTimeout == 0 {
			options.ReadTimeout = time.Second
		}
		if options.WriteTimeout == 0 {
			options.WriteTimeout = time.Second
		}
		// Cache health must fail promptly when Redis is unavailable; tool
		// execution already treats this backend as best-effort.
		options.MaxRetries = 0
		return cache.NewManager(redis.NewClient(options), rootLog), nil
	}

	maxEntries := 10000
	if value := os.Getenv("CACHE_MEMORY_MAX_ENTRIES"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			slog.Warn("invalid CACHE_MEMORY_MAX_ENTRIES; using default")
		} else {
			maxEntries = parsed
		}
	}
	return cache.NewMemoryManager(maxEntries, rootLog), nil
}

func main() {
	stdioFlag := flag.Bool("stdio", false, "Run in STDIO transport mode")
	flag.Parse()

	rateConfig, err := parseRateLimitConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid rate limit configuration")
		os.Exit(1)
	}

	transport := os.Getenv("MCP_TRANSPORT")
	useStdio := *stdioFlag || transport == "stdio"

	bannerWriter := io.Writer(os.Stdout)
	if useStdio {
		_ = os.Setenv("LOG_TO_STDERR", "true")
		bannerWriter = os.Stderr
	}

	// Stdio reserves stdout for the MCP JSON-RPC stream.
	banner.Print(bannerWriter, "ERPBridge Server", version)

	// Initialize Logger
	rootLog := logger.Init()

	slog.Info("Starting ERPBridge Server")

	mcpPort := os.Getenv("MCP_PORT")
	if mcpPort == "" {
		mcpPort = "8080"
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "data/erpbridge.db"
	}

	// In a real scenario, this should be the public URL of the server
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s", mcpPort)
	}

	// Initialize Cache. An invalid configured Redis URL is a startup error;
	// an unreachable but valid Redis URL remains selected and degrades at use.
	cacheMgr, err := initializeCache(rootLog)
	if err != nil {
		slog.Error("invalid cache configuration")
		os.Exit(1)
	}
	if cacheMgr.BackendName() == "redis" {
		slog.Info("cache initialized", slog.String("backend", "redis"))
	} else {
		slog.Info("cache initialized", slog.String("backend", "memory"))
	}

	conn := connector.NewClient(rootLog)
	server := mcp.NewServer(conn, cacheMgr, rootLog, rateConfig, dbPath)
	server.SetServerInfo(mcp.ServerInfo{Version: version, Commit: commit, Date: date})

	// Setup signal-aware context for background workers
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Reconcile persisted resources before accepting transport requests.
	server.Reconcile(ctx)

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
			slog.Error("stdio server failed")
		}
		return
	}

	mux := http.NewServeMux()
	server.ServeHTTP(mux, baseURL)

	// Metrics endpoint
	mux.Handle("/metrics", server.AuthHandler(promhttp.Handler(), "metrics", false))

	httpServer := &http.Server{
		Addr:              ":" + mcpPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		slog.Error("server stopped with error")
		return
	}
	slog.Info("ERPBridge Server listening")
	if err := serveHTTP(ctx, httpServer, listener); err != nil {
		slog.Error("server stopped with error")
	}
}
