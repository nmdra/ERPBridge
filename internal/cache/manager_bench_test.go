package cache

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
)

var (
	benchmarkCacheKey   string
	benchmarkCacheEntry *Entry
	benchmarkCacheErr   error
)

func BenchmarkArgsHash(b *testing.B) {
	cases := map[string]map[string]any{
		"empty": {},
		"small": {"employee": "E-0001", "includeInactive": false, "limit": 25},
		"nested": {
			"filters": map[string]any{"department": "Operations", "enabled": true},
			"fields":  []any{"name", "status", "modified"},
			"page":    2,
			"sort":    "modified desc",
		},
	}

	for name, args := range cases {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkCacheKey = argsHash(args)
			}
		})
	}
}

func BenchmarkExactKey(b *testing.B) {
	args := map[string]any{"employee": "E-0001", "includeInactive": false, "limit": 25}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCacheKey = exactKey("list_employees", "shared", args)
	}
}

func BenchmarkMemoryManager(b *testing.B) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := logger.WithLogger(context.Background(), log)
	args := map[string]any{"employee": "E-0001", "includeInactive": false}
	cfg := Config{Enabled: true}
	response := []byte(`{"content":[{"type":"text","text":"cached employee"}]}`)

	b.Run("hit", func(b *testing.B) {
		manager := NewMemoryManager(10, log)
		if err := manager.Set(ctx, "get_employee", "shared", args, response, cfg); err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkCacheEntry, benchmarkCacheErr = manager.Get(ctx, "get_employee", "shared", args, cfg)
			if benchmarkCacheErr != nil {
				b.Fatal(benchmarkCacheErr)
			}
		}
	})

	b.Run("set", func(b *testing.B) {
		manager := NewMemoryManager(10, log)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkCacheErr = manager.Set(ctx, "get_employee", "shared", args, response, cfg)
			if benchmarkCacheErr != nil {
				b.Fatal(benchmarkCacheErr)
			}
		}
	})
}
