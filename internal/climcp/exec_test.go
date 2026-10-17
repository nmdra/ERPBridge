package climcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	helperModeEnv   = "CLIMCP_HELPER_MODE"
	helperExitEnv   = "CLIMCP_HELPER_EXIT"
	helperStdoutEnv = "CLIMCP_HELPER_STDOUT"
)

// TestHelperProcess is the controllable child used by the Runner tests. It
// only acts when the mode variable is present, so an ordinary test run is
// unaffected.
func TestHelperProcess(_ *testing.T) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		return
	}
	switch mode {
	case "echo":
		_, _ = fmt.Fprint(os.Stdout, os.Getenv(helperStdoutEnv))
		_, _ = fmt.Fprint(os.Stderr, "helper-stderr")
		os.Exit(helperExitCode())
	case "flood":
		chunk := strings.Repeat("x", 4096)
		for {
			if _, err := os.Stdout.WriteString(chunk); err != nil {
				os.Exit(0)
			}
		}
	case "block":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "stdin":
		buf := make([]byte, 16)
		n, err := os.Stdin.Read(buf)
		_, _ = fmt.Fprintf(os.Stdout, "n=%d eof=%t", n, errors.Is(err, io.EOF))
		os.Exit(0)
	}
	os.Exit(0)
}

func helperExitCode() int {
	raw := os.Getenv(helperExitEnv)
	if raw == "" {
		return 0
	}
	code := 0
	if _, err := fmt.Sscanf(raw, "%d", &code); err != nil {
		return 0
	}
	return code
}

func helperRunner(timeout time.Duration, maxOutput int64, env ...string) *Runner {
	return &Runner{
		Executable:     os.Args[0],
		PrefixArgs:     []string{"-test.run=^TestHelperProcess$", "--"},
		Env:            append(os.Environ(), env...),
		Timeout:        timeout,
		MaxOutputBytes: maxOutput,
	}
}

func TestRunnerCapturesOutputAndReportsExitCode(t *testing.T) {
	runner := helperRunner(5*time.Second, MaxChildOutputBytes,
		helperModeEnv+"=echo", helperStdoutEnv+"=child-stdout", helperExitEnv+"=3")

	outcome, err := runner.Run(context.Background(), CommandSpec{Argv: []string{"context", "list"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := string(outcome.Stdout); got != "child-stdout" {
		t.Fatalf("stdout = %q, want %q", got, "child-stdout")
	}
	if got := string(outcome.Stderr); got != "helper-stderr" {
		t.Fatalf("stderr = %q, want %q", got, "helper-stderr")
	}
	if outcome.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", outcome.ExitCode)
	}
	if outcome.TimedOut || outcome.Cancelled || outcome.OutputLimited {
		t.Fatalf("outcome flags = %+v, want all false", outcome)
	}
}

func TestRunnerRejectsSecondChildWhileBusy(t *testing.T) {
	runner := helperRunner(5*time.Second, MaxChildOutputBytes, helperModeEnv+"=block")

	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = runner.Run(context.Background(), CommandSpec{Argv: []string{"cache", "stats"}})
	}()
	<-started
	time.Sleep(150 * time.Millisecond)

	_, err := runner.Run(context.Background(), CommandSpec{Argv: []string{"cache", "stats"}})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second Run error = %v, want ErrBusy", err)
	}
}

func TestRunnerStopsChildOnTimeout(t *testing.T) {
	runner := helperRunner(200*time.Millisecond, MaxChildOutputBytes, helperModeEnv+"=block")

	start := time.Now()
	outcome, err := runner.Run(context.Background(), CommandSpec{Argv: []string{"api", "list"}})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Run error = %v, want ErrTimeout", err)
	}
	if !outcome.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took %s, want a bounded stop", elapsed)
	}
}

func TestRunnerStopsChildWhenOutputExceedsLimit(t *testing.T) {
	runner := helperRunner(10*time.Second, 4096, helperModeEnv+"=flood")

	outcome, err := runner.Run(context.Background(), CommandSpec{Argv: []string{"log", "stats"}})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("Run error = %v, want ErrOutputLimit", err)
	}
	if !outcome.OutputLimited {
		t.Fatalf("OutputLimited = false, want true")
	}
	if int64(len(outcome.Stdout)) > 4096 {
		t.Fatalf("captured %d stdout bytes, want at most 4096", len(outcome.Stdout))
	}
}

func TestRunnerStopsChildWhenParentContextIsCancelled(t *testing.T) {
	runner := helperRunner(30*time.Second, MaxChildOutputBytes, helperModeEnv+"=block")
	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := runner.Run(ctx, CommandSpec{Argv: []string{"api", "list"}})
		result <- err
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("Run error = %v, want ErrCancelled", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("cancellation did not stop the child")
	}
}

func TestRunnerGivesChildNoStdin(t *testing.T) {
	runner := helperRunner(5*time.Second, MaxChildOutputBytes, helperModeEnv+"=stdin")

	outcome, err := runner.Run(context.Background(), CommandSpec{Argv: []string{"context", "list"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := string(outcome.Stdout); got != "n=0 eof=true" {
		t.Fatalf("child stdin observation = %q, want %q", got, "n=0 eof=true")
	}
}

func TestStagerWritesPrivateFileAndCleansUp(t *testing.T) {
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	path, err := stager.Stage("manifest.json", []byte(`{"kind":"tool"}`))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat staged file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("staged file mode = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(stager.Dir())
	if err != nil {
		t.Fatalf("stat staging dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("staging dir mode = %o, want 700", perm)
	}

	if _, err := stager.Stage("../escape.json", []byte("{}")); err == nil {
		t.Fatal("Stage accepted a traversal name")
	}

	if _, err := stager.Stage("large.json", make([]byte, MaxManifestBytes+1)); err == nil {
		t.Fatal("Stage accepted a manifest above the size limit")
	}
	if _, err := stager.Stage("exact.json", make([]byte, MaxManifestBytes)); err != nil {
		t.Fatalf("Stage rejected a manifest at the size limit: %v", err)
	}

	if err := stager.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("staged file survived cleanup: %v", err)
	}
}
