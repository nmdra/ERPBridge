// Package cli implements the command-line interface for bridgectl,
// providing tools for managing ERPBridge environments and APIs.
package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
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
	cliListUse           = "list"
	cliNameField         = "name"
	cliMCPScope          = "mcp"
	cliApplyFileUse      = "apply -f [file]"
	cliValidateFileUse   = "validate -f [file]"
	cliContentTypeHeader = "Content-Type"
	cliJSONContentType   = "application/json"
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
and supports multiple output formats including Table, JSON, and YAML.

Control-plane failures use stable error codes, safe messages, and remediation
suggestions. They do not expose upstream response bodies or credentials.`,
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
			if errors.Is(err, config.ErrContextNotFound) {
				return NewError(CodeNotFound, "CONTEXT_NOT_FOUND", err.Error(), "Run 'bridgectl context list' or select a configured context with --context.")
			}
			return fmt.Errorf("load config: %w", err)
		}

		if ctxOverride != "" {
			cfg.CurrentContext = ctxOverride
		}
		if _, err := cfg.EffectiveContext(); err != nil {
			if errors.Is(err, config.ErrContextNotFound) {
				return NewError(CodeNotFound, "CONTEXT_NOT_FOUND", err.Error(), "Run 'bridgectl context list' or select a configured context with --context.")
			}
			return err
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
	err = mapKnownCLIError(err)
	if aErr, ok := errors.AsType[*AgentActionableError](err); ok {
		if outputFormat == "json" {
			_ = renderActionableError(os.Stdout, aErr, outputFormat)
		} else {
			_ = renderActionableError(os.Stderr, aErr, outputFormat)
		}
		os.Exit(aErr.Code)
	}

	// General errors are local validation failures. Remote HTTP errors are
	// converted before they reach this fallback, so response bodies are never
	// printed here.
	fmt.Fprintln(os.Stderr, err)
	os.Exit(CodeGeneralErr)
}

func mapKnownCLIError(err error) error {
	switch {
	case errors.Is(err, config.ErrContextNotFound):
		return NewError(CodeNotFound, "CONTEXT_NOT_FOUND", "the selected context was not found", "run 'bridgectl context list' or select a configured context")
	case errors.Is(err, idp.ErrLegacyRegistry), errors.Is(err, idp.ErrLegacyCredentials):
		return NewError(CodePrecondFail, "LEGACY_REGISTRY", "the registry requires an explicit credential-safe migration", "run 'bridgectl api scrub-credentials' or migrate the legacy registry")
	case errors.Is(err, idp.ErrRegistryConflict):
		return NewError(CodeConflict, "REGISTRY_CONFLICT", "an API with that name already exists", "use --force only when replacement is intentional")
	default:
		return err
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table, json, yaml")
	RootCmd.PersistentFlags().StringVarP(&ctxOverride, "context", "c", "", "Override active context")
	RootCmd.PersistentFlags().StringVar(&tokenOverride, "token", "", "API token for the ERPBridge server")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show full HTTP request/response detail")
}
