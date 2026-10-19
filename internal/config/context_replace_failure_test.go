package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Removing the staged file at the existing pre-replacement seam makes rename
// fail on every platform, without relying on Unix directory permission bits.
func TestSetCurrentContextReplacementFailurePreservesOriginal(t *testing.T) {
	home := withConfigHome(t)
	path := writeRawConfig(t, home)
	original, err := os.ReadFile(path) // #nosec G304 -- test-owned configuration
	if err != nil {
		t.Fatal(err)
	}
	beforeReplace = func() {
		entries, readErr := os.ReadDir(filepath.Dir(path))
		if readErr != nil {
			t.Fatal(readErr)
		}
		removed := false
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".config-") {
				if removeErr := os.Remove(filepath.Join(filepath.Dir(path), entry.Name())); removeErr != nil {
					t.Fatal(removeErr)
				}
				removed = true
			}
		}
		if !removed {
			t.Fatal("no staged replacement reached the seam")
		}
	}
	t.Cleanup(func() { beforeReplace = nil })
	if err := SetCurrentContext("staging"); err == nil || !strings.Contains(err.Error(), "install configuration") {
		t.Fatalf("expected replacement error, got %v", err)
	}
	current, err := os.ReadFile(path) // #nosec G304 -- test-owned configuration
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, current) {
		t.Fatal("failed replacement changed the original configuration")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".config-") {
			t.Fatal("failed replacement left a staged file")
		}
	}
}
