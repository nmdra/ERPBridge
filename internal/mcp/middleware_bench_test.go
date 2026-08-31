package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
)

var benchmarkToolResult *mcp.CallToolResult

func BenchmarkCacheMiddleware(b *testing.B) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := logger.WithLogger(context.Background(), log)
	args := map[string]any{"employee": "E-0001"}
	req := mcp.CallToolRequest{}
	req.Params.Name = "get_employee"
	req.Params.Arguments = args

	b.Run("hit", func(b *testing.B) {
		manager := cache.NewMemoryManager(10, log)
		tool := &Tool{
			Metadata: Metadata{Name: "get_employee", Version: "1.0.0"},
			Spec:     ToolSpec{Cache: &cache.Config{Enabled: true, IsReadOnly: true}},
		}
		cached, err := json.Marshal(mcp.NewToolResultText("cached employee"))
		if err != nil {
			b.Fatal(err)
		}
		if err := manager.Set(ctx, tool.Metadata.Name, "", args, cached, *tool.Spec.Cache); err != nil {
			b.Fatal(err)
		}
		server := &Server{cache: manager, log: log}
		handler := server.CacheMiddleware(tool)(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			b.Fatal("cache hit invoked the next handler")
			return nil, nil
		})

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkToolResult, err = handler(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("hit_parallel", func(b *testing.B) {
		manager := cache.NewMemoryManager(10, log)
		tool := &Tool{
			Metadata: Metadata{Name: "get_employee", Version: "1.0.0"},
			Spec:     ToolSpec{Cache: &cache.Config{Enabled: true, IsReadOnly: true}},
		}
		cached, err := json.Marshal(mcp.NewToolResultText("cached employee"))
		if err != nil {
			b.Fatal(err)
		}
		if err := manager.Set(ctx, tool.Metadata.Name, "", args, cached, *tool.Spec.Cache); err != nil {
			b.Fatal(err)
		}
		server := &Server{cache: manager, log: log}
		var handlerCalls atomic.Int64
		var failures atomic.Int64
		handler := server.CacheMiddleware(tool)(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			handlerCalls.Add(1)
			return nil, errors.New("cache hit invoked the next handler")
		})

		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				result, callErr := handler(ctx, req)
				if callErr != nil || result == nil {
					failures.Add(1)
					return
				}
			}
		})
		b.StopTimer()
		if handlerCalls.Load() != 0 {
			b.Fatalf("fallback handler calls = %d, want 0", handlerCalls.Load())
		}
		if failures.Load() != 0 {
			b.Fatalf("cache hit failures = %d, want 0", failures.Load())
		}
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "ops/s")
	})

	b.Run("disabled", func(b *testing.B) {
		tool := &Tool{
			Metadata: Metadata{Name: "get_employee", Version: "1.0.0"},
			Spec:     ToolSpec{Cache: &cache.Config{Enabled: false}},
		}
		server := &Server{log: log}
		calls := 0
		handler := server.CacheMiddleware(tool)(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls++
			return mcp.NewToolResultText("fresh employee"), nil
		})
		if _, err := handler(ctx, req); err != nil {
			b.Fatal(err)
		}
		if calls != 1 {
			b.Fatalf("handler calls = %d, want 1", calls)
		}
		calls = 0

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var err error
			benchmarkToolResult, err = handler(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if calls != b.N {
			b.Fatalf("handler calls = %d, want %d", calls, b.N)
		}
	})
}
