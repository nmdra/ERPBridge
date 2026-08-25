package idp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	log := logger.Init()

	t.Run("empty path uses home dir", func(t *testing.T) {
		reg, err := NewRegistry("", log)
		require.NoError(t, err)
		assert.NotNil(t, reg)

		home, _ := os.UserHomeDir()
		expectedPath := filepath.Join(home, ".bridgectl", "registry.json")
		assert.Equal(t, expectedPath, reg.path)
	})

	t.Run("specific path", func(t *testing.T) {
		tmpPath := filepath.Join(t.TempDir(), "test-registry.json")
		reg, err := NewRegistry(tmpPath, log)
		require.NoError(t, err)
		assert.Equal(t, tmpPath, reg.path)
		assert.NotNil(t, reg.APIs)
	})

	t.Run("load existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpPath := filepath.Join(tmpDir, "existing.json")

		const testAPIName = "test-api"
		initialData := struct {
			APIs map[string]API
		}{
			APIs: map[string]API{
				testAPIName: {ID: "1", Name: testAPIName},
			},
		}
		data, _ := json.Marshal(initialData)
		err := os.WriteFile(tmpPath, data, 0600)
		require.NoError(t, err)

		reg, err := NewRegistry(tmpPath, log)
		require.NoError(t, err)
		assert.Len(t, reg.APIs, 1)
		assert.Equal(t, "1", reg.APIs[testAPIName].ID)
	})
}

func TestRegistry_Load(t *testing.T) {
	log := logger.Init()
	tmpPath := filepath.Join(t.TempDir(), "test-registry.json")
	reg := &Registry{
		path: tmpPath,
		log:  log,
		APIs: make(map[string]API),
	}

	t.Run("file does not exist", func(t *testing.T) {
		err := reg.load()
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("invalid json", func(t *testing.T) {
		err := os.WriteFile(tmpPath, []byte("{invalid-json"), 0600)
		require.NoError(t, err)

		err = reg.load()
		assert.Error(t, err)
	})
}

func TestRegistry_Save(t *testing.T) {
	log := logger.Init()
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "save-test.json")

	reg := &Registry{
		path: tmpPath,
		log:  log,
		APIs: map[string]API{
			"api1": {ID: "1", Name: "api1"},
		},
	}

	t.Run("success", func(t *testing.T) {
		err := reg.save()
		assert.NoError(t, err)

		// #nosec G304 -- test file path is within temporary test directory
		data, err := os.ReadFile(filepath.Clean(tmpPath))
		assert.NoError(t, err)
		assert.Contains(t, string(data), "api1")
	})

	t.Run("unwritable dir", func(t *testing.T) {
		unwritableDir := filepath.Join(tmpDir, "readonly")
		err := os.Mkdir(unwritableDir, 0400)
		require.NoError(t, err)

		reg.path = filepath.Join(unwritableDir, "subdir", "test.json")
		err = reg.save()
		assert.Error(t, err)
	})
}

func TestRegistry_RegisterListDeleteGet(t *testing.T) {
	log := logger.Init()
	tmpPath := filepath.Join(t.TempDir(), "test-registry.json")
	reg, err := NewRegistry(tmpPath, log)
	require.NoError(t, err)

	// Test Register
	t.Run("Register new API", func(t *testing.T) {
		api := &API{
			Name: "test-api",
			URL:  "http://localhost",
		}

		err := reg.Register(api)
		assert.NoError(t, err)

		assert.NotEmpty(t, api.ID)
		assert.Equal(t, "active", api.Status)
		assert.NotZero(t, api.CreatedAt)

		assert.Len(t, reg.APIs, 1)
		assert.Equal(t, "test-api", reg.APIs["test-api"].Name)
	})

	// Test Register with existing ID
	t.Run("Register API with ID", func(t *testing.T) {
		api := &API{
			ID:   "custom-id",
			Name: "test-api-2",
		}

		err := reg.Register(api)
		assert.NoError(t, err)

		assert.Equal(t, "custom-id", api.ID)
		assert.Len(t, reg.APIs, 2)
	})

	// Test List
	t.Run("List APIs", func(t *testing.T) {
		list := reg.List()
		assert.Len(t, list, 2)
	})

	// Test Get
	t.Run("Get existing API", func(t *testing.T) {
		api, ok := reg.Get("test-api")
		assert.True(t, ok)
		assert.Equal(t, "test-api", api.Name)
	})

	t.Run("Get non-existing API", func(t *testing.T) {
		_, ok := reg.Get("non-existing")
		assert.False(t, ok)
	})

	// Test Delete
	t.Run("Delete API", func(t *testing.T) {
		err := reg.Delete("test-api")
		assert.NoError(t, err)

		assert.Len(t, reg.APIs, 1)
		_, ok := reg.Get("test-api")
		assert.False(t, ok)
	})
}

func TestRegistry_LegacyCredentialsAreDetectedAndBlocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	data := []byte(`{"apis":{"legacy":{"name":"legacy","url":"https://erp","authKey":"SECRET_KEY","authToken":"SECRET_TOKEN"}}}`)
	require.NoError(t, os.WriteFile(path, data, 0600))
	reg, err := NewRegistry(path, logger.Init())
	require.NoError(t, err)
	assert.True(t, reg.HasLegacyCredentials())
	assert.ErrorIs(t, reg.Register(&API{Name: "new"}), ErrLegacyCredentials)
	assert.ErrorIs(t, reg.Delete("legacy"), ErrLegacyCredentials)
	assert.ErrorIs(t, reg.SetCredentialRef("legacy", "ERP_KEY"), ErrLegacyCredentials)
}

func TestRegistry_ScrubCredentialsNoOpWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "registry.json")
	reg, err := NewRegistry(path, logger.Init())
	require.NoError(t, err)
	require.NoError(t, reg.ScrubCredentials())
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRegistry_ScrubCredentialsAtomicallyRemovesLegacyFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	data := []byte(`{"apis":{"legacy":{"name":"legacy","url":"https://erp","authKey":"SECRET_KEY","authToken":"SECRET_TOKEN","module":"sales"}}}`)
	require.NoError(t, os.WriteFile(path, data, 0600))
	reg, err := NewRegistry(path, logger.Init())
	require.NoError(t, err)
	require.NoError(t, reg.ScrubCredentials())
	result := string(mustReadRegistry(t, path))
	assert.NotContains(t, result, "authKey")
	assert.NotContains(t, result, "authToken")
	assert.NotContains(t, result, "SECRET_KEY")
	assert.NotContains(t, result, "SECRET_TOKEN")
	assert.Contains(t, result, "sales")
	assert.False(t, reg.HasLegacyCredentials())
}

func TestRegistry_ScrubCredentialsWriteFailureLeavesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	data := []byte(`{"apis":{"legacy":{"name":"legacy","authKey":"SECRET_KEY"}}}`)
	require.NoError(t, os.WriteFile(path, data, 0600))
	reg, err := NewRegistry(path, logger.Init())
	require.NoError(t, err)
	reg.rename = func(_, _ string) error {
		return errors.New("injected rename failure")
	}
	err = reg.ScrubCredentials()
	require.Error(t, err)
	result := string(mustReadRegistry(t, path))
	assert.Contains(t, result, "authKey")
	assert.Contains(t, result, "SECRET_KEY")
	assert.True(t, reg.HasLegacyCredentials())
}

func mustReadRegistry(t *testing.T, path string) []byte {
	t.Helper()
	// #nosec G304 -- test path is created under t.TempDir.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestRegistry_ConcurrentWritesReloadUnderLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.json")
	log := logger.Init()
	first, err := NewRegistry(path, log)
	require.NoError(t, err)
	second, err := NewRegistry(path, log)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i, registry := range []*Registry{first, second} {
		wg.Add(1)
		go func(i int, registry *Registry) {
			defer wg.Done()
			api := &API{Name: fmt.Sprintf("api-%d", i), URL: "http://localhost"}
			if err := registry.Register(api); err != nil {
				t.Errorf("register concurrently: %v", err)
			}
		}(i, registry)
	}
	wg.Wait()

	loaded, err := NewRegistry(path, log)
	require.NoError(t, err)
	assert.Len(t, loaded.List(), 2)
}
