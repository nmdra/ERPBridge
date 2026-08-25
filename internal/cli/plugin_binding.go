package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var pluginBindingCmd = &cobra.Command{
	Use:   "binding",
	Short: "Manage external plugin bindings",
}

func decodePluginBindingDocuments(data []byte, filePath string) ([]mcp.PluginBinding, error) {
	return decodeResourceDocuments[mcp.PluginBinding](data, filePath)
}

var pluginBindingApplyCmd = &cobra.Command{
	Use:   cliApplyFileUse,
	Short: "Apply one or more external plugin bindings",
	RunE: func(cmd *cobra.Command, _ []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			return fmt.Errorf("file path is required")
		}
		baseURL, err := pluginServerBase()
		if err != nil {
			return err
		}
		return applyResourceFiles(filePath, func(path string, data []byte) error {
			bindings, err := decodePluginBindingDocuments(data, path)
			if err != nil {
				return err
			}
			for _, binding := range bindings {
				if err := binding.Validate(); err != nil {
					return fmt.Errorf("invalid plugin binding (%s): %w", path, err)
				}
				payload, err := json.Marshal(binding)
				if err != nil {
					return fmt.Errorf("marshal plugin binding (%s): %w", path, err)
				}
				resp, err := doBridgeRequestWithHeaders(cmd, http.MethodPost, baseURL+"/apis/erpbridge.io/v1/pluginbindings", bytes.NewReader(payload), http.Header{cliContentTypeHeader: []string{cliJSONContentType}})
				if err != nil {
					return fmt.Errorf("apply failed (%s): %w", path, err)
				}
				body, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr != nil {
					return fmt.Errorf("read apply response (%s): %w", path, readErr)
				}
				if resp.StatusCode >= http.StatusBadRequest {
					return fmt.Errorf("server error (%d) for %s: %s", resp.StatusCode, path, string(body))
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "plugin binding %s applied successfully\n", binding.Metadata.Name)
			}
			return nil
		})
	},
}

var pluginBindingGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Display one or more external plugin bindings",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseURL, err := pluginServerBase()
		if err != nil {
			return err
		}
		listURL := baseURL + "/apis/erpbridge.io/v1/pluginbindings"
		if len(args) == 1 {
			listURL += "?" + url.Values{cliNameField: []string{args[0]}}.Encode()
		}
		resp, err := doBridgeRequest(cmd, http.MethodGet, listURL)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
		}
		var bindings []*mcp.PluginBinding
		if err := json.NewDecoder(resp.Body).Decode(&bindings); err != nil {
			return err
		}
		if len(args) == 1 {
			var target *mcp.PluginBinding
			for _, binding := range bindings {
				if binding.Metadata.Name == args[0] {
					target = binding
					break
				}
			}
			if target == nil {
				return fmt.Errorf("plugin binding %s not found", args[0])
			}
			return renderPluginBinding(cmd.OutOrStdout(), cmd, target, nil)
		}
		return renderPluginBinding(cmd.OutOrStdout(), cmd, nil, bindings)
	},
}

var pluginBindingValidateCmd = &cobra.Command{
	Use:   cliValidateFileUse,
	Short: "Locally validate plugin binding definitions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			return fmt.Errorf("file path is required")
		}
		// #nosec G304 -- path is supplied via a CLI flag.
		data, err := os.ReadFile(filepath.Clean(filePath))
		if err != nil {
			return err
		}
		bindings, err := decodePluginBindingDocuments(data, filePath)
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
		for _, binding := range bindings {
			if err := binding.Validate(); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ plugin binding %s is locally valid\n", binding.Metadata.Name)
		}
		return nil
	},
}

var pluginBindingDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Remove an external plugin binding",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		hard, _ := cmd.Flags().GetBool("hard")
		yes, _ := cmd.Flags().GetBool("yes")
		if hard && !yes {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "WARNING: This will permanently delete plugin binding '%s'. Are you sure? (y/N): ", name)
			var response string
			_, _ = fmt.Fscan(cmd.InOrStdin(), &response)
			if response != "y" && response != "Y" && response != "yes" && response != "YES" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Operation aborted.")
				return nil
			}
		}
		baseURL, err := pluginServerBase()
		if err != nil {
			return err
		}
		query := url.Values{cliNameField: []string{name}}
		if hard {
			query.Set("hard", "true")
		}
		resp, err := doBridgeRequest(cmd, http.MethodDelete, baseURL+"/apis/erpbridge.io/v1/pluginbindings?"+query.Encode())
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("delete failed (%d): %s", resp.StatusCode, string(body))
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "plugin binding %s deleted successfully\n", name)
		return nil
	},
}

func renderPluginBinding(out io.Writer, cmd *cobra.Command, target *mcp.PluginBinding, bindings []*mcp.PluginBinding) error {
	format, _ := cmd.Flags().GetString("output")
	if target != nil {
		switch format {
		case string(output.FormatYAML):
			return yaml.NewEncoder(out).Encode(target)
		case string(output.FormatJSON):
			encoder := json.NewEncoder(out)
			encoder.SetIndent("", "  ")
			return encoder.Encode(target)
		default:
			return (&PluginBindingListResponse{Bindings: []*mcp.PluginBinding{target}}).RenderTable(out)
		}
	}
	response := &PluginBindingListResponse{Bindings: bindings}
	if format == string(output.FormatJSON) {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(response)
	}
	if format == string(output.FormatYAML) {
		return yaml.NewEncoder(out).Encode(response)
	}
	return response.RenderTable(out)
}

// PluginBindingListResponse wraps binding resources for output formatting.
type PluginBindingListResponse struct {
	Bindings []*mcp.PluginBinding `json:"bindings" yaml:"bindings"`
}

// RenderTable renders binding identities and references.
func (r *PluginBindingListResponse) RenderTable(w io.Writer) error {
	tw := output.NewTabWriter(w)
	_, _ = fmt.Fprintln(tw, "NAME\tPLUGIN\tTOOL\tPHASE\tPOLICY\tSTATUS")
	for _, binding := range r.Bindings {
		status := "INACTIVE"
		if binding.Metadata.IsActive {
			status = "ACTIVE"
		}
		policy := binding.Spec.FailurePolicy
		if policy == "" {
			policy = mcp.PluginFailurePolicyContinue
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s@%s\t%s@%s\t%s\t%s\t%s\n", binding.Metadata.Name,
			binding.Spec.PluginRef.Name, binding.Spec.PluginRef.Version,
			binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version,
			binding.Spec.Phase, policy, status)
	}
	return tw.Flush()
}

func init() {
	pluginCmd.AddCommand(pluginBindingCmd)
	pluginBindingCmd.AddCommand(pluginBindingApplyCmd, pluginBindingGetCmd, pluginBindingValidateCmd, pluginBindingDeleteCmd)
	pluginBindingApplyCmd.Flags().StringP("file", "f", "", "Path to the plugin binding resource file or directory")
	pluginBindingGetCmd.Flags().StringP("output", "o", "table", "Output format (table|yaml|json)")
	pluginBindingValidateCmd.Flags().StringP("file", "f", "", "Path to the plugin binding resource file")
	pluginBindingDeleteCmd.Flags().Bool("hard", false, "Permanently delete the plugin binding")
	pluginBindingDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt for hard delete")
}
