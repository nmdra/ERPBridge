// Package cli implements the command-line interface for bridgectl,
// providing tools for managing ERPBridge environments and APIs.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var (
	outputFormat  string
	ctxOverride   string
	tokenOverride string
	verbose       bool

	// Version is the current version of the bridgectl CLI.
	Version = "dev"

	cfg       *config.Config
	formatter *output.Formatter

	// RootLog is the global logger instance used by the CLI.
	RootLog *slog.Logger
)

const (
	cliListUse   = "list"
	cliNameField = "name"
	cliMCPScope  = "mcp"
)

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:           "bridgectl",
	Short:         "Middleware for Bridging Legacy ERP and Agentic AI",
	Version:       Version,
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: `bridgectl is the developer CLI for the ERPBridge ecosystem. 
It provides tools to manage environments, register and test ERP APIs, 
generate and validate MCP tool schemas, and monitor the middleware's 
health through real-time log streaming and cache analytics.

The CLI interacts with the ERPBridge middleware via a REST API 
and supports multiple output formats including Table, JSON, and YAML.`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Initialize Logger for CLI
		if verbose {
			_ = os.Setenv("LOG_LEVEL", "debug")
		} else {
			_ = os.Setenv("LOG_LEVEL", "error") // Only errors in CLI by default
		}
		RootLog = logger.Init()

		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if ctxOverride != "" {
			cfg.CurrentContext = ctxOverride
		}

		formatter = &output.Formatter{
			Format: output.Format(outputFormat),
			Out:    cmd.OutOrStdout(),
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		handleError(err)
	}
}

func handleError(err error) {
	if aErr, ok := errors.AsType[*AgentActionableError](err); ok {
		if outputFormat == "json" {
			// In JSON mode, only the error object goes to Stdout
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(aErr)
		} else {
			// Human-readable error to Stderr
			fmt.Fprintf(os.Stderr, "Error: [%s] %s\n", aErr.ErrorCode, aErr.Message)
			if aErr.Suggestion != "" {
				fmt.Fprintf(os.Stderr, "Suggestion: %s\n", aErr.Suggestion)
			}
		}
		os.Exit(aErr.Code)
	}

	// General error
	fmt.Fprintln(os.Stderr, err)
	os.Exit(CodeGeneralErr)
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table, json, yaml")
	RootCmd.PersistentFlags().StringVarP(&ctxOverride, "context", "c", "", "Override active context")
	RootCmd.PersistentFlags().StringVar(&tokenOverride, "token", "", "API token for the ERPBridge server")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show full HTTP request/response detail")
}
