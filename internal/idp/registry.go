// Package idp provides API registration and OpenAPI-to-MCP tool schema generation.
package idp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/credentials"
	"github.com/nmdra/ERPBridge/internal/logger"
)

// API represents a registered downstream ERP API endpoint and authentication metadata.
type API struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	Method        string    `json:"method"`
	AuthType      string    `json:"authType"`
	AuthHeader    string    `json:"authHeader,omitempty"`
	CredentialRef string    `json:"credentialRef,omitempty"`
	AuthUsername  string    `json:"authUsername,omitempty"`
	Module        string    `json:"module"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
}

// ErrLegacyCredentials indicates that the registry still contains raw legacy credentials.
var ErrLegacyCredentials = errors.New("registry contains legacy plaintext credentials; run bridgectl api scrub-credentials --yes")

// ErrLegacyRegistry indicates that the old global registry needs explicit migration.
var ErrLegacyRegistry = errors.New("legacy global registry exists; run bridgectl api migrate-registry --context <name> --yes")

// ErrRegistryConflict indicates that an API with the same name already exists.
var ErrRegistryConflict = errors.New("API already exists; use --force to replace it")

// GlobalRegistryPath returns the pre-context-isolation registry path.
func GlobalRegistryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bridgectl", "registry.json")
}

// ContextRegistryPath returns the isolated registry path for a validated context.
func ContextRegistryPath(contextName string) (string, error) {
	if err := config.ValidateContextName(contextName); err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bridgectl", "registries", contextName+".json"), nil
}

// Registry manages storage and retrieval of registered API definitions.
type Registry struct {
	path              string
	log               *slog.Logger
	mu                sync.RWMutex
	APIs              map[string]API `json:"apis"`
	legacyCredentials bool
	rename            func(string, string) error
}

// NewRegistry initializes an API registry from the specified file path or the
// legacy global user-home path. New CLI API commands should use
// NewRegistryForContext instead.
func NewRegistry(path string, rootLog *slog.Logger) (*Registry, error) {
	if path == "" {
		path = GlobalRegistryPath()
	}

	reg := &Registry{
		path: path,
		log:  logger.Component(rootLog, "idp"),
		APIs: make(map[string]API),
	}

	if err := reg.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load registry: %w", err)
	}

	return reg, nil
}

// NewRegistryForContext opens the registry isolated for contextName. It
// refuses to proceed while a legacy global registry exists, so old state is
// never silently ignored.
func NewRegistryForContext(contextName string, rootLog *slog.Logger) (*Registry, error) {
	path, err := ContextRegistryPath(contextName)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(GlobalRegistryPath()); err == nil {
		return nil, ErrLegacyRegistry
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect legacy registry: %w", err)
	}
	return NewRegistry(path, rootLog)
}

func (r *Registry) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *Registry) loadLocked() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	var persisted struct {
		APIs map[string]json.RawMessage `json:"apis"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}
	apis := make(map[string]API, len(persisted.APIs))
	legacy := false
	for name, raw := range persisted.APIs {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return fmt.Errorf("decode API %q: %w", name, err)
		}
		if _, ok := fields["authKey"]; ok {
			legacy = true
		}
		if _, ok := fields["authToken"]; ok {
			legacy = true
		}
		var api API
		if err := json.Unmarshal(raw, &api); err != nil {
			return fmt.Errorf("decode API %q: %w", name, err)
		}
		apis[name] = api
	}
	r.APIs = apis
	r.legacyCredentials = legacy
	return nil
}

func (r *Registry) save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	lock, err := acquireRegistryLock(r.path + ".lock")
	if err != nil {
		return err
	}
	defer lock.release()
	return r.saveLocked()
}

func (r *Registry) saveLocked() error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if r.rename != nil {
		return r.rename(tempPath, r.path)
	}
	return os.Rename(tempPath, r.path)
}

func (r *Registry) withWrite(fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	lock, err := acquireRegistryLock(r.path + ".lock")
	if err != nil {
		return err
	}
	defer lock.release()

	if err := r.loadLocked(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reload registry: %w", err)
	}
	if r.APIs == nil {
		r.APIs = make(map[string]API)
	}
	if r.legacyCredentials {
		return ErrLegacyCredentials
	}
	if err := fn(); err != nil {
		return err
	}
	return r.saveLocked()
}

type registryFileLock struct {
	path string
	file *os.File
}

func acquireRegistryLock(path string) (*registryFileLock, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		// #nosec G304 -- the lock path is derived from the caller-selected registry path.
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			return &registryFileLock{path: path, file: file}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire registry lock: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("acquire registry lock: timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *registryFileLock) release() {
	_ = l.file.Close()
	_ = os.Remove(l.path)
}

// Register adds a new API definition. Existing names are rejected; use
// RegisterWithOptions with force=true for an explicit replacement.
func (r *Registry) Register(api *API) error {
	_, err := r.RegisterWithOptions(api, false)
	return err
}

// RegisterWithOptions registers an API and reports whether it replaced an
// existing definition.
func (r *Registry) RegisterWithOptions(api *API, force bool) (bool, error) {
	if api == nil {
		return false, errors.New("API is required")
	}
	if api.CredentialRef != "" {
		if err := credentials.ValidateReference(api.CredentialRef); err != nil {
			return false, err
		}
	}
	replaced := false
	err := r.withWrite(func() error {
		if _, exists := r.APIs[api.Name]; exists {
			if !force {
				return ErrRegistryConflict
			}
			replaced = true
		}
		if api.ID == "" {
			api.ID = fmt.Sprintf("api-%d", time.Now().UnixNano())
		}
		api.CreatedAt = time.Now()
		api.Status = "active"
		r.APIs[api.Name] = *api

		r.log.Info("API registered",
			slog.String("name", api.Name),
			slog.String("module", api.Module),
			slog.String("url", api.URL),
		)
		return nil
	})
	return replaced, err
}

// HasLegacyCredentials reports whether the loaded registry contains raw legacy fields.
func (r *Registry) HasLegacyCredentials() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.legacyCredentials
}

// List returns a slice of all registered APIs.
func (r *Registry) List() []API {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]API, 0, len(r.APIs))
	for _, api := range r.APIs {
		list = append(list, api)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name == list[j].Name {
			return list[i].ID < list[j].ID
		}
		return list[i].Name < list[j].Name
	})
	return list
}

// Delete removes an API definition by name.
func (r *Registry) Delete(name string) error {
	return r.withWrite(func() error {
		delete(r.APIs, name)
		return nil
	})
}

// SetCredentialRef assigns an environment-backed credential reference to an API.
func (r *Registry) SetCredentialRef(name, ref string) error {
	if err := credentials.ValidateReference(ref); err != nil {
		return err
	}
	return r.withWrite(func() error {
		api, ok := r.APIs[name]
		if !ok {
			return fmt.Errorf("API %q not found", name)
		}
		api.CredentialRef = ref
		r.APIs[name] = api
		return nil
	})
}

// ScrubCredentials removes legacy raw credential fields atomically. A missing
// registry is a successful no-op. It never creates a plaintext backup.
func (r *Registry) ScrubCredentials() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := os.Stat(r.path); os.IsNotExist(err) {
		r.APIs = make(map[string]API)
		r.legacyCredentials = false
		return nil
	}
	lock, err := acquireRegistryLock(r.path + ".lock")
	if err != nil {
		return err
	}
	defer lock.release()

	if err := r.loadLocked(); err != nil {
		if os.IsNotExist(err) {
			r.APIs = make(map[string]API)
			r.legacyCredentials = false
			return nil
		}
		return fmt.Errorf("load registry: %w", err)
	}
	if !r.legacyCredentials {
		return nil
	}
	// saveLocked serializes API, which has no raw credential fields.
	if err := r.saveLocked(); err != nil {
		r.legacyCredentials = true
		return err
	}
	r.legacyCredentials = false
	return nil
}

// ScrubAllRegistries removes legacy credential fields from the global and
// every context-scoped registry. It emits no registry contents or credential
// values in errors or output.
func ScrubAllRegistries(rootLog *slog.Logger) error {
	paths := []string{GlobalRegistryPath()}
	home, _ := os.UserHomeDir()
	entries, err := os.ReadDir(filepath.Join(home, ".bridgectl", "registries"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("list context registries: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(home, ".bridgectl", "registries", entry.Name()))
	}
	sort.Strings(paths)
	for _, path := range paths {
		registry, err := NewRegistry(path, rootLog)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("load registry for scrubbing: %w", err)
		}
		if err := registry.ScrubCredentials(); err != nil {
			return err
		}
	}
	return nil
}

// MigrateGlobalRegistry copies the global registry to one context and removes
// the old file only after a successful, collision-free write. Legacy raw
// credentials must be scrubbed before migration.
func MigrateGlobalRegistry(contextName string, force bool, rootLog *slog.Logger) (int, error) {
	if _, err := ContextRegistryPath(contextName); err != nil {
		return 0, err
	}
	global := GlobalRegistryPath()
	if _, err := os.Stat(global); err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("%w: global registry does not exist", ErrLegacyRegistry)
		}
		return 0, fmt.Errorf("inspect global registry: %w", err)
	}
	source, err := NewRegistry(global, rootLog)
	if err != nil {
		return 0, err
	}
	if source.HasLegacyCredentials() {
		return 0, ErrLegacyCredentials
	}
	targetPath, err := ContextRegistryPath(contextName)
	if err != nil {
		return 0, err
	}
	target, err := NewRegistry(targetPath, rootLog)
	if err != nil {
		return 0, fmt.Errorf("load target registry: %w", err)
	}
	apis := source.List()
	for _, api := range apis {
		if _, exists := target.Get(api.Name); exists && !force {
			return 0, fmt.Errorf("%w: API %q", ErrRegistryConflict, api.Name)
		}
	}
	if err := target.withWrite(func() error {
		for _, api := range apis {
			target.APIs[api.Name] = api
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("write migrated registry: %w", err)
	}
	if err := os.Remove(global); err != nil {
		return 0, fmt.Errorf("remove global registry after migration: %w", err)
	}
	return len(apis), nil
}

// Get returns the API definition by name if found.
func (r *Registry) Get(name string) (API, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	api, ok := r.APIs[name]
	return api, ok
}
