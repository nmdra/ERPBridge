package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// ErrConfigChanged reports that the configuration file changed while it was
// being updated, so the update was refused rather than overwriting the change.
var ErrConfigChanged = errors.New("configuration file changed while it was being updated")

// beforeReplace is a test seam. It runs after the replacement file is written
// and before the original file is re-checked.
var beforeReplace func()

// SetCurrentContext updates only the saved default context in the raw
// configuration file.
//
// The raw file is edited rather than a loaded copy, so environment placeholders,
// stored values, and unknown fields survive. The replacement is written with
// mode 0600 to a private file in the same directory and is installed with a
// rename. An external change between the read and the rename is refused.
func SetCurrentContext(name string) error {
	if err := ValidateContextName(name); err != nil {
		return err
	}
	path := filepath.Clean(configPath())

	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to replace the configuration symlink %s", path)
		}
	case os.IsNotExist(err):
		return createDefaultConfigFile(path, name)
	default:
		return fmt.Errorf("inspect configuration: %w", err)
	}

	original, err := os.ReadFile(path) // #nosec G304 -- resolved within the user home
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	fingerprint := fingerprintBytes(original)

	// Parse without environment expansion so placeholders stay placeholders.
	document := make(map[string]any)
	if err := yaml.Unmarshal(original, &document); err != nil {
		return fmt.Errorf("parse configuration: %w", err)
	}
	if document == nil {
		document = make(map[string]any)
	}
	if !savedContextExists(document, name) {
		return fmt.Errorf("context %q is not configured", name)
	}
	document["current-context"] = name

	encoded, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	return replaceConfigFile(path, encoded, fingerprint, false)
}

// savedContextExists reports whether the raw document declares the named
// context.
func savedContextExists(document map[string]any, name string) bool {
	contexts, ok := document["contexts"].(map[string]any)
	if !ok {
		return false
	}
	_, present := contexts[name]
	return present
}

// replaceConfigFile installs encoded as the configuration file. When
// mustNotExist is set, the file must still be absent; otherwise the current
// file must still match fingerprint.
func replaceConfigFile(path string, encoded []byte, fingerprint string, mustNotExist bool) error {
	directory := filepath.Dir(path)
	existing, err := os.Stat(path)
	mode := os.FileMode(0o600)
	if err == nil {
		mode = existing.Mode().Perm()
	}

	temporary, err := os.CreateTemp(directory, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create replacement configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("secure replacement configuration: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		cleanup()
		return fmt.Errorf("write replacement configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync replacement configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close replacement configuration: %w", err)
	}

	if beforeReplace != nil {
		beforeReplace()
	}

	current, err := os.ReadFile(path) // #nosec G304 -- resolved within the user home
	switch {
	case mustNotExist:
		if err == nil {
			_ = os.Remove(temporaryPath)
			return ErrConfigChanged
		}
		if !os.IsNotExist(err) {
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("re-check configuration: %w", err)
		}
	case err != nil:
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("re-read configuration: %w", err)
	case fingerprintBytes(current) != fingerprint:
		_ = os.Remove(temporaryPath)
		return ErrConfigChanged
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("install configuration: %w", err)
	}
	return nil
}

// createDefaultConfigFile writes the built-in defaults plus the selected
// context. Environment overrides are deliberately not persisted.
func createDefaultConfigFile(path, name string) error {
	if !savedContextExists(map[string]any{}, name) && name != DefaultContextName {
		return fmt.Errorf("context %q is not configured", name)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	document := struct {
		CurrentContext string             `yaml:"current-context"`
		Contexts       map[string]Context `yaml:"contexts"`
	}{
		CurrentContext: name,
		Contexts:       defaultConfig().Contexts,
	}
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	return replaceConfigFile(path, encoded, "", true)
}

// fingerprintBytes returns a stable fingerprint of raw configuration content.
func fingerprintBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
