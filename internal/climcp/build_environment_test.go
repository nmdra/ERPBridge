package climcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const buildHomeHelperEnv = "CLIMCP_BUILD_HOME_HELPER"

// Run in a fresh test process so the once-only CLI build cannot be hidden by
// another test. A fake Go builder observes HOME without downloading modules or
// relying on the runner's permission to remove read-only module-cache files.
func TestRealCLIBuildUsesHostHome(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-session build environment check in short mode")
	}
	mode := os.Getenv(buildHomeHelperEnv)
	if mode == "direct" {
		t.Setenv("HOME", t.TempDir())
		realCLI(t)
		return
	}
	if mode == "session" {
		hostHome := os.Getenv("HOME")
		session := realSession(t, newDeploymentFixture(t))
		for _, entry := range session.runner.Env {
			if strings.HasPrefix(entry, "HOME=") {
				if strings.TrimPrefix(entry, "HOME=") == hostHome {
					t.Fatal("runtime child inherited the build home instead of the fixture home")
				}
				return
			}
		}
		t.Fatal("runtime child has no isolated HOME")
	}

	binDirectory := t.TempDir()
	builder := filepath.Join(binDirectory, "go")
	const script = "#!/bin/sh\nprintf '%s' \"$HOME\" > \"$CLIMCP_BUILD_HOME_RECORD\"\n"
	// #nosec G306 -- the private fake builder must be executable by this test.
	if err := os.WriteFile(builder, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	hostHome := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, buildMode := range []string{"session", "direct"} {
		t.Run(buildMode, func(t *testing.T) {
			record := filepath.Join(t.TempDir(), "build-home")
			// #nosec G204 -- execute this test binary with a fixed test selector.
			command := exec.Command(executable, "-test.run=^TestRealCLIBuildUsesHostHome$")
			command.Env = append(acceptanceEnvironment(t, hostHome),
				buildHomeHelperEnv+"="+buildMode,
				"CLIMCP_BUILD_HOME_RECORD="+record,
				"PATH="+binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GOPATH=", "GOMODCACHE=",
			)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("build environment helper failed: %v\n%s", err, output)
			}
			observed, err := os.ReadFile(record) // #nosec G304 -- record is inside t.TempDir()
			if err != nil {
				t.Fatal(err)
			}
			if string(observed) != hostHome {
				t.Fatalf("build HOME = %q, want host HOME %q, not a fixture directory", observed, hostHome)
			}
		})
	}
}
