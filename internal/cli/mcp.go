package cli

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nmdra/ERPBridge/internal/climcp"
	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/spf13/cobra"
)

var (
	mcpStdio      bool
	mcpAllowWrite []string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Serve ERPBridge management operations over MCP stdio",
	Long: `Serve a fixed set of ERPBridge management operations to an MCP client.

This command is for AI coding agents. It is separate from the ERPBridge runtime,
which serves published ERP tools. Management tools cover the local API registry,
tool, plugin, binding, token, cache, and context operations that already exist
in this CLI.

The session is pinned to the selected context. Changing the saved default
context does not retarget a running session; restart this process to adopt
changed endpoints or credentials.

Stdout carries the MCP protocol stream. Diagnostics are written to stderr.

By default only inspection, local validation, and generation are available.
Repeat --allow-write to approve specific write groups.`,
	Example: `  bridgectl mcp --stdio
  bridgectl mcp --stdio --context staging
  bridgectl mcp --stdio --context staging --allow-write registry,resources`,
	Args: cobra.NoArgs,
	RunE: runMCP,
}

func runMCP(cmd *cobra.Command, _ []string) error {
	if !mcpStdio {
		return NewError(CodeBadArgs, "MCP_STDIO_REQUIRED",
			"the mcp command requires --stdio",
			"Run 'bridgectl mcp --stdio'.")
	}
	contextSettings, err := cfg.EffectiveContext()
	if err != nil {
		return err
	}
	groups := parseAllowWrite(mcpAllowWrite)
	snapshot := climcp.Snapshot{
		ContextName: cfg.CurrentContext,
		Server:      contextSettings.Server,
		MCPServer:   contextSettings.MCPServer,
		APIToken:    bridgeAPIToken(),
	}
	session, err := climcp.NewServer(climcp.Options{
		Snapshot: snapshot,
		Version:  Version,
		Groups:   groups,
		Stdin:    cmd.InOrStdin(),
		Stdout:   cmd.OutOrStdout(),
	})
	if err != nil {
		return err
	}

	serveContext := cmd.Context()
	if serveContext == nil {
		serveContext = context.Background()
	}
	serveContext, stop := signal.NotifyContext(serveContext, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := session.Serve(serveContext)
	if errors.Is(serveErr, context.Canceled) && serveContext.Err() != nil {
		// A signal ends the session cleanly, as it does for bridgectl web.
		return nil
	}
	return serveErr
}

// parseAllowWrite expands repeatable, comma-separated write groups. Unknown
// names are rejected by the adapter rather than ignored.
func parseAllowWrite(values []string) []climcp.WriteGroup {
	var groups []climcp.WriteGroup
	seen := make(map[string]bool)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if seen[part] {
				continue
			}
			seen[part] = true
			groups = append(groups, climcp.WriteGroup(part))
		}
	}
	return groups
}

// enableServeStdioMode keeps the MCP protocol stream exclusive. It must run
// before logger.Init so pre-run and transport diagnostics never reach stdout.
func enableServeStdioMode() {
	serveStdioMode = true
	_ = os.Setenv("LOG_TO_STDERR", "true")
}

// commandIsMCPServe reports whether this invocation resolves to the mcp
// command. It uses Cobra's own resolver so flag parsing failures after launch
// cannot leak a JSON error document onto the protocol stream.
func commandIsMCPServe() bool {
	resolved, _, err := RootCmd.Find(os.Args[1:])
	if err != nil {
		return false
	}
	return currentCommandIsMCPServe(resolved)
}

// currentCommandIsMCPServe reports whether cmd is, or is nested under, the mcp
// command.
func currentCommandIsMCPServe(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current == mcpCmd {
			return true
		}
	}
	return false
}

// verifyChildSnapshot enforces the launch-time selection for a child process
// spawned by the management MCP adapter. A mismatch fails the call instead of
// silently using a different context, endpoint, or credential.
func verifyChildSnapshot(contextName string, contextSettings config.Context) error {
	expected, present := os.LookupEnv(climcp.ChildSnapshotEnv)
	if !present || expected == "" {
		return nil
	}
	actual := climcp.Snapshot{
		ContextName: contextName,
		Server:      contextSettings.Server,
		MCPServer:   contextSettings.MCPServer,
		APIToken:    bridgeAPIToken(),
	}.Fingerprint()
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return NewError(CodePrecondFail, "MCP_CONTEXT_CHANGED",
			"the management MCP session selection no longer matches the saved configuration",
			"Restart the management MCP process to adopt the changed context, endpoint, or credential.")
	}
	return nil
}

// allowWriteHelp renders the accepted groups for flag documentation.
func allowWriteHelp() string {
	groups := make([]string, 0, len(climcp.WriteGroups()))
	for _, group := range climcp.WriteGroups() {
		groups = append(groups, string(group))
	}
	return fmt.Sprintf("Approve a write group (repeatable, comma-separated): %s", strings.Join(groups, ", "))
}

func init() {
	RootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().BoolVar(&mcpStdio, "stdio", false, "Serve the MCP protocol on stdin/stdout (required)")
	mcpCmd.Flags().StringArrayVar(&mcpAllowWrite, "allow-write", nil, allowWriteHelp())
}
