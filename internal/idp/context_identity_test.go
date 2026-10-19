package idp

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func registryIdentityHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestContextIdentityRegistryPaths(t *testing.T) {
	home := registryIdentityHome(t)
	for _, name := range []string{"CON", "con", "NUL.test", "aux", "PRN.log", "COM1", "com9.data", "LPT9", "lpt1.backup", "console", "com10", "lpt10", "CONSOLE.test", "prod", "Prod", "local", "my-context_2"} {
		t.Run(name, func(t *testing.T) {
			path, err := ContextRegistryPath(name)
			reserved := name == "CON" || name == "con" || name == "NUL.test" || name == "aux" || name == "PRN.log" || name == "COM1" || name == "com9.data" || name == "LPT9" || name == "lpt1.backup"
			if runtime.GOOS == "windows" && reserved {
				if err == nil || path != "" {
					t.Fatalf("unsafe registry path accepted: %q, %v", path, err)
				}
				if _, err := NewRegistryForContext(name, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
					t.Fatal("registry open bypassed path refusal")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				want := filepath.Join(home, ".bridgectl", "registries", name+".json")
				if path != want {
					t.Fatalf("path = %q, want %q", path, want)
				}
			}
		})
	}
	// Superscript device digits are already outside the existing ASCII grammar.
	for _, name := range []string{"COM¹", "LPT²", "../prod", ""} {
		if _, err := ContextRegistryPath(name); err == nil {
			t.Fatalf("invalid syntax accepted: %q", name)
		}
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("path validation created files: %v, %v", entries, err)
	}
}

func TestContextIdentityRegistryIsolation(t *testing.T) {
	home := registryIdentityHome(t)
	names := []string{"prod", "stage"}
	if runtime.GOOS != "windows" {
		names = append(names, "Prod", "CON", "NUL.test")
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	originals := make(map[string][]byte)
	for _, name := range names {
		reg, err := NewRegistryForContext(name, log)
		if err != nil {
			t.Fatal(err)
		}
		// Register only persists the fixture. It makes no remote request.
		if err := reg.Register(&API{Name: name, URL: "https://fixture.invalid/" + name, Method: "GET"}); err != nil {
			t.Fatal(err)
		}
		path, err := ContextRegistryPath(name)
		if err != nil {
			t.Fatal(err)
		}
		originals[name], err = os.ReadFile(path) // #nosec G304 -- path is confined to the test fixture home.
		if err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"CON", "NUL.test"} {
			if _, err := NewRegistryForContext(name, log); err == nil {
				t.Fatalf("unsafe context accepted: %q", name)
			}
		}
	}
	for _, name := range names {
		reg, err := NewRegistryForContext(name, log)
		if err != nil {
			t.Fatal(err)
		}
		apis := reg.List()
		if len(apis) != 1 || apis[0].Name != name {
			t.Fatalf("context %q aliases data: %+v", name, apis)
		}
		path, err := ContextRegistryPath(name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path) // #nosec G304 -- path is confined to the test fixture home.
		if err != nil || string(got) != string(originals[name]) {
			t.Fatalf("registry %q changed: %v", name, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, ".bridgectl", "registries"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(names) {
		t.Fatalf("unexpected registry/temp/lock files: %v", entries)
	}
}
