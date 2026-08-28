package mcp

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	tool1 := &Tool{
		Metadata: Metadata{
			Name:     testTool1,
			Version:  testVersion100,
			Module:   "mod1",
			IsActive: true,
		},
	}

	tool2 := &Tool{
		Metadata: Metadata{
			Name:     testTool1,
			Version:  testVersion110,
			Module:   "mod1",
			IsActive: true,
		},
	}

	t.Run("Save", func(t *testing.T) {
		err := store.Save(tool1)
		require.NoError(t, err)

		err = store.Save(tool2)
		require.NoError(t, err)

		// Save again to test conflict update
		tool1.Metadata.Module = "mod1-updated"
		err = store.Save(tool1)
		require.NoError(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		res, err := store.Get(testTool1, testVersion100)
		require.NoError(t, err)
		assert.Equal(t, testTool1, res.Metadata.Name)
		assert.Equal(t, "mod1-updated", res.Metadata.Module)

		// Not found
		_, err = store.Get(testTool1, "9.9.9")
		assert.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		tools, err := store.List()
		require.NoError(t, err)
		assert.Len(t, tools, 2)
		// Should be ordered by name, version DESC
		assert.Equal(t, testVersion110, tools[0].Metadata.Version)
		assert.Equal(t, testVersion100, tools[1].Metadata.Version)
	})

	t.Run("GetStateHash", func(t *testing.T) {
		hash, err := store.GetStateHash()
		require.NoError(t, err)
		assert.Len(t, hash, 64)
	})

	t.Run("Delete", func(t *testing.T) {
		err := store.Delete(testTool1, testVersion100)
		require.NoError(t, err)

		tools, err := store.List()
		require.NoError(t, err)
		assert.Len(t, tools, 2) // Remains in DB (soft-delete)

		res, err := store.Get(testTool1, testVersion100)
		require.NoError(t, err)
		assert.False(t, res.Metadata.IsActive)

		hash, err := store.GetStateHash()
		require.NoError(t, err)
		assert.Len(t, hash, 64)
	})

	t.Run("HardDelete", func(t *testing.T) {
		err := store.HardDelete(testTool1, testVersion100)
		require.NoError(t, err)

		tools, err := store.List()
		require.NoError(t, err)
		assert.Len(t, tools, 1) // Physically removed

		_, err = store.Get(testTool1, testVersion100)
		assert.Error(t, err) // Not found
		hash, err := store.GetStateHash()
		require.NoError(t, err)
		assert.Len(t, hash, 64)
	})
}

func TestStore_RoundTripsToolAnnotationsAndGuidance(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metadata.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	destructive := false
	tool := &Tool{
		Metadata: Metadata{Name: "persisted-metadata", Version: testVersion100, IsActive: true},
		Spec: ToolSpec{
			Description: Description{
				WhenToUse:    []string{"Use for metadata reads"},
				WhenNotToUse: []string{"Do not use for writes"},
				Examples:     []string{"Read metadata"},
			},
			Annotations: &ToolAnnotations{DestructiveHint: &destructive},
			Security:    Security{AllowedRoles: []string{"metadata_reader"}},
		},
	}
	require.NoError(t, store.Save(tool))

	loaded, err := store.Get(tool.Metadata.Name, tool.Metadata.Version)
	require.NoError(t, err)
	require.NotNil(t, loaded.Spec.Annotations)
	assert.Equal(t, false, *loaded.Spec.Annotations.DestructiveHint)
	assert.Equal(t, tool.Spec.Description.WhenToUse, loaded.Spec.Description.WhenToUse)
	assert.Equal(t, tool.Spec.Description.WhenNotToUse, loaded.Spec.Description.WhenNotToUse)
	assert.Equal(t, tool.Spec.Description.Examples, loaded.Spec.Description.Examples)
	assert.Equal(t, []string{"metadata_reader"}, loaded.Spec.Security.AllowedRoles)
}

func TestStore_NewStore_Errors(t *testing.T) {
	// Invalid path (directory instead of file)
	tempDir := t.TempDir()

	// Create a read-only directory to force an error on MkdirAll
	readOnlyDir := filepath.Join(tempDir, "readonly")
	err := os.MkdirAll(readOnlyDir, 0500)
	require.NoError(t, err)

	dbPath := filepath.Join(readOnlyDir, "subdir", "test.db")
	_, err = NewStore(dbPath)
	assert.Error(t, err)
}

func TestStore_GetStateHash_Empty(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "empty.db")

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	hash, err := store.GetStateHash()
	require.NoError(t, err)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", hash)
}

func TestStore_NewStore_MigratesMissingActiveColumn(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "legacy.db")

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
	_, err = legacy.Exec(`INSERT INTO tools (name, version, module, data) VALUES ('legacy', '1.0.0', 'mod', '{}')`)
	require.NoError(t, err)
	require.NoError(t, legacy.Close())

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	var active int
	err = store.db.QueryRow(`SELECT is_active FROM tools WHERE name = 'legacy'`).Scan(&active)
	require.NoError(t, err)
	assert.Equal(t, 1, active)
	tools, err := store.List()
	require.NoError(t, err)
	assert.Len(t, tools, 1)
}

func TestStore_GetStateHash_RenameChangesHash(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	require.NoError(t, store.Save(&Tool{Metadata: Metadata{Name: "first", Version: testVersion100, IsActive: true}}))
	first, err := store.GetStateHash()
	require.NoError(t, err)
	require.NoError(t, store.HardDelete("first", testVersion100))
	require.NoError(t, store.Save(&Tool{Metadata: Metadata{Name: "second", Version: testVersion100, IsActive: true}}))
	second, err := store.GetStateHash()
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
}

func TestStore_DBClosed(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(filepath.Join(tempDir, "test.db"))
	require.NoError(t, err)

	_ = store.Close()

	err = store.Save(&Tool{Metadata: Metadata{Name: "t", Version: "1"}})
	assert.Error(t, err)

	_, err = store.List()
	assert.Error(t, err)

	_, err = store.GetStateHash()
	assert.Error(t, err)

	_, err = store.Get("t", "1")
	assert.Error(t, err)

	err = store.Delete("t", "1")
	assert.Error(t, err)
}

func TestStore_Save_MarshalError(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(filepath.Join(tempDir, "test.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ch := make(chan int)
	var outputSchema any = ch
	tool := &Tool{
		Metadata: Metadata{Name: "t", Version: "1"},
		Spec: ToolSpec{
			OutputSchema: &outputSchema,
		},
	}
	err = store.Save(tool)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshal tool")
}

func TestStore_UnmarshalError(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(filepath.Join(tempDir, "test.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	_, err = store.db.Exec(`INSERT INTO tools (name, version, data) VALUES ('bad', '1', 'not-json')`)
	require.NoError(t, err)

	_, err = store.Get("bad", "1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal tool")

	_, err = store.List()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal tool")
}

func TestStore_List_ScanError(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(filepath.Join(tempDir, "test.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	_, err = store.db.Exec(`INSERT INTO tools (name, version, data) VALUES ('null', '1', NULL)`)
	require.NoError(t, err)

	_, err = store.List()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scan tool")
}

func TestStore_NewStore_InitError(t *testing.T) {
	tempDir := t.TempDir()
	// dbPath is a directory, not a file path, so sqlite will fail to open or exec
	_, err := NewStore(tempDir)
	assert.Error(t, err)
}
