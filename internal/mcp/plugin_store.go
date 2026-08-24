package mcp

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrPluginHasActiveBindings prevents deleting a plugin that is still
	// referenced by an active binding.
	ErrPluginHasActiveBindings = errors.New("plugin has active bindings")
	// ErrPluginBindingNotFound indicates that a named binding does not exist.
	ErrPluginBindingNotFound = errors.New("plugin binding not found")
)

// SavePlugin stores or updates one exact plugin version.
func (s *Store) SavePlugin(plugin *Plugin) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.savePlugin(plugin)
}

func (s *Store) savePlugin(plugin *Plugin) error {
	if plugin == nil {
		return errors.New("plugin is required")
	}
	data, err := json.Marshal(plugin)
	if err != nil {
		return fmt.Errorf("marshal plugin: %w", err)
	}
	isActive := 0
	if plugin.Metadata.IsActive {
		isActive = 1
	}
	_, err = s.db.Exec(`
		INSERT INTO plugins (name, version, is_active, data, updated_at)
		VALUES (?, ?, ?, ?, (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')))
		ON CONFLICT(name, version) DO UPDATE SET
			is_active = excluded.is_active,
			data = excluded.data,
			updated_at = (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW'))
	`, plugin.Metadata.Name, plugin.Metadata.Version, isActive, string(data))
	if err != nil {
		return fmt.Errorf("save plugin: %w", err)
	}
	return nil
}

// ListPlugins returns all persisted plugin versions, including inactive rows.
func (s *Store) ListPlugins() ([]*Plugin, error) {
	rows, err := s.db.Query(`SELECT data FROM plugins ORDER BY name, version DESC`)
	if err != nil {
		return nil, fmt.Errorf("query plugins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var plugins []*Plugin
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan plugin: %w", err)
		}
		var plugin Plugin
		if err := json.Unmarshal([]byte(data), &plugin); err != nil {
			return nil, fmt.Errorf("unmarshal plugin: %w", err)
		}
		plugins = append(plugins, &plugin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugins: %w", err)
	}
	return plugins, nil
}

// GetPlugin retrieves one exact plugin version.
func (s *Store) GetPlugin(name, version string) (*Plugin, error) {
	var data string
	err := s.db.QueryRow(`SELECT data FROM plugins WHERE name = ? AND version = ?`, name, version).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("plugin %s@%s not found", name, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get plugin: %w", err)
	}
	var plugin Plugin
	if err := json.Unmarshal([]byte(data), &plugin); err != nil {
		return nil, fmt.Errorf("unmarshal plugin: %w", err)
	}
	return &plugin, nil
}

// DeletePlugin performs a soft delete and retains the plugin definition.
func (s *Store) DeletePlugin(name, version string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	plugin, err := s.GetPlugin(name, version)
	if err != nil {
		return err
	}
	plugin.Metadata.IsActive = false
	return s.savePlugin(plugin)
}

// HardDeletePlugin permanently removes a plugin when no active binding uses it.
func (s *Store) HardDeletePlugin(name, version string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM plugins
		WHERE name = ? AND version = ?
		  AND NOT EXISTS (
			SELECT 1 FROM plugin_bindings
			WHERE plugin_name = ? AND plugin_version = ? AND is_active = 1
		  )
	`, name, version, name, version)
	if err != nil {
		return fmt.Errorf("hard delete plugin: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("inspect hard-deleted plugin: %w", err)
	} else if affected == 0 {
		var activeBindings int
		if err := s.db.QueryRow(`
			SELECT COUNT(*) FROM plugin_bindings
			WHERE plugin_name = ? AND plugin_version = ? AND is_active = 1
		`, name, version).Scan(&activeBindings); err != nil {
			return fmt.Errorf("check plugin bindings: %w", err)
		}
		if activeBindings > 0 {
			return fmt.Errorf("%w: %s@%s", ErrPluginHasActiveBindings, name, version)
		}
	}
	return nil
}

// SavePluginBinding stores or updates one named binding.
func (s *Store) SavePluginBinding(binding *PluginBinding) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.savePluginBinding(binding)
}

func (s *Store) savePluginBinding(binding *PluginBinding) error {
	if binding == nil {
		return errors.New("plugin binding is required")
	}
	data, err := json.Marshal(binding)
	if err != nil {
		return fmt.Errorf("marshal plugin binding: %w", err)
	}
	isActive := 0
	if binding.Metadata.IsActive {
		isActive = 1
	}
	_, err = s.db.Exec(`
		INSERT INTO plugin_bindings (
			name, plugin_name, plugin_version, tool_name, tool_version,
			priority, is_active, data, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')))
		ON CONFLICT(name) DO UPDATE SET
			plugin_name = excluded.plugin_name,
			plugin_version = excluded.plugin_version,
			tool_name = excluded.tool_name,
			tool_version = excluded.tool_version,
			priority = excluded.priority,
			is_active = excluded.is_active,
			data = excluded.data,
			updated_at = (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW'))
	`, binding.Metadata.Name, binding.Spec.PluginRef.Name, binding.Spec.PluginRef.Version,
		binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version, binding.Spec.Priority, isActive, string(data))
	if err != nil {
		return fmt.Errorf("save plugin binding: %w", err)
	}
	return nil
}

// ListPluginBindings returns all persisted bindings, including inactive rows.
func (s *Store) ListPluginBindings() ([]*PluginBinding, error) {
	rows, err := s.db.Query(`SELECT data FROM plugin_bindings ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query plugin bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bindings []*PluginBinding
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan plugin binding: %w", err)
		}
		var binding PluginBinding
		if err := json.Unmarshal([]byte(data), &binding); err != nil {
			return nil, fmt.Errorf("unmarshal plugin binding: %w", err)
		}
		bindings = append(bindings, &binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugin bindings: %w", err)
	}
	return bindings, nil
}

// GetPluginBinding retrieves one named binding.
func (s *Store) GetPluginBinding(name string) (*PluginBinding, error) {
	var data string
	err := s.db.QueryRow(`SELECT data FROM plugin_bindings WHERE name = ?`, name).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrPluginBindingNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("get plugin binding: %w", err)
	}
	var binding PluginBinding
	if err := json.Unmarshal([]byte(data), &binding); err != nil {
		return nil, fmt.Errorf("unmarshal plugin binding: %w", err)
	}
	return &binding, nil
}

// ListPluginBindingsForTool returns bindings for one exact tool version,
// including inactive rows retained by soft deletion.
func (s *Store) ListPluginBindingsForTool(name, version string) ([]*PluginBinding, error) {
	rows, err := s.db.Query(`
		SELECT data FROM plugin_bindings
		WHERE tool_name = ? AND tool_version = ?
		ORDER BY priority, name
	`, name, version)
	if err != nil {
		return nil, fmt.Errorf("query plugin bindings for tool: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bindings []*PluginBinding
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan plugin binding for tool: %w", err)
		}
		var binding PluginBinding
		if err := json.Unmarshal([]byte(data), &binding); err != nil {
			return nil, fmt.Errorf("unmarshal plugin binding for tool: %w", err)
		}
		bindings = append(bindings, &binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugin bindings for tool: %w", err)
	}
	return bindings, nil
}

// ListActivePluginBindingsForTool returns only active bindings whose exact
// plugin version is also active.
func (s *Store) ListActivePluginBindingsForTool(name, version string) ([]*PluginBinding, error) {
	rows, err := s.db.Query(`
		SELECT b.data
		FROM plugin_bindings AS b
		JOIN plugins AS p
		  ON p.name = b.plugin_name AND p.version = b.plugin_version
		WHERE b.tool_name = ? AND b.tool_version = ?
		  AND b.is_active = 1 AND p.is_active = 1
		ORDER BY b.priority, b.name
	`, name, version)
	if err != nil {
		return nil, fmt.Errorf("query active plugin bindings for tool: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bindings []*PluginBinding
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan active plugin binding for tool: %w", err)
		}
		var binding PluginBinding
		if err := json.Unmarshal([]byte(data), &binding); err != nil {
			return nil, fmt.Errorf("unmarshal active plugin binding for tool: %w", err)
		}
		bindings = append(bindings, &binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active plugin bindings for tool: %w", err)
	}
	return bindings, nil
}

// DeletePluginBinding performs a soft delete and retains the binding.
func (s *Store) DeletePluginBinding(name string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	binding, err := s.GetPluginBinding(name)
	if err != nil {
		return err
	}
	binding.Metadata.IsActive = false
	return s.savePluginBinding(binding)
}

// HardDeletePluginBinding permanently removes one named binding.
func (s *Store) HardDeletePluginBinding(name string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if _, err := s.db.Exec(`DELETE FROM plugin_bindings WHERE name = ?`, name); err != nil {
		return fmt.Errorf("hard delete plugin binding: %w", err)
	}
	return nil
}
