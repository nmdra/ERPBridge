package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseToolIdentifier(t *testing.T) {
	name, version := ParseToolIdentifier("mytool@1.2.3")
	assert.Equal(t, "mytool", name)
	assert.Equal(t, "1.2.3", version)

	name, version = ParseToolIdentifier("mytool")
	assert.Equal(t, "mytool", name)
	assert.Equal(t, "", version)
}

func TestToolRegistry_Add(t *testing.T) {
	registry := NewToolRegistry()

	// Valid add
	err := registry.Add(&Tool{
		Metadata: Metadata{Name: testTool1, Version: testVersion100, IsActive: true},
	})
	assert.NoError(t, err)

	// Invalid version
	err = registry.Add(&Tool{
		Metadata: Metadata{Name: testTool2, Version: "invalid"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid semver version")
}

func TestToolRegistry_Resolve(t *testing.T) {
	registry := NewToolRegistry()

	tools := []*Tool{
		{Metadata: Metadata{Name: testTool1, Version: testVersion100, IsActive: true}},
		{Metadata: Metadata{Name: testTool1, Version: testVersion110, IsActive: true}},
		{Metadata: Metadata{Name: testTool1, Version: "2.0.0-beta.1", IsActive: true}}, // prerelease
		{Metadata: Metadata{Name: testTool2, Version: "1.0.0-rc.1", IsActive: true}},   // only prerelease
		{Metadata: Metadata{Name: testTool2, Version: "1.0.0-rc.2", IsActive: true}},   // higher prerelease
	}

	for _, tool := range tools {
		require.NoError(t, registry.Add(tool))
	}
	require.NoError(t, registry.SetServing(testTool1, testVersion110))
	require.NoError(t, registry.SetServing(testTool2, "1.0.0-rc.2"))

	t.Run("Exact version", func(t *testing.T) {
		tool, err := registry.Resolve(testTool1, testVersion100)
		require.NoError(t, err)
		assert.Equal(t, testVersion100, tool.Metadata.Version)
	})

	t.Run("Semver constraint", func(t *testing.T) {
		tool, err := registry.Resolve(testTool1, "^1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "1.1.0", tool.Metadata.Version)
	})

	t.Run("Constraint no match", func(t *testing.T) {
		_, err := registry.Resolve("tool1", "^3.0.0")
		assert.Error(t, err)
	})

	t.Run("Invalid constraint", func(t *testing.T) {
		_, err := registry.Resolve("tool1", "invalid-constraint")
		assert.Error(t, err)
	})

	t.Run("Default to operator-selected serving revision", func(t *testing.T) {
		tool, err := registry.Resolve("tool1", "")
		require.NoError(t, err)
		assert.Equal(t, "1.1.0", tool.Metadata.Version)
	})

	t.Run("Serving revision may be a prerelease", func(t *testing.T) {
		tool, err := registry.Resolve("tool2", "")
		require.NoError(t, err)
		assert.Equal(t, "1.0.0-rc.2", tool.Metadata.Version)
	})

	t.Run("Not found", func(t *testing.T) {
		_, err := registry.Resolve("missing", "")
		assert.Error(t, err)
	})
}

func TestToolRegistry_ListStable(t *testing.T) {
	registry := NewToolRegistry()

	require.NoError(t, registry.Add(&Tool{Metadata: Metadata{Name: "b_tool", Version: testVersion100, IsActive: true, IsServing: true}}))
	require.NoError(t, registry.Add(&Tool{Metadata: Metadata{Name: "b_tool", Version: testVersion110, IsActive: true}}))
	require.NoError(t, registry.Add(&Tool{Metadata: Metadata{Name: "a_tool", Version: "2.0.0", IsActive: true, IsServing: true}}))

	stable := registry.ListStable()
	require.Len(t, stable, 2)
	assert.Equal(t, "a_tool", stable[0].Metadata.Name)
	assert.Equal(t, "2.0.0", stable[0].Metadata.Version)
	assert.Equal(t, "b_tool", stable[1].Metadata.Name)
	assert.Equal(t, testVersion100, stable[1].Metadata.Version)
}

func TestToolRegistry_ActiveNonServingAndImmutableSnapshot(t *testing.T) {
	registry := NewToolRegistry()
	v1 := &Tool{Metadata: Metadata{Name: testTool1, Version: testVersion100, IsActive: true, IsServing: true}}
	v1.Spec.Description.Short = "v1"
	v2 := &Tool{Metadata: Metadata{Name: testTool1, Version: testVersion110, IsActive: true}}
	v2.Spec.Description.Short = "v2"
	require.NoError(t, registry.Add(v1))
	require.NoError(t, registry.Add(v2))

	servingSnapshot, err := registry.Resolve(testTool1, "")
	require.NoError(t, err)
	require.Equal(t, testVersion100, servingSnapshot.Metadata.Version)

	exactV2, err := registry.Resolve(testTool1, testVersion110)
	require.NoError(t, err)
	require.Equal(t, "v2", exactV2.Spec.Description.Short)

	require.NoError(t, registry.SetServing(testTool1, testVersion110))
	registry.Remove(testTool1, testVersion100)
	require.True(t, servingSnapshot.Metadata.IsActive)
	require.True(t, servingSnapshot.Metadata.IsServing)
	require.Equal(t, "v1", servingSnapshot.Spec.Description.Short)

	current, err := registry.Resolve(testTool1, "")
	require.NoError(t, err)
	require.Equal(t, testVersion110, current.Metadata.Version)
}

func TestToolRegistry_ListAll(t *testing.T) {
	registry := NewToolRegistry()

	require.NoError(t, registry.Add(&Tool{Metadata: Metadata{Name: testTool1, Version: testVersion100, IsActive: true}}))
	require.NoError(t, registry.Add(&Tool{Metadata: Metadata{Name: testTool1, Version: testVersion110, IsActive: true}}))

	all := registry.ListAll()
	assert.Len(t, all, 2)
}

func TestToolRegistry_Remove(t *testing.T) {
	registry := NewToolRegistry()

	require.NoError(t, registry.Add(&Tool{Metadata: Metadata{Name: testTool1, Version: testVersion100, IsActive: true}}))
	require.NoError(t, registry.Add(&Tool{Metadata: Metadata{Name: testTool1, Version: testVersion110, IsActive: true}}))

	// Remove one version
	registry.Remove(testTool1, testVersion100)

	_, err := registry.Resolve(testTool1, testVersion100)
	assert.Error(t, err)

	_, err = registry.Resolve(testTool1, testVersion110)
	assert.NoError(t, err)

	// Remove remaining version
	registry.Remove(testTool1, testVersion110)
	_, err = registry.Resolve(testTool1, "")
	assert.Error(t, err)

	// Safe to remove non-existent
	registry.Remove("missing", testVersion100)
}
