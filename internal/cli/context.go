package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage bridgectl contexts",
	Long: `The context command allows you to switch between different ERPBridge 
environments (e.g., local, staging, production). Each context defines the 
middleware server URL, default ERP base URL, and authentication credentials.`,
}

var contextListCmd = &cobra.Command{
	Use:     cliListUse,
	Short:   "List saved contexts",
	Long:    `Display a table of all configured contexts from your ~/.bridgectl/config.yaml.`,
	Example: `  bridgectl context list`,
	RunE: func(_ *cobra.Command, _ []string) error {
		var items []ContextItem
		for name := range cfg.Contexts {
			current := name == cfg.CurrentContext
			items = append(items, ContextItem{
				Name:    name,
				Server:  cfg.Contexts[name].Server,
				Current: current,
			})
		}
		resp := &ContextListResponse{Items: items}
		return formatter.Print(resp)
	},
}

// ContextItem represents a single configured environment in the CLI.
type ContextItem struct {
	Name    string `json:"name"`
	Server  string `json:"server"`
	Current bool   `json:"current"`
}

// ContextListResponse wraps a list of ContextItem for table rendering.
type ContextListResponse struct {
	Items []ContextItem `json:"items"`
}

// RenderTable implements the output.TableRenderer interface.
func (r *ContextListResponse) RenderTable(w io.Writer) error {
	tw := output.NewTabWriter(w)
	_, _ = fmt.Fprintln(tw, "NAME\tSERVER\tCURRENT")
	for _, item := range r.Items {
		curr := ""
		if item.Current {
			curr = "✓"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", item.Name, item.Server, curr)
	}
	return tw.Flush()
}

var contextSetCmd = &cobra.Command{
	Use:     "set [name]",
	Short:   "Switch active context",
	Long:    `Update the active context in your configuration file. All subsequent commands will use this context unless overridden by the --context flag.`,
	Example: `  bridgectl context set production`,
	Args:    cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]
		if _, ok := cfg.Contexts[name]; !ok {
			return fmt.Errorf("context %s not found", name)
		}
		cfg.CurrentContext = name
		return saveConfig()
	},
}

// saveConfig writes the current CLI configuration to ~/.bridgectl/config.yaml.
func saveConfig() error {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".bridgectl", "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func init() {
	RootCmd.AddCommand(contextCmd)
	contextCmd.AddCommand(contextListCmd)
	contextCmd.AddCommand(contextSetCmd)
}
