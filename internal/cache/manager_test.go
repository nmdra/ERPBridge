// internal/cache/manager_test.go
package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgsHash(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "empty args",
			args: map[string]any{},
			want: "empty",
		},
		{
			name: "simple args",
			args: map[string]any{"a": 1, "b": "2"},
			want: "347158d22448fce173d48b99e0e863efb89ffc92144215c41c21e44a2da25580",
		},
		{
			name: "reordered args",
			args: map[string]any{"b": "2", "a": 1},
			want: "347158d22448fce173d48b99e0e863efb89ffc92144215c41c21e44a2da25580", // Should be the same
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := argsHash(tt.args); got != tt.want {
				t.Errorf("argsHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoleScope(t *testing.T) {
	tests := []struct {
		role       string
		isReadOnly bool
		want       string
	}{
		{"admin", false, "admin"},
		{"", false, "anonymous"},
		{"user", true, "shared"},
	}

	for _, tt := range tests {
		got := roleScope(tt.role, tt.isReadOnly)
		assert.Equal(t, tt.want, got)
	}
}

func TestExactKey(t *testing.T) {
	key := exactKey("tool", "role", map[string]any{"a": 1})
	assert.Contains(t, key, "exact:tool:role:")
}

func TestNewManager(t *testing.T) {
	log := logger.Init()
	m := NewManager(nil, log)
	assert.NotNil(t, m)
}

func TestManager_RedisBackendRemainsSelectedWhenClosed(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr(), MaxRetries: 0})
	manager := NewManager(client, logger.Init())
	cfg := Config{Enabled: true, TTLSeconds: 60}
	ctx := context.Background()

	require.Equal(t, "redis", manager.BackendName())
	require.NoError(t, manager.Set(ctx, "tool", "", map[string]any{"id": 1}, []byte(`{"ok":true}`), cfg))
	mini.Close()

	entry, err := manager.Get(ctx, "tool", "", map[string]any{"id": 1}, cfg)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, "miss", entry.HitType)
	require.Error(t, manager.Set(ctx, "tool", "", map[string]any{"id": 2}, []byte(`{"ok":true}`), cfg))
	_, err = manager.Stats(ctx)
	require.Error(t, err)
}

func TestManager_MemoryBackendLabel(t *testing.T) {
	require.Equal(t, "memory", NewMemoryManager(10, logger.Init()).BackendName())
}

func TestMemoryBackend_BoundedLRU(t *testing.T) {
	backend := NewMemoryBackend(2)
	ctx := context.Background()

	require.NoError(t, backend.Set(ctx, "a", []byte("A"), 0))
	require.NoError(t, backend.Set(ctx, "b", []byte("B"), 0))
	if _, err := backend.Get(ctx, "a"); err != nil {
		t.Fatalf("expected a hit before eviction: %v", err)
	}
	require.NoError(t, backend.Set(ctx, "c", []byte("C"), 0))
	if _, err := backend.Get(ctx, "b"); !errors.Is(err, errCacheMiss) {
		t.Fatalf("least recently used entry b should be evicted, got %v", err)
	}
	if _, err := backend.Get(ctx, "a"); err != nil {
		t.Fatalf("recently used entry a should remain, got %v", err)
	}
}

func TestMemoryBackend_ZeroCapacityDisablesStorage(t *testing.T) {
	backend := NewMemoryBackend(0)
	ctx := context.Background()
	require.NoError(t, backend.Set(ctx, "a", []byte("A"), 0))
	if _, err := backend.Get(ctx, "a"); !errors.Is(err, errCacheMiss) {
		t.Fatalf("zero capacity should not store entries, got %v", err)
	}
}

func TestManager_MemorySetGetStoresEnvelopeTimestamp(t *testing.T) {
	m := NewMemoryManager(10, logger.Init())
	cfg := Config{Enabled: true}
	response := []byte(`{"content":[{"type":"text","text":"ok"}]}`)

	require.NoError(t, m.Set(context.Background(), "tool", "", map[string]any{"id": 1}, response, cfg))
	entry, err := m.Get(context.Background(), "tool", "", map[string]any{"id": 1}, cfg)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "exact", entry.HitType)
	assert.Equal(t, response, []byte(entry.Response))
	assert.False(t, entry.CachedAt.IsZero())
}
