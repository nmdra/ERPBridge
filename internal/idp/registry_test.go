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

func TestContextRegistryPathValidatesAndScopesByContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := ContextRegistryPath("staging")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".bridgectl", "registries", "staging.json"), path)
	_, err = ContextRegistryPath("../escape")
	assert.Error(t, err)
	_, err = ContextRegistryPath("nested/context")
	assert.Error(t, err)
}

func TestContextRegistryDoesNotIgnoreGlobalRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	global := filepath.Join(home, ".bridgectl", "registry.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(global), 0700))
	require.NoError(t, os.WriteFile(global, []byte(`{"apis":{"legacy":{"name":"legacy"}}}`), 0600))

	_, err := NewRegistryForContext("local", logger.Init())
	assert.ErrorIs(t, err, ErrLegacyRegistry)
}

func TestContextRegistriesIsolateSameAPIName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	first, err := NewRegistryForContext("first", logger.Init())
	require.NoError(t, err)
	second, err := NewRegistryForContext("second", logger.Init())
	require.NoError(t, err)
	require.NoError(t, first.Register(&API{Name: "shared", URL: "http://first"}))
	require.NoError(t, second.Register(&API{Name: "shared", URL: "http://second"}))
	firstAPI, ok := first.Get("shared")
	require.True(t, ok)
	secondAPI, ok := second.Get("shared")
	require.True(t, ok)
	assert.Equal(t, "http://first", firstAPI.URL)
	assert.Equal(t, "http://second", secondAPI.URL)
}

func TestRegistryRegisterRejectsDuplicateUnlessForced(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"), logger.Init())
	require.NoError(t, err)
	first := &API{Name: "same", URL: "http://one"}
	require.NoError(t, reg.Register(first))
	second := &API{Name: "same", URL: "http://two"}
	err = reg.Register(second)
	assert.ErrorIs(t, err, ErrRegistryConflict)
	stored, ok := reg.Get("same")
	require.True(t, ok)
	assert.Equal(t, "http://one", stored.URL)

	replaced, err := reg.RegisterWithOptions(second, true)
	require.NoError(t, err)
	assert.True(t, replaced)
	stored, ok = reg.Get("same")
	require.True(t, ok)
	assert.Equal(t, "http://two", stored.URL)
}

func TestRegistryListIsSorted(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "registry.json"), logger.Init())
	require.NoError(t, err)
	require.NoError(t, reg.Register(&API{Name: "z"}))
	require.NoError(t, reg.Register(&API{Name: "a"}))
	list := reg.List()
	require.Len(t, list, 2)
	assert.Equal(t, "a", list[0].Name)
	assert.Equal(t, "z", list[1].Name)
}

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

func TestRegistry_MigrateGlobalRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	global := filepath.Join(home, ".bridgectl", "registry.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(global), 0700))
	require.NoError(t, os.WriteFile(global, []byte(`{"apis":{"legacy":{"name":"legacy","url":"https://erp"}}}`), 0600))

	migrated, err := MigrateGlobalRegistry("local", false, logger.Init())
	require.NoError(t, err)
	assert.Equal(t, 1, migrated)
	_, err = os.Stat(global)
	assert.ErrorIs(t, err, os.ErrNotExist)
	target, err := NewRegistryForContext("local", logger.Init())
	require.NoError(t, err)
	_, ok := target.Get("legacy")
	assert.True(t, ok)
}

func TestRegistry_MigrateRefusesLegacyAndCollisions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	global := filepath.Join(home, ".bridgectl", "registry.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(global), 0700))
	require.NoError(t, os.WriteFile(global, []byte(`{"apis":{"same":{"name":"same","authToken":"SECRET"}}}`), 0600))
	_, err := MigrateGlobalRegistry("local", false, logger.Init())
	assert.ErrorIs(t, err, ErrLegacyCredentials)

	require.NoError(t, (&Registry{path: global, log: logger.Init(), APIs: map[string]API{}}).ScrubCredentials())
	targetPath, pathErr := ContextRegistryPath("local")
	require.NoError(t, pathErr)
	target, err := NewRegistry(targetPath, logger.Init())
	require.NoError(t, err)
	require.NoError(t, target.Register(&API{Name: "same", URL: "target"}))
	_, err = MigrateGlobalRegistry("local", false, logger.Init())
	assert.ErrorIs(t, err, ErrRegistryConflict)
}

func TestRegistry_ScrubAllRegistries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".bridgectl", "registries")
	require.NoError(t, os.MkdirAll(dir, 0700))
	secret := "SECRET_SCRUB_ALL"
	global := filepath.Join(home, ".bridgectl", "registry.json")
	require.NoError(t, os.WriteFile(global, []byte(`{"apis":{"global":{"name":"global","authToken":"`+secret+`"}}}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "local.json"), []byte(`{"apis":{"local":{"name":"local","authKey":"`+secret+`"}}}`), 0600))
	require.NoError(t, ScrubAllRegistries(logger.Init()))
	for _, path := range []string{global, filepath.Join(dir, "local.json")} {
		data, readErr := os.ReadFile(path) // #nosec G304 -- test paths are under t.TempDir.
		require.NoError(t, readErr)
		assert.NotContains(t, string(data), secret)
		assert.NotContains(t, string(data), "authToken")
		assert.NotContains(t, string(data), "authKey")
	}
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
