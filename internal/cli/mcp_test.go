package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/climcp"
	"github.com/nmdra/ERPBridge/internal/config"

	"github.com/nmdra/ERPBridge/internal/testutil"
)

// helperArgsEnv transports the CLI argument vector into the helper process.
const helperArgsEnv = "CLIMCP_CLI_HELPER_ARGS"

// TestHelperCLIProcess runs the real CLI entry point in a child process so the
// process exit code and stream selection can be observed. handleError calls
// os.Exit, so it cannot be observed in-process.
func TestHelperCLIProcess(_ *testing.T) {
	raw := os.Getenv(helperArgsEnv)
	if raw == "" {
		return
	}
	os.Args = append([]string{"bridgectl"}, strings.Split(raw, "\x1f")...)
	Execute()
	os.Exit(0)
}

type cliProcessResult struct {
	stdout string
	stderr string
	code   int
}

// runCLIProcess executes the CLI in a child process with an isolated home and
// no inherited bridge credentials.
func runCLIProcess(t *testing.T, args ...string) cliProcessResult {
	t.Helper()
	return runCLIProcessIn(t, t.TempDir(), args...)
}

// runCLIProcessIn executes the CLI in a child process that uses the supplied
// home directory, so a test can seed a saved context before the run.
func runCLIProcessIn(t *testing.T, home string, args ...string) cliProcessResult {
	t.Helper()
	env := testutil.HomeEnvironment(os.Environ(), home, helperArgsEnv, climcp.ChildSnapshotEnv)
	env = append(env, helperArgsEnv+"="+strings.Join(args, "\x1f"))

	// #nosec G204 G702 -- the test binary re-executes itself with a fixed
	// argument vector; the CLI arguments travel through the environment.
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperCLIProcess$", "--")
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := errors.AsType[*exec.ExitError](err)
		if !ok {
			t.Fatalf("run helper process: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return cliProcessResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func TestMCPStartupErrorNeverReachesProtocolStdout(t *testing.T) {
	result := runCLIProcess(t, "-o", "json", "mcp")
	if result.stdout != "" {
		t.Fatalf("protocol stdout carried a CLI error document: %q", result.stdout)
	}
	if !strings.Contains(result.stderr, "MCP_STDIO_REQUIRED") {
		t.Fatalf("stderr = %q, want the MCP_STDIO_REQUIRED error", result.stderr)
	}
	if result.code != CodeBadArgs {
		t.Fatalf("exit code = %d, want %d", result.code, CodeBadArgs)
	}
}

func TestMCPFlagParseFailureNeverReachesProtocolStdout(t *testing.T) {
	result := runCLIProcess(t, "-o", "json", "mcp", "--not-a-real-flag")
	if result.stdout != "" {
		t.Fatalf("protocol stdout carried a parse failure: %q", result.stdout)
	}
	if strings.TrimSpace(result.stderr) == "" {
		t.Fatal("parse failure produced no stderr diagnostic")
	}
}

func TestOrdinaryJSONErrorsStillUseStdout(t *testing.T) {
	result := runCLIProcess(t, "-o", "json", "api", "list", "--context", "nonexistent")
	if strings.TrimSpace(result.stderr) != "" {
		t.Fatalf("ordinary CLI run wrote to stderr: %q", result.stderr)
	}
	if !strings.Contains(result.stdout, "CONTEXT_NOT_FOUND") {
		t.Fatalf("stdout = %q, want a JSON context error", result.stdout)
	}
	if result.code != CodeNotFound {
		t.Fatalf("exit code = %d, want %d", result.code, CodeNotFound)
	}
}

const (
	testInitLine = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	testPingLine = `{"jsonrpc":"2.0","id":2,"method":"ping"}`
)

// isolateConfigHome points configuration resolution at a temporary home and
// clears inherited bridge variables so a test cannot read the developer's own
// context file or credentials.
func isolateConfigHome(t *testing.T) {
	t.Helper()
	testutil.SetHome(t, t.TempDir())
	for _, key := range []string{
		"BRIDGE_CONTEXT", "BRIDGE_SERVER", "BRIDGE_MCP_SERVER", "BRIDGE_API_TOKEN",
		"BRIDGE_ERP_BASE", "BRIDGE_AUTH_TYPE", "BRIDGE_API_KEY", "BRIDGE_AUTH_HEADER",
		"BRIDGE_TOKEN", "BRIDGE_USERNAME", "BRIDGE_PASSWORD",
		climcp.ChildSnapshotEnv, "LOG_TO_STDERR",
	} {
		t.Setenv(key, "")
	}
}

func resetPreRunState(t *testing.T) {
	t.Helper()
	mcpStdio = false
	mcpAllowWrite = nil
	outputFormat = "table"
	ctxOverride = ""
	tokenOverride = ""
	serveStdioMode = false
	t.Cleanup(func() {
		mcpStdio = false
		mcpAllowWrite = nil
		outputFormat = "table"
		ctxOverride = ""
		tokenOverride = ""
		serveStdioMode = false
	})
}

func runRootCommand(t *testing.T, args []string, stdin string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	RootCmd.SetArgs(args)
	RootCmd.SetOut(&stdout)
	RootCmd.SetErr(&stderr)
	RootCmd.SetIn(strings.NewReader(stdin))
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
		RootCmd.SetIn(nil)
	})
	err := RootCmd.Execute()
	return stdout.String(), err
}

func TestMCPCommandRequiresStdio(t *testing.T) {
	isolateConfigHome(t)
	resetPreRunState(t)

	_, err := runRootCommand(t, []string{"mcp"}, "")
	if err == nil {
		t.Fatal("mcp without --stdio succeeded")
	}
	actionable, ok := errors.AsType[*AgentActionableError](err)
	if !ok {
		t.Fatalf("error = %v, want an actionable CLI error", err)
	}
	if actionable.ErrorCode != "MCP_STDIO_REQUIRED" {
		t.Fatalf("error code = %q, want MCP_STDIO_REQUIRED", actionable.ErrorCode)
	}
}

func TestMCPCommandServesProtocolOnStdout(t *testing.T) {
	isolateConfigHome(t)
	resetPreRunState(t)

	stdout, err := runRootCommand(t, []string{"mcp", "--stdio"}, testInitLine+"\n"+testPingLine+"\n")
	if err != nil {
		t.Fatalf("mcp --stdio returned error: %v", err)
	}
	if !strings.Contains(stdout, `"name":"bridgectl"`) {
		t.Fatalf("initialize response did not identify the bridgectl server: %q", stdout)
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("stdout line is not a JSON-RPC message: %q (%v)", line, err)
		}
	}
}

func TestMCPCommandRejectsUnknownWriteGroup(t *testing.T) {
	isolateConfigHome(t)
	resetPreRunState(t)

	_, err := runRootCommand(t, []string{"mcp", "--stdio", "--allow-write", "everything"}, "")
	if err == nil {
		t.Fatal("mcp accepted an unknown write group")
	}
	if !strings.Contains(err.Error(), "unknown write group") {
		t.Fatalf("error = %v, want an unknown write group error", err)
	}
}

func TestParseAllowWriteExpandsAndDeduplicates(t *testing.T) {
	groups := parseAllowWrite([]string{"registry,resources", "registry", "", " probe "})
	want := []climcp.WriteGroup{climcp.GroupRegistry, climcp.GroupResources, climcp.GroupProbe}
	if len(groups) != len(want) {
		t.Fatalf("groups = %v, want %v", groups, want)
	}
	for i := range want {
		if groups[i] != want[i] {
			t.Fatalf("groups = %v, want %v", groups, want)
		}
	}
}

func TestCurrentCommandIsMCPServeMatchesOnlyMCPCommand(t *testing.T) {
	if !currentCommandIsMCPServe(mcpCmd) {
		t.Fatal("the mcp command was not recognized")
	}
	if currentCommandIsMCPServe(contextCmd) {
		t.Fatal("an unrelated command was treated as the mcp command")
	}
	if currentCommandIsMCPServe(nil) {
		t.Fatal("a nil command was treated as the mcp command")
	}
}

func TestEnableServeStdioModeKeepsStdoutForProtocol(t *testing.T) {
	isolateConfigHome(t)
	resetPreRunState(t)

	enableServeStdioMode()
	if !serveStdioMode {
		t.Fatal("serveStdioMode was not set")
	}
	if errorOutputIsStdout() {
		t.Fatal("an actionable error would be written to the protocol stream")
	}

	outputFormat = "json"
	if errorOutputIsStdout() {
		t.Fatal("-o json still targeted stdout in serve stdio mode")
	}

	serveStdioMode = false
	if !errorOutputIsStdout() {
		t.Fatal("ordinary CLI runs no longer use stdout for JSON errors")
	}
}

func TestMCPCommandExitsCleanlyOnCancellation(t *testing.T) {
	isolateConfigHome(t)
	setupTest()
	resetPreRunState(t)
	mcpStdio = true

	stdinReader, stdinWriter := io.Pipe()
	defer func() { _ = stdinWriter.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	mcpCmd.SetContext(ctx)
	t.Cleanup(func() { mcpCmd.SetContext(context.Background()) })

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	mcpCmd.SetOut(stdout)
	mcpCmd.SetErr(stderr)
	mcpCmd.SetIn(stdinReader)
	t.Cleanup(func() {
		mcpCmd.SetOut(nil)
		mcpCmd.SetErr(nil)
		mcpCmd.SetIn(nil)
	})

	done := make(chan error, 1)
	go func() { done <- runMCP(mcpCmd, nil) }()
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runMCP returned %v, want a clean exit on cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runMCP did not stop after cancellation")
	}
}

func TestVerifyChildSnapshotAcceptsMatchingSelection(t *testing.T) {
	isolateConfigHome(t)
	setupTest()

	settings := config.Context{Server: "http://test"}
	expected := climcp.Snapshot{ContextName: "test", Server: settings.Server, MCPServer: settings.MCPServer}.Fingerprint()
	t.Setenv(climcp.ChildSnapshotEnv, expected)

	if err := verifyChildSnapshot("test", settings); err != nil {
		t.Fatalf("verifyChildSnapshot rejected a matching selection: %v", err)
	}
}

func TestVerifyChildSnapshotRejectsChangedSelection(t *testing.T) {
	isolateConfigHome(t)
	setupTest()

	settings := config.Context{Server: "http://test"}
	stale := climcp.Snapshot{ContextName: "test", Server: "http://other", MCPServer: settings.MCPServer}.Fingerprint()
	t.Setenv(climcp.ChildSnapshotEnv, stale)

	err := verifyChildSnapshot("test", settings)
	if err == nil {
		t.Fatal("verifyChildSnapshot accepted a changed endpoint")
	}
	actionable, ok := errors.AsType[*AgentActionableError](err)
	if !ok {
		t.Fatalf("error = %v, want an actionable CLI error", err)
	}
	if actionable.ErrorCode != "MCP_CONTEXT_CHANGED" {
		t.Fatalf("error code = %q, want MCP_CONTEXT_CHANGED", actionable.ErrorCode)
	}

	tokenStale := climcp.Snapshot{ContextName: "test", Server: settings.Server, MCPServer: settings.MCPServer, APIToken: "newly-configured-token"}.Fingerprint()
	t.Setenv(climcp.ChildSnapshotEnv, tokenStale)
	if err := verifyChildSnapshot("test", settings); err == nil {
		t.Fatal("verifyChildSnapshot accepted a newly configured token")
	}

	// Absent to present: the session started without a token and the child now
	// resolves one. Environment pinning cannot cover this, so the fingerprint must.
	absent := climcp.Snapshot{ContextName: "test", Server: settings.Server}.Fingerprint()
	t.Setenv(climcp.ChildSnapshotEnv, absent)
	t.Setenv("BRIDGE_API_TOKEN", "appeared-during-session")
	if err := verifyChildSnapshot("test", settings); err == nil {
		t.Fatal("verifyChildSnapshot accepted a token that appeared during the session")
	}

	// Absent to present endpoint: the same gap for an empty MCP endpoint.
	t.Setenv("BRIDGE_API_TOKEN", "")
	absentEndpoint := climcp.Snapshot{ContextName: "test", Server: settings.Server}.Fingerprint()
	t.Setenv(climcp.ChildSnapshotEnv, absentEndpoint)
	if err := verifyChildSnapshot("test", config.Context{Server: settings.Server, MCPServer: "http://mcp-appeared"}); err == nil {
		t.Fatal("verifyChildSnapshot accepted an endpoint that appeared during the session")
	}
}

func TestVerifyChildSnapshotIsSkippedWithoutMarker(t *testing.T) {
	isolateConfigHome(t)
	setupTest()

	if err := verifyChildSnapshot("test", config.Context{Server: "http://anything"}); err != nil {
		t.Fatalf("verifyChildSnapshot applied to an ordinary CLI run: %v", err)
	}
}
