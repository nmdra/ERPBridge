package mcp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	// Register pure Go SQLite driver
	_ "modernc.org/sqlite"
)

// Store handles the persistence of Tool resources using SQLite.
type Store struct {
	db *sql.DB
}

// NewStore initializes a new SQLite store at the specified path.
func NewStore(dbPath string) (*Store, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) init() error {
	query := `
	CREATE TABLE IF NOT EXISTS tools (
		name TEXT,
		version TEXT,
		module TEXT,
		is_active INTEGER DEFAULT 1,
		data TEXT,
		created_at DATETIME DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')),
		updated_at DATETIME DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')),
		PRIMARY KEY (name, version)
	);
	CREATE INDEX IF NOT EXISTS idx_tools_module ON tools(module);
	CREATE TABLE IF NOT EXISTS api_tokens (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		scopes TEXT NOT NULL DEFAULT '[]',
		roles TEXT NOT NULL DEFAULT '[]',
		expires_at TEXT,
		revoked_at TEXT,
		created_at TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'NOW'))
	);
	CREATE INDEX IF NOT EXISTS idx_api_tokens_name ON api_tokens(name);
	`
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}

	rows, err := s.db.Query("PRAGMA table_info(tools)")
	if err != nil {
		return fmt.Errorf("inspect tools schema: %w", err)
	}
	hasActiveColumn := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan tools schema: %w", err)
		}
		if name == "is_active" {
			hasActiveColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate tools schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close tools schema: %w", err)
	}
	if !hasActiveColumn {
		if _, err := s.db.Exec("ALTER TABLE tools ADD COLUMN is_active INTEGER DEFAULT 1"); err != nil {
			return fmt.Errorf("migrate tools schema: %w", err)
		}
	}

	return nil
}

// Save stores or updates a tool in the database.
func (s *Store) Save(t *Tool) error {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal tool: %w", err)
	}

	isActive := 1
	if !t.Metadata.IsActive {
		isActive = 0
	}

	query := `
	INSERT INTO tools (name, version, module, is_active, data, updated_at)
	VALUES (?, ?, ?, ?, ?, (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')))
	ON CONFLICT(name, version) DO UPDATE SET
		module = excluded.module,
		is_active = excluded.is_active,
		data = excluded.data,
		updated_at = (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW'));
	`
	_, err = s.db.Exec(query, t.Metadata.Name, t.Metadata.Version, t.Metadata.Module, isActive, string(data))
	if err != nil {
		return fmt.Errorf("save tool: %w", err)
	}
	return nil
}

// List returns all tools stored in the database.
func (s *Store) List() ([]*Tool, error) {
	query := `SELECT data FROM tools ORDER BY name, version DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query tools: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tools []*Tool
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan tool: %w", err)
		}

		var t Tool
		if err := json.Unmarshal([]byte(data), &t); err != nil {
			return nil, fmt.Errorf("unmarshal tool: %w", err)
		}
		tools = append(tools, &t)
	}

	return tools, nil
}

// ListByModule returns every stored tool version in a module, including inactive tools.
func (s *Store) ListByModule(module string) ([]*Tool, error) {
	rows, err := s.db.Query(`SELECT data FROM tools WHERE module = ? ORDER BY name, version DESC`, module)
	if err != nil {
		return nil, fmt.Errorf("query module tools: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tools []*Tool
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan module tool: %w", err)
		}
		var tool Tool
		if err := json.Unmarshal([]byte(data), &tool); err != nil {
			return nil, fmt.Errorf("unmarshal module tool: %w", err)
		}
		tools = append(tools, &tool)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate module tools: %w", err)
	}
	return tools, nil
}

// GetStateHash returns a string representing the current state of the tools table.
// This is used to short-circuit reconciliation if no changes have occurred.
func (s *Store) GetStateHash() (string, error) {
	rows, err := s.db.Query(`SELECT name, version, is_active, updated_at FROM tools ORDER BY name, version`)
	if err != nil {
		return "", fmt.Errorf("get state hash: %w", err)
	}
	defer func() { _ = rows.Close() }()

	hash := sha256.New()
	for rows.Next() {
		var name, version string
		var updatedAt sql.NullString
		var isActive int
		if err := rows.Scan(&name, &version, &isActive, &updatedAt); err != nil {
			return "", fmt.Errorf("scan state hash row: %w", err)
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%d\x00%s\x00", name, version, isActive, updatedAt.String)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate state hash rows: %w", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Get retrieves a specific version of a tool.
func (s *Store) Get(name, version string) (*Tool, error) {
	query := `SELECT data FROM tools WHERE name = ? AND version = ?`
	var data string
	err := s.db.QueryRow(query, name, version).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tool %s@%s not found", name, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get tool: %w", err)
	}

	var t Tool
	if err := json.Unmarshal([]byte(data), &t); err != nil {
		return nil, fmt.Errorf("unmarshal tool: %w", err)
	}
	return &t, nil
}

// Delete performs a soft-delete by marking the tool as inactive.
func (s *Store) Delete(name, version string) error {
	// First get the tool to ensure it exists and to update its JSON data
	t, err := s.Get(name, version)
	if err != nil {
		return err
	}

	t.Metadata.IsActive = false
	return s.Save(t)
}

// HardDelete permanently removes a tool from the database.
func (s *Store) HardDelete(name, version string) error {
	query := `DELETE FROM tools WHERE name = ? AND version = ?`
	_, err := s.db.Exec(query, name, version)
	if err != nil {
		return fmt.Errorf("hard delete tool: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
