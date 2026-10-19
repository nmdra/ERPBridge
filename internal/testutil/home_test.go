package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHomeIsolation(t *testing.T) {
	if os.Getenv("TESTUTIL_HOME_CHILD") == "1" {
		home, err := os.UserHomeDir()
		if err != nil || home != os.Getenv("TESTUTIL_EXPECT_HOME") {
			t.Fatalf("child home = %q, %v", home, err)
		}
		if os.Getenv("BRIDGE_TEST_CREDENTIAL") != "" {
			t.Fatal("child inherited synthetic credential")
		}
		return
	}
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	SetHome(t, home)
	got, err := os.UserHomeDir()
	if err != nil || got != home {
		t.Fatalf("home = %q, %v", got, err)
	}
	env := HomeEnvironment(append(os.Environ(), "BRIDGE_TEST_CREDENTIAL=synthetic"), home)
	env = append(env, "TESTUTIL_HOME_CHILD=1", "TESTUTIL_EXPECT_HOME="+home)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- reexecute this test binary with a fixed test selector.
	cmd := exec.Command(exe, "-test.run=^TestHomeIsolation$")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	data, err := os.ReadFile(sentinel) // #nosec G304 -- sentinel belongs to t.TempDir().
	if err != nil || string(data) != "untouched" {
		t.Fatalf("outside sentinel changed: %v", err)
	}
}

func TestHomeIsolationEnvironment(t *testing.T) {
	base := []string{"HOME=old", "USERPROFILE=old", "Home=variant", "Bridge_TEST_CREDENTIAL=variant", "BRIDGE_TEST_CREDENTIAL=synthetic", "LOG_TO_STDERR=1", "Snapshot=secret", "PATH=path", "SystemRoot=root", "TEMP=temp", "TMP=tmp"}
	env := HomeEnvironment(base, "fixture", "Snapshot")
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, entry := range []string{"PATH=path", "SystemRoot=root", "TEMP=temp", "TMP=tmp", "HOME=fixture"} {
		if !strings.Contains(joined, "\n"+entry+"\n") {
			t.Errorf("missing %s", entry)
		}
	}
	for _, entry := range []string{"HOME=old", "BRIDGE_TEST_CREDENTIAL=synthetic", "LOG_TO_STDERR=1", "Snapshot=secret"} {
		if strings.Contains(joined, "\n"+entry+"\n") {
			t.Errorf("retained %s", entry)
		}
	}
	for _, entry := range []string{"Home=variant", "Bridge_TEST_CREDENTIAL=variant", "USERPROFILE=old"} {
		retained := strings.Contains(joined, "\n"+entry+"\n")
		if retained != (runtime.GOOS != "windows") {
			t.Errorf("wrong platform case/home semantics for %s", entry)
		}
	}
	if runtime.GOOS == "windows" && !strings.Contains(joined, "\nUSERPROFILE=fixture\n") {
		t.Fatal("missing Windows fixture profile")
	}
}
