// Package climcp implements the bridgectl management MCP adapter. It exposes a
// fixed catalogue of existing CLI operations over MCP stdio by running the
// bridgectl binary as an isolated child process, so no command handler shares
// the MCP process, its stdin, or its stdout.
package climcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ServerName is the MCP server identity reported to clients. It is distinct
// from the runtime "ERPBridge" server, which serves published ERP tools.
const ServerName = "bridgectl"

// WriteGroup names one startup permission group. No wildcard exists.
type WriteGroup string

const (
	// GroupRegistry allows API registration and credential-reference changes.
	GroupRegistry WriteGroup = "registry"
	// GroupResources allows applying tool, plugin, and binding manifests.
	GroupResources WriteGroup = "resources"
	// GroupDestructive allows deletion, revocation, and cache flush.
	GroupDestructive WriteGroup = "destructive"
	// GroupProbe allows server-side registered API tests.
	GroupProbe WriteGroup = "probe"
	// GroupContext allows changing the saved default context.
	GroupContext WriteGroup = "context"
)

// Options configures one management session.
type Options struct {
	// Snapshot is the immutable launch-time context selection.
	Snapshot Snapshot
	// Version is the reported server version.
	Version string
	// Groups are the operator-approved write groups. An empty set permits
	// inspection, local validation, and generation only.
	Groups []WriteGroup
	// Stdin and Stdout carry the MCP protocol stream.
	Stdin  io.Reader
	Stdout io.Writer
	// Runner and Stager are injectable seams for tests.
	Runner *Runner
	Stager *Stager
}

// Server is one management MCP session.
type Server struct {
	snapshot  Snapshot
	groups    map[WriteGroup]bool
	runner    *Runner
	stager    *Stager
	stdin     io.Reader
	stdout    io.Writer
	mcpServer *server.MCPServer
}

// NewServer builds a management MCP session.
func NewServer(options Options) (*Server, error) {
	if options.Snapshot.ContextName == "" {
		return nil, errors.New("climcp: a selected context is required")
	}
	groups, err := parseWriteGroups(options.Groups)
	if err != nil {
		return nil, err
	}
	stdin := options.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	runner := options.Runner
	if runner == nil {
		executable, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable path: %w", err)
		}
		runner = &Runner{
			Executable: executable,
			Env:        options.Snapshot.ChildEnvironment(os.Environ()),
		}
	}
	stager := options.Stager
	if stager == nil {
		stager, err = NewStager()
		if err != nil {
			return nil, err
		}
	}
	version := options.Version
	if version == "" {
		version = "dev"
	}

	mcpServer := server.NewMCPServer(ServerName, version,
		server.WithToolCapabilities(false),
		server.WithInputSchemaValidation(),
		server.WithStrictInputSchemaDefault(),
	)

	session := &Server{
		snapshot:  options.Snapshot,
		groups:    groups,
		runner:    runner,
		stager:    stager,
		stdin:     stdin,
		stdout:    stdout,
		mcpServer: mcpServer,
	}
	registerCatalogue(session)
	return session, nil
}

// SessionSnapshot returns the immutable session selection.
func (s *Server) SessionSnapshot() Snapshot { return s.snapshot }

// Allows reports whether a write group was approved at startup.
func (s *Server) Allows(group WriteGroup) bool { return s.groups[group] }

// ToolNames returns the advertised tool names in deterministic order.
func (s *Server) ToolNames() []string {
	tools := s.mcpServer.ListTools()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Serve runs the stdio transport until the input ends or ctx is cancelled.
// Serve runs the stdio transport until the input ends, ctx is cancelled, or the
// stream fails. Session resources are released before it returns. ctx must not
// be nil.
func (s *Server) Serve(ctx context.Context) error {
	sessionContext, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = s.Close() }()

	stdio := server.NewStdioServer(s.mcpServer)
	input := newCancelOnEndReader(newBoundedLineReader(s.stdin, MaxCallMessageBytes), cancel)
	listenErr := stdio.Listen(sessionContext, input, s.stdout)

	// Ending the input cancels the session so an active child is stopped, which
	// also surfaces as context.Canceled. Report the real cause instead.
	ended, cause := input.inputEnd()
	switch {
	case ended && errors.Is(cause, io.EOF) && errors.Is(listenErr, context.Canceled) && ctx.Err() == nil:
		return nil
	case ended && !errors.Is(cause, io.EOF) && errors.Is(listenErr, context.Canceled):
		return cause
	default:
		return listenErr
	}
}

// Close releases session resources. An active child is stopped by cancelling
// the session context, not by Close.
func (s *Server) Close() error {
	return s.stager.Cleanup()
}

// cancelOnEndReader cancels the session when its input stream ends, and records
// why it ended. The SDK waits for an in-flight handler before Listen returns,
// so without the cancellation an EOF would leave an active child running until
// its own deadline.
type cancelOnEndReader struct {
	source io.Reader
	cancel context.CancelFunc

	mu      sync.Mutex
	ended   bool
	lastErr error
}

func newCancelOnEndReader(source io.Reader, cancel context.CancelFunc) *cancelOnEndReader {
	return &cancelOnEndReader{source: source, cancel: cancel}
}

func (r *cancelOnEndReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if err != nil {
		r.mu.Lock()
		if !r.ended {
			r.ended = true
			r.lastErr = err
		}
		r.mu.Unlock()
		r.cancel()
	}
	return n, err
}

// inputEnd reports whether the input stream ended and why.
func (r *cancelOnEndReader) inputEnd() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ended, r.lastErr
}

// register adds one fixed tool. It is unexported so the catalogue is the only
// source of advertised tools.
func (s *Server) register(tool mcp.Tool, handler server.ToolHandlerFunc) {
	s.mcpServer.AddTool(tool, handler)
}

// parseWriteGroups validates operator-supplied group names. Unknown names and
// wildcards are rejected rather than ignored.
func parseWriteGroups(groups []WriteGroup) (map[WriteGroup]bool, error) {
	accepted := make(map[WriteGroup]bool)
	for _, group := range WriteGroups() {
		accepted[group] = true
	}
	approved := make(map[WriteGroup]bool, len(groups))
	for _, group := range groups {
		if !accepted[group] {
			return nil, fmt.Errorf("climcp: unknown write group %q", group)
		}
		approved[group] = true
	}
	return approved, nil
}

// WriteGroups returns the accepted startup permission groups in deterministic
// order. Validation and operator help both use this list, so they cannot drift.
func WriteGroups() []WriteGroup {
	return []WriteGroup{GroupContext, GroupDestructive, GroupProbe, GroupRegistry, GroupResources}
}

// ErrMessageTooLarge reports that one incoming stdio message exceeded the
// adapter limit. The session ends rather than buffering the message.
var ErrMessageTooLarge = errors.New("climcp: incoming message exceeded the size limit")

// boundedLineReader stops reading once a single newline-delimited message
// exceeds max bytes, so an oversized request cannot grow memory without bound.
type boundedLineReader struct {
	source  io.Reader
	limit   int
	pending int
}

func newBoundedLineReader(source io.Reader, limit int) *boundedLineReader {
	return &boundedLineReader{source: source, limit: limit}
}

func (r *boundedLineReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	for _, b := range p[:n] {
		if b == '\n' {
			r.pending = 0
			continue
		}
		r.pending++
		if r.pending > r.limit {
			return n, ErrMessageTooLarge
		}
	}
	return n, err
}
