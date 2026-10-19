package climcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/testutil"
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
		testutil.SetHome(t, t.TempDir())
		realCLI(t)
		return
	}
	if mode == "session" {
		hostHome := os.Getenv("HOME")
		session := realSession(t, newDeploymentFixture(t))
		fixtureHome, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		if fixtureHome == hostHome {
			t.Fatal("session did not isolate the runtime home")
		}
		required := map[string]bool{"HOME": false}
		if runtime.GOOS == "windows" {
			required["USERPROFILE"] = false
		}
		for _, entry := range session.runner.Env {
			key, value, _ := strings.Cut(entry, "=")
			if runtime.GOOS == "windows" {
				key = strings.ToUpper(key)
			}
			if _, needed := required[key]; needed {
				if value != fixtureHome {
					t.Fatalf("runtime child %s = %q, want fixture home %q", key, value, fixtureHome)
				}
				required[key] = true
			}
		}
		for key, present := range required {
			if !present {
				t.Fatalf("runtime child has no isolated %s", key)
			}
		}
		return
	}

	binDirectory := t.TempDir()
	builder := filepath.Join(binDirectory, testutil.ExecutableName("go"))
	// Compile with the real tool and preserved host environment before PATH
	// points at the fake builder. Never place a Go cache in a runtime home.
	// #nosec G204 -- fixed Go helper source and a test-owned executable path.
	build := exec.Command("go", "build", "-o", builder, "./testdata/fakego")
	build.Env = cliBuildEnvironment
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake Go helper: %v\n%s", err, output)
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
			command.Env = append(testutil.HomeEnvironment(os.Environ(), hostHome, ChildSnapshotEnv, "PATH", "GOPATH", "GOMODCACHE", buildHomeHelperEnv, "CLIMCP_BUILD_HOME_RECORD"),
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
