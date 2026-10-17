package climcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultCallTimeout bounds one child invocation.
	DefaultCallTimeout = 30 * time.Second
	// MaxChildOutputBytes bounds each captured child stream.
	MaxChildOutputBytes = 1 << 20
	// MaxManifestBytes bounds one inline manifest.
	MaxManifestBytes = 256 << 10
	// MaxCallMessageBytes bounds one incoming stdio message.
	MaxCallMessageBytes = 1 << 20
	// stagingDirPerm is the private directory mode for staged manifests.
	stagingDirPerm = 0o700
	// stagingFilePerm is the private file mode for staged manifests.
	stagingFilePerm = 0o600
)

var (
	// ErrBusy reports that another child process already holds the slot.
	ErrBusy = errors.New("climcp: another child process is already running")
	// ErrOutputLimit reports that a child stream exceeded the capture limit.
	ErrOutputLimit = errors.New("climcp: child output exceeded the capture limit")
	// ErrTimeout reports that the child exceeded the call deadline.
	ErrTimeout = errors.New("climcp: child execution timed out")
	// ErrCancelled reports that the caller cancelled the child.
	ErrCancelled = errors.New("climcp: child execution was cancelled")
)

// CommandSpec is one fixed child invocation. Argv is built by the adapter from
// a validated descriptor, never from raw caller input.
type CommandSpec struct {
	// Argv is the complete CLI argument vector, excluding the executable.
	Argv []string
}

// Outcome is the bounded result of one child invocation. Stdout and Stderr are
// untrusted data and are never returned to an MCP client without projection.
type Outcome struct {
	Stdout        []byte
	Stderr        []byte
	ExitCode      int
	Duration      time.Duration
	TimedOut      bool
	Cancelled     bool
	OutputLimited bool
}

// Runner executes one child at a time and captures bounded output.
type Runner struct {
	// Executable is the absolute path of the binary to run.
	Executable string
	// PrefixArgs are fixed leading arguments (test seams only).
	PrefixArgs []string
	// Env is the complete child environment.
	Env []string
	// Timeout bounds one invocation. Zero selects DefaultCallTimeout.
	Timeout time.Duration
	// MaxOutputBytes bounds each captured stream. Zero selects
	// MaxChildOutputBytes.
	MaxOutputBytes int64

	active atomic.Bool
}

// Run executes spec and returns its bounded outcome. A non-zero child exit code
// is data, not an error; busy, limit, timeout, and cancellation conditions are
// reported as errors. ctx must not be nil.
func (r *Runner) Run(ctx context.Context, spec CommandSpec) (Outcome, error) {
	if !r.active.CompareAndSwap(false, true) {
		return Outcome{}, ErrBusy
	}
	defer r.active.Store(false)

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	maxOutput := r.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = MaxChildOutputBytes
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, 0, len(r.PrefixArgs)+len(spec.Argv))
	args = append(args, r.PrefixArgs...)
	args = append(args, spec.Argv...)

	// #nosec G204 -- executable is the pinned bridgectl binary; argv comes only
	// from a fixed descriptor, never from caller-supplied text.
	cmd := exec.CommandContext(runContext, r.Executable, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Env = r.Env
	// A child never receives the MCP stdin stream.
	cmd.Stdin = strings.NewReader("")

	var overflow atomic.Bool
	var overflowOnce sync.Once
	onOverflow := func() {
		overflowOnce.Do(func() {
			overflow.Store(true)
			cancel()
		})
	}
	stdout := newLimitedBuffer(maxOutput, onOverflow)
	stderr := newLimitedBuffer(maxOutput, onOverflow)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	runErr := cmd.Run()
	outcome := Outcome{
		Stdout:        stdout.Bytes(),
		Stderr:        stderr.Bytes(),
		Duration:      time.Since(start),
		OutputLimited: overflow.Load(),
		ExitCode:      -1,
	}
	if cmd.ProcessState != nil {
		outcome.ExitCode = cmd.ProcessState.ExitCode()
	}

	if outcome.OutputLimited {
		return outcome, ErrOutputLimit
	}
	if runErr == nil {
		return outcome, nil
	}
	switch {
	case errors.Is(runContext.Err(), context.DeadlineExceeded):
		outcome.TimedOut = true
		return outcome, ErrTimeout
	case errors.Is(ctx.Err(), context.Canceled):
		outcome.Cancelled = true
		return outcome, ErrCancelled
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		// A non-zero exit code is command data for the caller to classify.
		return outcome, nil
	}
	return outcome, runErr
}

// limitedBuffer records at most max bytes and reports the first overflow.
type limitedBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	limit    int64
	overflow func()
}

func newLimitedBuffer(limit int64, overflow func()) *limitedBuffer {
	return &limitedBuffer{limit: limit, overflow: overflow}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	// The overflow callback runs outside the lock so a cancelled child cannot
	// deadlock against a concurrent Bytes read.
	overflowed := func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		remaining := b.limit - int64(b.buf.Len())
		switch {
		case remaining <= 0:
			return true
		case int64(len(p)) > remaining:
			_, _ = b.buf.Write(p[:remaining])
			return true
		default:
			_, _ = b.buf.Write(p)
			return false
		}
	}()
	if overflowed {
		b.overflow()
	}
	// The full length is always reported so the child is never blocked on a
	// partial write while the overflow handler stops it.
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

// Stager owns the private directory used for staged manifests.
type Stager struct {
	mu  sync.Mutex
	dir string
}

// NewStager creates a private staging directory.
func NewStager() (*Stager, error) {
	dir, err := os.MkdirTemp("", "bridgectl-mcp-")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	if err := os.Chmod(dir, stagingDirPerm); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("secure staging directory: %w", err)
	}
	return &Stager{dir: dir}, nil
}

// Dir returns the staging directory path.
func (s *Stager) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir
}

// Stage writes one private manifest file and returns its path. It refuses
// oversized input before creating the file.
func (s *Stager) Stage(name string, data []byte) (string, error) {
	if len(data) > MaxManifestBytes {
		return "", fmt.Errorf("climcp: manifest is %d bytes, limit is %d", len(data), MaxManifestBytes)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return "", errors.New("climcp: staging directory is not available")
	}
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", fmt.Errorf("climcp: invalid staging name %q", name)
	}
	path := filepath.Join(s.dir, name)
	if err := os.WriteFile(path, data, stagingFilePerm); err != nil {
		return "", fmt.Errorf("stage %s: %w", name, err)
	}
	return path, nil
}

// Cleanup removes the staging directory and its contents.
func (s *Stager) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return nil
	}
	err := os.RemoveAll(s.dir)
	s.dir = ""
	return err
}
