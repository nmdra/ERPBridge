package climcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/testutil"
)

// This file drives the real bridgectl binary over stdio with a strict JSON-RPC
// reader, so protocol framing, discovery, gating, and shutdown are checked
// against the real transport rather than an in-process session.

const stdioInitRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"acceptance","version":"1"}}}`

// stdioSession is one real management MCP process under test.
type stdioSession struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	reader  *bufio.Reader
	stderr  *syncBuffer
	mu      sync.Mutex
}

// acceptanceEnvironment builds a child environment with a temporary home and no
// inherited bridge credentials.
func acceptanceEnvironment(t *testing.T, home string) []string {
	t.Helper()
	return testutil.HomeEnvironment(os.Environ(), home, ChildSnapshotEnv)
}

// startSession launches the real management server against one fixture.
func startSession(t *testing.T, fixture *deploymentFixture, groups ...string) *stdioSession {
	t.Helper()
	home := t.TempDir()
	writeContextConfig(t, home, fixture.server.URL)

	args := []string{"mcp", "--stdio", "--context", "test"}
	for _, group := range groups {
		args = append(args, "--allow-write", group)
	}
	// #nosec G204 G702 -- the executable is the binary this test compiled and
	// the argument vector is fixed by the test.
	command := exec.Command(realCLI(t), args...)
	command.Env = acceptanceEnvironment(t, home)

	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr := &syncBuffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start management server: %v", err)
	}

	session := &stdioSession{command: command, stdin: stdin, reader: bufio.NewReader(stdout), stderr: stderr}
	t.Cleanup(session.terminate)
	return session
}

func (s *stdioSession) send(t *testing.T, payload string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := io.WriteString(s.stdin, payload+"\n"); err != nil {
		t.Fatalf("write protocol message: %v (stderr: %s)", err, s.stderr.String())
	}
}

// readLine reads one protocol line, failing the test if the process closes
// stdout or the line is not valid JSON-RPC.
func (s *stdioSession) readLine(t *testing.T) string {
	t.Helper()
	type outcome struct {
		line string
		err  error
	}
	results := make(chan outcome, 1)
	go func() {
		line, err := s.reader.ReadString('\n')
		results <- outcome{line: line, err: err}
	}()
	select {
	case result := <-results:
		if result.err != nil && strings.TrimSpace(result.line) == "" {
			t.Fatalf("protocol stdout closed: %v (stderr: %s)", result.err, s.stderr.String())
		}
		trimmed := strings.TrimSpace(result.line)
		var message map[string]any
		if err := json.Unmarshal([]byte(trimmed), &message); err != nil {
			t.Fatalf("stdout line is not JSON-RPC: %q (%v)", trimmed, err)
		}
		if message["jsonrpc"] != "2.0" {
			t.Fatalf("stdout line is not JSON-RPC 2.0: %q", trimmed)
		}
		return trimmed
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for protocol output (stderr: %s)", s.stderr.String())
		return ""
	}
}

// nextMessage reads the next protocol message, including notifications and
// error responses that carry no numeric id.
func (s *stdioSession) nextMessage(t *testing.T) map[string]any {
	t.Helper()
	line := s.readLine(t)
	var message map[string]any
	if err := json.Unmarshal([]byte(line), &message); err != nil {
		t.Fatalf("decode protocol line: %v", err)
	}
	return message
}

// response reads until the response carrying id is seen. Notifications are
// skipped, so an unexpected non-JSON line still fails.
func (s *stdioSession) response(t *testing.T, id int) map[string]any {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		line := s.readLine(t)
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("decode protocol line: %v", err)
		}
		rawID, present := message["id"]
		if !present {
			continue
		}
		if number, ok := rawID.(float64); ok && int(number) == id {
			return message
		}
	}
	t.Fatalf("no response for request %d", id)
	return nil
}

func (s *stdioSession) initialize(t *testing.T) map[string]any {
	t.Helper()
	s.send(t, stdioInitRequest)
	response := s.response(t, 1)
	s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return response
}

// listTools returns the advertised tool names and their schemas.
func (s *stdioSession) listTools(t *testing.T, id int) map[string]map[string]any {
	t.Helper()
	s.send(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/list"}`, id))
	response := s.response(t, id)
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list returned no result: %v", response)
	}
	entries, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list returned no tools array: %v", result)
	}
	tools := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		tool, _ := entry.(map[string]any)
		name, _ := tool["name"].(string)
		tools[name] = tool
	}
	return tools
}

// callTool performs tools/call and returns the decoded envelope. The second
// return value is false when the server answered with a protocol error.
func (s *stdioSession) callTool(t *testing.T, id int, name string, args map[string]any) (Result, bool) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatalf("encode tool call: %v", err)
	}
	s.send(t, string(payload))
	response := s.response(t, id)
	result, ok := response["result"].(map[string]any)
	if !ok {
		return Result{}, false
	}
	raw, err := json.Marshal(result["structuredContent"])
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var envelope Result
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	isError, _ := result["isError"].(bool)
	if isError != !envelope.Success {
		t.Fatalf("isError %t disagrees with the envelope: %s", isError, raw)
	}
	return envelope, true
}

// closeInput ends the protocol stream and returns the process exit code.
func (s *stdioSession) closeInput(t *testing.T) int {
	t.Helper()
	_ = s.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- s.command.Wait() }()
	select {
	case err := <-done:
		return exitCodeOf(err)
	case <-time.After(10 * time.Second):
		_ = s.command.Process.Kill()
		t.Fatal("the management server did not exit after its input ended")
		return -1
	}
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// terminate stops the session if the test has not already done so.
func (s *stdioSession) terminate() {
	_ = s.stdin.Close()
	if s.command.Process == nil {
		return
	}
	_ = s.command.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _, _ = s.command.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = s.command.Process.Kill()
	}
}

func TestStdioAcceptanceReadOnlySession(t *testing.T) {
	fixture := newDeploymentFixture(t)
	session := startSession(t, fixture)

	init := session.initialize(t)
	result, ok := init["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize returned no result: %v", init)
	}
	serverInfo, _ := result["serverInfo"].(map[string]any)
	if serverInfo["name"] != ServerName {
		t.Fatalf("serverInfo.name = %v, want %q", serverInfo["name"], ServerName)
	}
	capabilities, _ := result["capabilities"].(map[string]any)
	if _, present := capabilities["tools"]; !present {
		t.Fatalf("initialize did not declare the tools capability: %v", capabilities)
	}

	// MCP-01: the default session exposes exactly the read catalogue.
	tools := session.listTools(t, 2)
	if got, want := sortedCopy(keysOfTools(tools)), defaultToolNames(); !equalStrings(got, want) {
		t.Fatalf("advertised tools = %v, want %v", got, want)
	}
	for name := range writeToolNames() {
		if _, present := tools[name]; present {
			t.Fatalf("%s is advertised without its startup group", name)
		}
	}

	// Every advertised schema must be a strict object.
	for name, tool := range tools {
		schema, _ := tool["inputSchema"].(map[string]any)
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatalf("%s is not a strict object schema: %v", name, schema)
		}
	}

	// Ping keeps the session alive.
	session.send(t, `{"jsonrpc":"2.0","id":3,"method":"ping"}`)
	if _, present := session.response(t, 3)["result"]; !present {
		t.Fatal("ping returned no result")
	}

	// A read call succeeds and reaches the pinned deployment.
	envelope, ok := session.callTool(t, 4, "bridgectl_cache_stats", nil)
	if !ok {
		t.Fatal("cache stats returned a protocol error")
	}
	if !envelope.Success {
		t.Fatalf("cache stats failed: %+v", envelope.Error)
	}
	stats := decodeData[CacheStatsData](t, envelope.Data)
	if stats.ExactKeys != 7 {
		t.Fatalf("cache stats = %+v", stats)
	}

	// MCP-03: a grouped tool cannot be called by name without the grant.
	denied, ok := session.callTool(t, 5, "bridgectl_cache_flush", map[string]any{
		"selector": "all", "confirmContext": "test", "confirmTarget": "all",
	})
	if ok && denied.Success {
		t.Fatal("a write tool ran without its startup group")
	}
	for _, method := range fixture.methods() {
		if method != "GET" {
			t.Fatalf("a denied write reached the deployment: %v", fixture.methods())
		}
	}

	// MCP-05: malformed JSON is a parse error, and the session continues.
	session.send(t, `{"jsonrpc":`)
	parseError := session.nextMessage(t)
	details, ok := parseError["error"].(map[string]any)
	if !ok {
		t.Fatalf("malformed JSON did not produce a JSON-RPC error: %v", parseError)
	}
	if code, _ := details["code"].(float64); code != -32700 {
		t.Fatalf("parse error code = %v, want -32700", details["code"])
	}
	session.send(t, `{"jsonrpc":"2.0","id":6,"method":"ping"}`)
	if _, present := session.response(t, 6)["result"]; !present {
		t.Fatal("the session did not survive a malformed message")
	}

	// MCP-15: the canary deployment values never reach either stream.
	for _, canary := range []string{"canary-token-value", "canary-log-message", "canary-query", "canary-config-value"} {
		if strings.Contains(session.stderr.String(), canary) {
			t.Fatalf("stderr leaked %q", canary)
		}
	}

	// MCP-14: EOF ends the session cleanly.
	if code := session.closeInput(t); code != 0 {
		t.Fatalf("exit code after EOF = %d, want 0 (stderr: %s)", code, session.stderr.String())
	}
}

func keysOfTools(tools map[string]map[string]any) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	return names
}

func TestStdioAcceptanceWriteSession(t *testing.T) {
	fixture := newDeploymentFixture(t)
	session := startSession(t, fixture, string(GroupRegistry), string(GroupResources), string(GroupDestructive), string(GroupProbe), string(GroupContext))
	home := os.Getenv("HOME")
	_ = home
	session.initialize(t)

	if got := len(session.listTools(t, 2)); got != 24 {
		t.Fatalf("advertised %d tools with every group, want 24", got)
	}

	// A completed write reports command completion, not verification.
	flushed, ok := session.callTool(t, 3, "bridgectl_cache_flush", map[string]any{
		"selector": "all", "confirmContext": "test", "confirmTarget": "all",
	})
	if !ok || !flushed.Success {
		t.Fatalf("cache flush failed: %+v", flushed.Error)
	}
	flushData := decodeData[CacheFlushData](t, flushed.Data)
	if flushData.Verified || flushData.Deleted != 3 {
		t.Fatalf("flush data = %+v", flushData)
	}

	// Mutation readback: an independent read observes the deployment again.
	readback, ok := session.callTool(t, 4, "bridgectl_cache_stats", nil)
	if !ok || !readback.Success {
		t.Fatalf("readback failed: %+v", readback.Error)
	}

	// A mismatched confirmation performs no execution.
	mismatched, ok := session.callTool(t, 5, "bridgectl_cache_flush", map[string]any{
		"selector": "all", "confirmContext": "test", "confirmTarget": "tool:other",
	})
	if !ok {
		t.Fatal("a mismatched confirmation returned a protocol error")
	}
	if mismatched.Success || mismatched.Error.Code != CodeConfirmationMismatch {
		t.Fatalf("mismatched confirmation = %+v", mismatched)
	}

	if code := session.closeInput(t); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, session.stderr.String())
	}
}

func TestStdioAcceptanceRejectsOversizedMessage(t *testing.T) {
	fixture := newDeploymentFixture(t)
	session := startSession(t, fixture)
	session.initialize(t)

	oversized := `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"bridgectl_context_list","arguments":{"pad":"` +
		strings.Repeat("x", MaxCallMessageBytes+64) + `"}}}`
	session.send(t, oversized)

	// The session must end rather than buffer the message.
	if code := session.closeInput(t); code == 0 {
		t.Log("the session ended without a distinct exit code; the input was rejected")
	}
	if strings.Contains(session.stderr.String(), strings.Repeat("x", 128)) {
		t.Fatal("the oversized message was echoed to stderr")
	}
}

func TestStdioAcceptanceAuthenticationFailure(t *testing.T) {
	server := newUnauthorizedFixture(t)
	fixture := &deploymentFixture{server: server}
	session := startSession(t, fixture)
	home := t.TempDir()
	_ = home
	session.initialize(t)

	envelope, ok := session.callTool(t, 2, "bridgectl_token_list", nil)
	if !ok {
		t.Fatal("token list returned a protocol error")
	}
	if envelope.Success {
		t.Fatal("an unauthenticated read was reported as success")
	}
	if envelope.Error.Code != CodeAuthentication {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, CodeAuthentication)
	}

	if code := session.closeInput(t); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestStdioAcceptanceClientCancellation(t *testing.T) {
	fixture := newDeploymentFixture(t)
	fixture.blockReads = 30 * time.Second
	session := startSession(t, fixture)
	session.initialize(t)

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": "tools/call",
		"params": map[string]any{"name": "bridgectl_cache_stats", "arguments": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("encode tool call: %v", err)
	}
	session.send(t, string(payload))

	// Wait until the blocked request is in flight, then cancel it.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && len(fixture.methods()) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	session.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7,"reason":"acceptance"}}`)

	response := session.response(t, 7)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("cancellation took %s; the blocked child was not stopped", elapsed)
	}
	result, _ := response["result"].(map[string]any)
	if result == nil {
		t.Fatalf("cancelled call returned no result: %v", response)
	}
	raw, err := json.Marshal(result["structuredContent"])
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var envelope Result
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Success {
		t.Fatal("a cancelled read was reported as success")
	}
	if envelope.Error.Code != CodeCancelled {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, CodeCancelled)
	}

	// The session survives a cancelled request.
	session.send(t, `{"jsonrpc":"2.0","id":8,"method":"ping"}`)
	if _, present := session.response(t, 8)["result"]; !present {
		t.Fatal("the session did not survive a cancelled request")
	}
	_ = session.closeInput(t)
}

func TestStdioAcceptanceSignalShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Process.Signal SIGTERM is POSIX-only; EOF and protocol cancellation remain covered")
	}
	fixture := newDeploymentFixture(t)
	session := startSession(t, fixture)
	session.initialize(t)

	if err := session.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- session.command.Wait() }()
	select {
	case err := <-done:
		if code := exitCodeOf(err); code != 0 {
			t.Fatalf("exit code after SIGTERM = %d, want 0 (stderr: %s)", code, session.stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = session.command.Process.Kill()
		t.Fatal("the management server did not stop within the shutdown budget")
	}
}

func TestStdioAcceptanceInterruptedWriteReportsUnknownOutcome(t *testing.T) {
	fixture := newDeploymentFixture(t)
	fixture.blockWrites = 30 * time.Second
	session := startSession(t, fixture, string(GroupResources))
	session.initialize(t)

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 11, "method": "tools/call",
		"params": map[string]any{"name": "bridgectl_tool_apply", "arguments": validWriteArgs(t, "bridgectl_tool_apply")},
	})
	if err != nil {
		t.Fatalf("encode tool call: %v", err)
	}
	session.send(t, string(payload))

	// Wait until the deployment has received the write, then interrupt it.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && len(fixture.methods()) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(fixture.methods()) == 0 {
		t.Fatal("the write never reached the deployment")
	}

	start := time.Now()
	session.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":11,"reason":"acceptance"}}`)
	response := session.response(t, 11)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("interrupting the write took %s", elapsed)
	}

	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("interrupted write returned no result: %v", response)
	}
	raw, err := json.Marshal(result["structuredContent"])
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var envelope Result
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Success {
		t.Fatal("an interrupted write was reported as success")
	}
	if envelope.Error.Code != CodeOutcomeUnknown {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, CodeOutcomeUnknown)
	}
	if envelope.Error.Cause == "" {
		t.Fatal("an unknown outcome carried no cause")
	}
	if !strings.Contains(envelope.Error.Suggestion, "Read the target resource") {
		t.Fatalf("suggestion = %q, want read-back guidance", envelope.Error.Suggestion)
	}
	if strings.Contains(string(raw), "roll") {
		t.Fatalf("the result claims a rollback: %s", raw)
	}

	// The session stays usable, and an independent read observes the deployment.
	readback, ok := session.callTool(t, 12, "bridgectl_tool_get", map[string]any{"name": "list_items"})
	if !ok || !readback.Success {
		t.Fatalf("readback after an interrupted write failed: %+v", readback.Error)
	}

	_ = session.closeInput(t)
}

func TestStdioAcceptanceUsesThePinnedCredential(t *testing.T) {
	fixture := newDeploymentFixture(t)
	home := t.TempDir()
	writeContextConfigWithToken(t, home, fixture.server.URL, "pinned-acceptance-token")

	command := exec.Command(realCLI(t), "mcp", "--stdio", "--context", "test") // #nosec G204 G702 -- fixed test argv
	command.Env = acceptanceEnvironment(t, home)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr := &syncBuffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start management server: %v", err)
	}
	session := &stdioSession{command: command, stdin: stdin, reader: bufio.NewReader(stdout), stderr: stderr}
	t.Cleanup(session.terminate)

	session.initialize(t)
	if _, ok := session.callTool(t, 2, "bridgectl_cache_stats", nil); !ok {
		t.Fatal("cache stats returned a protocol error")
	}
	headers := fixture.authorizationHeaders()
	if len(headers) == 0 {
		t.Fatal("the deployment saw no request")
	}
	if headers[0] != "Bearer pinned-acceptance-token" {
		t.Fatalf("Authorization = %q, want the pinned credential", headers[0])
	}

	_ = session.closeInput(t)
}
