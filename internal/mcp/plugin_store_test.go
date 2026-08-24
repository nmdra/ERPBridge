package mcp

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginStore_CRUDAndExactBindingLookup(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "plugins.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	plugin := validPluginForTest("http://plugin.example.test")
	require.NoError(t, store.SavePlugin(&plugin))
	other := validPluginForTest("https://other.example.test")
	other.Metadata.Name = "other-plugin"
	require.NoError(t, store.SavePlugin(&other))

	got, err := store.GetPlugin(plugin.Metadata.Name, plugin.Metadata.Version)
	require.NoError(t, err)
	require.Equal(t, plugin.Spec.Endpoint, got.Spec.Endpoint)

	plugins, err := store.ListPlugins()
	require.NoError(t, err)
	require.Len(t, plugins, 2)

	binding := validPluginBindingForTest()
	require.NoError(t, store.SavePluginBinding(&binding))
	gotBinding, err := store.GetPluginBinding(binding.Metadata.Name)
	require.NoError(t, err)
	require.Equal(t, binding.Spec.ToolRef, gotBinding.Spec.ToolRef)

	allForTool, err := store.ListPluginBindingsForTool(binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)
	require.NoError(t, err)
	require.Len(t, allForTool, 1)

	missingTool, err := store.ListPluginBindingsForTool("other-tool", testVersion100)
	require.NoError(t, err)
	require.Empty(t, missingTool)

	require.NoError(t, store.DeletePluginBinding(binding.Metadata.Name))
	gotBinding, err = store.GetPluginBinding(binding.Metadata.Name)
	require.NoError(t, err)
	require.False(t, gotBinding.Metadata.IsActive)

	require.NoError(t, store.HardDeletePluginBinding(binding.Metadata.Name))
	_, err = store.GetPluginBinding(binding.Metadata.Name)
	require.Error(t, err)

	require.NoError(t, store.DeletePlugin(plugin.Metadata.Name, plugin.Metadata.Version))
	got, err = store.GetPlugin(plugin.Metadata.Name, plugin.Metadata.Version)
	require.NoError(t, err)
	require.False(t, got.Metadata.IsActive)

	require.NoError(t, store.HardDeletePlugin(plugin.Metadata.Name, plugin.Metadata.Version))
	_, err = store.GetPlugin(plugin.Metadata.Name, plugin.Metadata.Version)
	require.Error(t, err)
}

func TestPluginStore_HardDeletePluginRejectsActiveBinding(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "references.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	plugin := validPluginForTest("http://plugin.example.test")
	require.NoError(t, store.SavePlugin(&plugin))
	binding := validPluginBindingForTest()
	require.NoError(t, store.SavePluginBinding(&binding))

	otherVersion := plugin
	otherVersion.Metadata.Version = "2.0.0"
	require.NoError(t, store.SavePlugin(&otherVersion))
	require.NoError(t, store.HardDeletePlugin(otherVersion.Metadata.Name, otherVersion.Metadata.Version))

	err = store.HardDeletePlugin(plugin.Metadata.Name, plugin.Metadata.Version)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPluginHasActiveBindings))

	require.NoError(t, store.DeletePlugin(plugin.Metadata.Name, plugin.Metadata.Version))
	activeBindings, err := store.ListActivePluginBindingsForTool(binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)
	require.NoError(t, err)
	require.Empty(t, activeBindings)
	storedBindings, err := store.ListPluginBindingsForTool(binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)
	require.NoError(t, err)
	require.Len(t, storedBindings, 1)

	require.NoError(t, store.DeletePluginBinding(binding.Metadata.Name))
	require.NoError(t, store.HardDeletePlugin(plugin.Metadata.Name, plugin.Metadata.Version))
	retainedBinding, err := store.GetPluginBinding(binding.Metadata.Name)
	require.NoError(t, err)
	require.False(t, retainedBinding.Metadata.IsActive)
}

func TestPluginStore_StateHashIncludesAllResources(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "hash.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	before, err := store.GetStateHash()
	require.NoError(t, err)

	plugin := validPluginForTest("http://plugin.example.test")
	require.NoError(t, store.SavePlugin(&plugin))
	afterPlugin, err := store.GetStateHash()
	require.NoError(t, err)
	require.NotEqual(t, before, afterPlugin)

	plugin.Spec.Endpoint = "http://plugin-updated.example.test"
	require.NoError(t, store.SavePlugin(&plugin))
	afterPluginUpdate, err := store.GetStateHash()
	require.NoError(t, err)
	require.NotEqual(t, afterPlugin, afterPluginUpdate)

	binding := validPluginBindingForTest()
	require.NoError(t, store.SavePluginBinding(&binding))
	afterBinding, err := store.GetStateHash()
	require.NoError(t, err)
	require.NotEqual(t, afterPluginUpdate, afterBinding)

	binding.Spec.Config[pluginTestModeKey] = "strict"
	require.NoError(t, store.SavePluginBinding(&binding))
	afterBindingUpdate, err := store.GetStateHash()
	require.NoError(t, err)
	require.NotEqual(t, afterBinding, afterBindingUpdate)

	require.NoError(t, store.DeletePluginBinding(binding.Metadata.Name))
	afterDelete, err := store.GetStateHash()
	require.NoError(t, err)
	require.NotEqual(t, afterBinding, afterDelete)
}

func TestPluginStore_InitializesLegacyToolDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = legacy.Exec(`CREATE TABLE tools (
		name TEXT,
		version TEXT,
		module TEXT,
		data TEXT,
		created_at DATETIME,
		updated_at DATETIME,
		PRIMARY KEY (name, version)
	)`)
	require.NoError(t, err)
	_, err = legacy.Exec(`INSERT INTO tools (name, version, module, data) VALUES ('legacy', '1.0.0', 'legacy-module', '{"metadata":{"name":"legacy","version":"1.0.0","module":"legacy-module"}}')`)
	require.NoError(t, err)
	require.NoError(t, legacy.Close())

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	for _, table := range []string{"plugins", "plugin_bindings"} {
		var name string
		err = store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		require.NoError(t, err)
		require.Equal(t, table, name)
	}
	plugins, err := store.ListPlugins()
	require.NoError(t, err)
	require.Empty(t, plugins)
	legacyTool, err := store.Get("legacy", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "legacy", legacyTool.Metadata.Name)
	require.Equal(t, "legacy-module", legacyTool.Metadata.Module)
}

func TestPluginStore_DBClosed(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "closed.db"))
	require.NoError(t, err)
	require.NoError(t, store.Close())

	plugin := validPluginForTest("http://plugin.example.test")
	require.Error(t, store.SavePlugin(&plugin))
	_, err = store.ListPlugins()
	require.Error(t, err)
	_, err = store.GetPlugin("missing", testVersion100)
	require.Error(t, err)
	require.Error(t, store.DeletePlugin("missing", testVersion100))
	require.Error(t, store.HardDeletePlugin("missing", testVersion100))
}
