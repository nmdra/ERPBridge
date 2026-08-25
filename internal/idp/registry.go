// Package idp provides API registration and OpenAPI-to-MCP tool schema generation.
package idp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

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

// Registry manages storage and retrieval of registered API definitions.
type Registry struct {
	path              string
	log               *slog.Logger
	mu                sync.RWMutex
	APIs              map[string]API `json:"apis"`
	legacyCredentials bool
	rename            func(string, string) error
}

// NewRegistry initializes an API registry from the specified file path or default user home path.
func NewRegistry(path string, rootLog *slog.Logger) (*Registry, error) {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".bridgectl", "registry.json")
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

// Register adds or updates an API definition in the registry.
func (r *Registry) Register(api *API) error {
	if api == nil {
		return errors.New("API is required")
	}
	if api.CredentialRef != "" {
		if err := credentials.ValidateReference(api.CredentialRef); err != nil {
			return err
		}
	}
	return r.withWrite(func() error {
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

// Get returns the API definition by name if found.
func (r *Registry) Get(name string) (API, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	api, ok := r.APIs[name]
	return api, ok
}
