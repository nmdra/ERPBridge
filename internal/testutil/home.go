// Package testutil provides isolated homes for tests and runtime child processes.
package testutil

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

const windowsOS = "windows"

// SetHome isolates native home resolution and verifies it before any default
// configuration path is used. It does not change the host build environment.
func SetHome(t testing.TB, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS == windowsOS {
		t.Setenv("USERPROFILE", home)
	}
	resolved, err := os.UserHomeDir()
	if err != nil || resolved != home {
		t.Fatalf("fixture home resolved to %q, want %q: %v", resolved, home, err)
	}
}

// HomeEnvironment preserves host process essentials but removes inherited bridge
// credentials and caller-specified process controls. Windows environment keys
// are case-insensitive. Unix keys retain their normal case-sensitive meaning.
func HomeEnvironment(base []string, home string, blocked ...string) []string {
	normalize := func(key string) string {
		if runtime.GOOS == windowsOS {
			return strings.ToUpper(key)
		}
		return key
	}
	excluded := map[string]bool{"HOME": true, "LOG_TO_STDERR": true}
	if runtime.GOOS == windowsOS {
		excluded["USERPROFILE"] = true
	}
	for _, key := range blocked {
		excluded[normalize(key)] = true
	}
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		key = normalize(key)
		if excluded[key] || strings.HasPrefix(key, "BRIDGE_") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "HOME="+home)
	if runtime.GOOS == windowsOS {
		env = append(env, "USERPROFILE="+home)
	}
	return env
}

// ExecutableName adds the native executable suffix for a fixture binary.
func ExecutableName(name string) string {
	if runtime.GOOS == windowsOS {
		return name + ".exe"
	}
	return name
}
