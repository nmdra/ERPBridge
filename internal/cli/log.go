// Package cli implements the command line interface commands for bridgectl.
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Manage and view logs",
	Long: `The log command provides utilities to monitor the middleware's execution. 
You can stream live logs as they happen or view a summary of recent events 
to identify trends and frequent errors.`,
}

var (
	logComponent string
	logTool      string
	logLevel     string
	logRequestID string
)

var logStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Summarise log events from the middleware",
	Long: `Fetch the most recent logs from the middleware and perform a basic 
frequency analysis of log levels and tool invocations.`,
	Example: `  bridgectl log stats`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, err := cfg.EffectiveContext()
		if err != nil {
			return err
		}
		if err := ValidateServerURL(ctx.Server, "BRIDGE", cfg.CurrentContext); err != nil {
			return err
		}

		url := ctx.Server + "/api/logs/recent"
		resp, err := doBridgeRequest(cmd, http.MethodGet, url)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		var logs []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
			return err
		}

		// Analysis
		counts := make(map[string]int)
		tools := make(map[string]int)
		for _, l := range logs {
			level, _ := l["level"].(string)
			counts[level]++

			if tool, ok := l["tool_name"].(string); ok && tool != "" {
				tools[tool]++
			}
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintln(out, "Log Statistics (Recent 1000 events)")
		_, _ = fmt.Fprintln(out, "\nLevel breakdown:")
		for level, count := range counts {
			_, _ = fmt.Fprintf(out, "  %-10s %d\n", level, count)
		}

		_, _ = fmt.Fprintln(out, "\nTop tools by call count:")
		for tool, count := range tools {
			_, _ = fmt.Fprintf(out, "  %-25s %d calls\n", tool, count)
		}

		return nil
	},
}

var logTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Stream live logs from the middleware",
	Long: `Connect to the middleware's SSE log stream and display structured 
log messages in real-time. You can filter the stream by component, tool, 
log level, or a specific request ID.`,
	Example: `  bridgectl log tail
  bridgectl log tail --level error
  bridgectl log tail --tool finance.get-invoices
  bridgectl log tail --component mcp`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, err := cfg.EffectiveContext()
		if err != nil {
			return err
		}
		if err := ValidateServerURL(ctx.Server, "BRIDGE", cfg.CurrentContext); err != nil {
			return err
		}

		url := ctx.Server + "/api/logs/stream"
		resp, err := doBridgeRequest(cmd, http.MethodGet, url)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server error: %s", resp.Status)
		}

		out := cmd.OutOrStdout()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			if after, ok := strings.CutPrefix(line, "data: "); ok {
				msg := after
				msg = strings.TrimSpace(msg)

				// For simplicity, we'll just print it.
				// In a real app, we'd parse JSON and filter based on flags.
				if shouldPrint(msg) {
					_, _ = fmt.Fprintln(out, msg)
				}
			}
		}

		return nil
	},
}

func shouldPrint(msg string) bool {
	var fields map[string]any
	if err := json.Unmarshal([]byte(msg), &fields); err != nil {
		return true
	}
	fieldMatches := func(name, expected string) bool {
		if expected == "" {
			return true
		}
		value, ok := fields[name].(string)
		return ok && value == expected
	}
	return fieldMatches("component", logComponent) &&
		fieldMatches("tool_name", logTool) &&
		fieldMatches("level", strings.ToUpper(logLevel)) &&
		fieldMatches("request_id", logRequestID)
}

func init() {
	RootCmd.AddCommand(logCmd)
	logCmd.AddCommand(logTailCmd)
	logCmd.AddCommand(logStatsCmd)

	logTailCmd.Flags().StringVar(&logComponent, "component", "", "Filter by component")
	logTailCmd.Flags().StringVar(&logTool, "tool", "", "Filter by tool name")
	logTailCmd.Flags().StringVar(&logLevel, "level", "", "Filter by log level")
	logTailCmd.Flags().StringVar(&logRequestID, "request-id", "", "Filter by request ID")
}
