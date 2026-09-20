package mcp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	// Register pure Go SQLite driver
	_ "modernc.org/sqlite"
)

// Store handles the persistence of Tool resources using SQLite.
type Store struct {
	db      *sql.DB
	writeMu sync.Mutex
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

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
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
	CREATE TABLE IF NOT EXISTS plugins (
		name TEXT,
		version TEXT,
		is_active INTEGER DEFAULT 1,
		data TEXT,
		created_at DATETIME DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')),
		updated_at DATETIME DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')),
		PRIMARY KEY (name, version)
	);
	CREATE INDEX IF NOT EXISTS idx_plugins_identity ON plugins(name, version);
	CREATE TABLE IF NOT EXISTS plugin_bindings (
		name TEXT PRIMARY KEY,
		plugin_name TEXT NOT NULL,
		plugin_version TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		tool_version TEXT NOT NULL,
		priority INTEGER DEFAULT 0,
		is_active INTEGER DEFAULT 1,
		data TEXT,
		created_at DATETIME DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW')),
		updated_at DATETIME DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW'))
	);
	CREATE INDEX IF NOT EXISTS idx_plugin_bindings_plugin_ref
		ON plugin_bindings(plugin_name, plugin_version, is_active);
	CREATE INDEX IF NOT EXISTS idx_plugin_bindings_tool_ref
		ON plugin_bindings(tool_name, tool_version, is_active, priority);
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

	return s.migrateLegacyTools()
}

// Save stores or updates runtime lifecycle state for a tool. Control-plane
// admission must use Admit so executable content cannot replace an admitted
// name and version.
func (s *Store) Save(t *Tool) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.saveTool(t)
}

type toolSQLExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (s *Store) saveTool(t *Tool) error {
	return saveToolWith(s.db, t)
}

func saveToolWith(execer toolSQLExecer, t *Tool) error {
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
	_, err = execer.Exec(query, t.Metadata.Name, t.Metadata.Version, t.Metadata.Module, isActive, string(data))
	if err != nil {
		return fmt.Errorf("save tool: %w", err)
	}
	return nil
}

// Admit atomically creates an immutable admitted revision or reapplies the
// identical content. A changed resource cannot replace the same name/version.
func (s *Store) Admit(t *Tool, actor string) error {
	requestedServing := t.Metadata.IsServing
	digest, err := CanonicalToolDigest(t)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tool admission: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`SELECT data FROM tools WHERE name = ?`, t.Metadata.Name)
	if err != nil {
		return fmt.Errorf("read admitted tools: %w", err)
	}
	var siblings []*Tool
	var existing *Tool
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan admitted tool: %w", err)
		}
		var stored Tool
		if err := json.Unmarshal([]byte(data), &stored); err != nil {
			_ = rows.Close()
			return fmt.Errorf("unmarshal admitted tool: %w", err)
		}
		siblings = append(siblings, &stored)
		if stored.Metadata.Version == t.Metadata.Version {
			existing = &stored
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close admitted tools: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate admitted tools: %w", err)
	}

	if existing != nil {
		existingDigest, digestErr := CanonicalToolDigest(existing)
		if digestErr != nil {
			return digestErr
		}
		if existingDigest != digest {
			return ErrAdmissionConflict
		}
		t.Metadata.ResourceDigest = existingDigest
		t.Metadata.Admission = existing.Metadata.Admission
		t.Metadata.IsServing = requestedServing || existing.Metadata.IsServing
		if t.Metadata.Admission == nil {
			t.Metadata.Admission = newAdmissionRecord(actor, time.Now())
		}
	} else {
		t.Metadata.ResourceDigest = digest
		t.Metadata.Admission = newAdmissionRecord(actor, time.Now())
	}

	hasServing := false
	for _, sibling := range siblings {
		if sibling.Metadata.IsActive && sibling.Metadata.IsServing && sibling.Metadata.Version != t.Metadata.Version {
			hasServing = true
			break
		}
	}
	if !hasServing && (existing == nil || !existing.Metadata.IsServing) {
		t.Metadata.IsServing = true
	}
	if t.Metadata.IsServing {
		for _, sibling := range siblings {
			if sibling.Metadata.Version == t.Metadata.Version || !sibling.Metadata.IsServing {
				continue
			}
			sibling.Metadata.IsServing = false
			if err := saveToolWith(tx, sibling); err != nil {
				return err
			}
		}
	}
	if err := saveToolWith(tx, t); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tool admission: %w", err)
	}
	return nil
}

func (s *Store) migrateLegacyTools() error {
	rows, err := s.db.Query(`SELECT name, version, module, is_active, data FROM tools ORDER BY name, version`)
	if err != nil {
		return fmt.Errorf("query legacy tools: %w", err)
	}
	var tools []*Tool
	for rows.Next() {
		var name, version, module, data string
		var isActive int
		if err := rows.Scan(&name, &version, &module, &isActive, &data); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan legacy tool: %w", err)
		}
		var tool Tool
		if err := json.Unmarshal([]byte(data), &tool); err != nil {
			_ = rows.Close()
			return fmt.Errorf("unmarshal legacy tool: %w", err)
		}
		if tool.Metadata.Name == "" {
			tool.Metadata.Name = name
		}
		if tool.Metadata.Version == "" {
			tool.Metadata.Version = version
		}
		if tool.Metadata.Module == "" {
			tool.Metadata.Module = module
		}
		tool.Metadata.IsActive = isActive != 0
		tools = append(tools, &tool)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy tools: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy tools: %w", err)
	}
	if len(tools) == 0 {
		return nil
	}

	byName := make(map[string][]*Tool)
	legacyNames := make(map[string]bool)
	changed := false
	now := time.Now()
	for _, tool := range tools {
		if tool.Metadata.ResourceDigest == "" || tool.Metadata.Admission == nil {
			legacyNames[tool.Metadata.Name] = true
		}
		if tool.Spec.Execution.Endpoint != "" && len(tool.Spec.Execution.ApprovedOrigins) == 0 {
			if err := bindApprovedOrigins(tool); err != nil {
				return fmt.Errorf("migrate approved origins for %s@%s: %w", tool.Metadata.Name, tool.Metadata.Version, err)
			}
			changed = true
		}
		if tool.Metadata.ResourceDigest == "" || tool.Metadata.Admission == nil {
			digest, err := CanonicalToolDigest(tool)
			if err != nil {
				return fmt.Errorf("digest legacy tool %s@%s: %w", tool.Metadata.Name, tool.Metadata.Version, err)
			}
			tool.Metadata.ResourceDigest = digest
			if tool.Metadata.Admission == nil {
				tool.Metadata.Admission = newAdmissionRecord("legacy-migration", now)
			}
			changed = true
		}
		if tool.Metadata.IsActive {
			byName[tool.Metadata.Name] = append(byName[tool.Metadata.Name], tool)
		}
	}
	for name, active := range byName {
		if !legacyNames[name] {
			continue
		}
		servingCount := 0
		for _, tool := range active {
			if tool.Metadata.IsServing {
				servingCount++
			}
		}
		if servingCount == 1 {
			continue
		}
		selected := latestLegacyRevision(active)
		for _, tool := range active {
			want := tool == selected
			if tool.Metadata.IsServing != want {
				tool.Metadata.IsServing = want
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin legacy tool migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, tool := range tools {
		if err := saveToolWith(tx, tool); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy tool migration: %w", err)
	}
	return nil
}

func latestLegacyRevision(tools []*Tool) *Tool {
	var latestStable *semver.Version
	var latestStableTool *Tool
	var latestAny *semver.Version
	var latestAnyTool *Tool
	for _, tool := range tools {
		version, err := semver.NewVersion(tool.Metadata.Version)
		if err != nil {
			continue
		}
		if latestAny == nil || version.GreaterThan(latestAny) {
			latestAny, latestAnyTool = version, tool
		}
		if version.Prerelease() == "" && (latestStable == nil || version.GreaterThan(latestStable)) {
			latestStable, latestStableTool = version, tool
		}
	}
	if latestStableTool != nil {
		return latestStableTool
	}
	return latestAnyTool
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

// GetStateHash returns a hash of all persisted declarative resources. It is
// used to short-circuit reconciliation when no tool, plugin, or binding state
// has changed.
func (s *Store) GetStateHash() (string, error) {
	rows, err := s.db.Query(`
		SELECT resource_kind, resource_name, resource_version, is_active, updated_at, resource_data
		FROM (
			SELECT 'tool' AS resource_kind, name AS resource_name, version AS resource_version,
				is_active, updated_at, COALESCE(data, '') AS resource_data
			FROM tools
			UNION ALL
			SELECT 'plugin' AS resource_kind, name AS resource_name, version AS resource_version,
				is_active, updated_at, COALESCE(data, '') AS resource_data
			FROM plugins
			UNION ALL
			SELECT 'binding' AS resource_kind, name AS resource_name,
				plugin_name || '@' || plugin_version || '|' || tool_name || '@' || tool_version AS resource_version,
				is_active, updated_at, COALESCE(data, '') AS resource_data
			FROM plugin_bindings
		)
		ORDER BY resource_kind, resource_name, resource_version
	`)
	if err != nil {
		return "", fmt.Errorf("get state hash: %w", err)
	}
	defer func() { _ = rows.Close() }()

	hash := sha256.New()
	for rows.Next() {
		var kind, name, version, data string
		var updatedAt sql.NullString
		var isActive int
		if err := rows.Scan(&kind, &name, &version, &isActive, &updatedAt, &data); err != nil {
			return "", fmt.Errorf("scan state hash row: %w", err)
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00", kind, name, version, isActive, updatedAt.String, data)
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
	t.Metadata.IsServing = false
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
