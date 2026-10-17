package climcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const testChildTool = "bridgectl_test_block"

// syncBuffer lets a test read protocol output while the session is running.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// childErrorCode maps a runner error to the adapter code the catalogue uses.
func childErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrBusy):
		return CodeBusy
	case errors.Is(err, ErrOutputLimit):
		return CodeOutputLimit
	case errors.Is(err, ErrTimeout):
		return CodeTimeout
	case errors.Is(err, ErrCancelled):
		return CodeCancelled
	default:
		return CodeInternal
	}
}

// newLifecycleSession builds a session whose runner executes the controllable
// helper child. The stager is returned so a test can assert its cleanup.
func newLifecycleSession(t *testing.T, stdout io.Writer, stdin io.Reader, runner *Runner) (*Server, *Stager) {
	t.Helper()
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	session, err := NewServer(Options{
		Snapshot: testSnapshot(),
		Version:  "test-version",
		Stdin:    stdin,
		Stdout:   stdout,
		Stager:   stager,
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return session, stager
}

// registerTestChildTool registers a test-only tool that runs one helper child,
// so transport lifecycle behaviour can be observed without the real catalogue.
func registerTestChildTool(s *Server) {
	s.register(mcp.NewTool(testChildTool, mcp.WithDescription("Test-only child runner probe")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			outcome, err := s.runner.Run(ctx, CommandSpec{Argv: []string{"test", "probe"}})
			if err != nil {
				return NewFailure(testChildTool, s.snapshot.ContextName, childErrorCode(err),
					"the child did not complete", "Retry the operation.").callToolResult()
			}
			return NewSuccess(testChildTool, s.snapshot.ContextName, map[string]any{
				"exitCode":  outcome.ExitCode,
				"cancelled": outcome.Cancelled,
				"timedOut":  outcome.TimedOut,
			}).callToolResult()
		})
}

func writeLine(t *testing.T, writer io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(writer, line+"\n"); err != nil {
		t.Fatalf("write protocol line: %v", err)
	}
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func callLine(id int) string {
	return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"tools/call","params":{"name":"` +
		testChildTool + `","arguments":{}}}`
}

func startLifecycleSession(t *testing.T, runner *Runner) (*Stager, *syncBuffer, *io.PipeWriter, chan error) {
	t.Helper()
	stdinReader, stdinWriter := io.Pipe()
	out := &syncBuffer{}
	session, stager := newLifecycleSession(t, out, stdinReader, runner)
	registerTestChildTool(session)

	done := make(chan error, 1)
	go func() { done <- session.Serve(context.Background()) }()
	t.Cleanup(func() { _ = stdinWriter.Close() })
	return stager, out, stdinWriter, done
}

func TestServeCancelsActiveChildAndCleansUpWhenInputEnds(t *testing.T) {
	runner := helperRunner(30*time.Second, MaxChildOutputBytes, helperModeEnv+"=block")
	stager, _, stdinWriter, done := startLifecycleSession(t, runner)

	writeLine(t, stdinWriter, testInitRequest)
	writeLine(t, stdinWriter, callLine(7))
	waitFor(t, "an active child", runner.active.Load)

	start := time.Now()
	if err := stdinWriter.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned error on EOF: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve exceeded the five second shutdown budget")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("shutdown took %s, want at most 5s", elapsed)
	}
	if runner.active.Load() {
		t.Fatal("the child slot was not released")
	}
	if _, err := os.Stat(stager.Dir()); !os.IsNotExist(err) {
		t.Fatalf("the staging directory survived session shutdown: %v", err)
	}

	// A released slot must be reusable rather than permanently busy. A short
	// deadline proves the slot was acquired by timing out instead of returning
	// ErrBusy.
	runner.Timeout = 50 * time.Millisecond
	if _, err := runner.Run(context.Background(), CommandSpec{Argv: []string{"test", "probe"}}); !errors.Is(err, ErrTimeout) {
		t.Fatalf("reuse error = %v, want ErrTimeout from a free slot", err)
	}
}

func TestServeCancelsActiveChildOnClientCancellation(t *testing.T) {
	runner := helperRunner(30*time.Second, MaxChildOutputBytes, helperModeEnv+"=block")
	_, out, stdinWriter, done := startLifecycleSession(t, runner)

	writeLine(t, stdinWriter, testInitRequest)
	writeLine(t, stdinWriter, callLine(7))
	waitFor(t, "an active child", runner.active.Load)

	writeLine(t, stdinWriter, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7,"reason":"test"}}`)
	waitFor(t, "the cancelled child to stop", func() bool { return !runner.active.Load() })

	// The session must remain usable after one cancelled request.
	writeLine(t, stdinWriter, testPingRequest)
	waitFor(t, "a ping response after cancellation", func() bool {
		return strings.Contains(out.String(), `"id":2`)
	})

	if err := stdinWriter.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned error after cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop after cancellation")
	}
}

func TestServeReturnsBusyWhileAnotherChildIsActive(t *testing.T) {
	runner := helperRunner(30*time.Second, MaxChildOutputBytes, helperModeEnv+"=block")
	_, out, stdinWriter, done := startLifecycleSession(t, runner)

	writeLine(t, stdinWriter, testInitRequest)
	writeLine(t, stdinWriter, callLine(7))
	waitFor(t, "an active child", runner.active.Load)

	writeLine(t, stdinWriter, callLine(8))
	waitFor(t, "a busy result for the second call", func() bool {
		return strings.Contains(out.String(), `"code":"`+CodeBusy+`"`)
	})

	// The first call must still be the only child.
	if !runner.active.Load() {
		t.Fatal("the first child stopped when the second call was rejected")
	}

	if err := stdinWriter.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	<-done
}

func TestServeCancelsActiveChildWhenParentContextIsCancelled(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	defer func() { _ = stdinWriter.Close() }()
	out := &syncBuffer{}
	runner := helperRunner(30*time.Second, MaxChildOutputBytes, helperModeEnv+"=block")
	session, _ := newLifecycleSession(t, out, stdinReader, runner)
	registerTestChildTool(session)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Serve(ctx) }()

	writeLine(t, stdinWriter, testInitRequest)
	writeLine(t, stdinWriter, callLine(7))
	waitFor(t, "an active child", runner.active.Load)

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop after parent cancellation")
	}
	if runner.active.Load() {
		t.Fatal("the child slot was not released after parent cancellation")
	}
}
