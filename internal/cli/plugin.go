package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage external plugin resources",
}

func decodePluginDocuments(data []byte, filePath string) ([]mcp.Plugin, error) {
	return decodeResourceDocuments[mcp.Plugin](data, filePath)
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON documents are not allowed")
		}
		return err
	}
	return nil
}

func decodeResourceDocuments[T any](data []byte, filePath string) ([]T, error) {
	if strings.HasSuffix(strings.ToLower(filePath), ".json") {
		var resources []T
		if err := decodeStrictJSON(data, &resources); err == nil {
			if len(resources) == 0 {
				return nil, fmt.Errorf("unmarshal json (%s): no resource documents found", filePath)
			}
			return resources, nil
		}
		var resource T
		if err := decodeStrictJSON(data, &resource); err != nil {
			return nil, fmt.Errorf("unmarshal json (%s): %w", filePath, err)
		}
		return []T{resource}, nil
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var resources []T
	for {
		var document any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("unmarshal yaml (%s): %w", filePath, err)
		}
		if document == nil {
			continue
		}
		items := []any{document}
		if sequence, ok := document.([]any); ok {
			items = sequence
		}
		for _, item := range items {
			encoded, err := yaml.Marshal(item)
			if err != nil {
				return nil, fmt.Errorf("marshal yaml document (%s): %w", filePath, err)
			}
			var resource T
			if err := yaml.UnmarshalWithOptions(encoded, &resource, yaml.DisallowUnknownField()); err != nil {
				return nil, fmt.Errorf("unmarshal yaml document (%s): %w", filePath, err)
			}
			resources = append(resources, resource)
		}
	}
	if len(resources) == 0 {
		return nil, fmt.Errorf("unmarshal yaml (%s): no resource documents found", filePath)
	}
	return resources, nil
}

func applyResourceFiles(filePath string, apply func(string, []byte) error) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	applyFile := func(path string) error {
		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".json") && !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			return nil
		}
		// #nosec G304 -- path is supplied by the CLI or directory traversal.
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		return apply(path, data)
	}
	if !info.IsDir() {
		return applyFile(filePath)
	}
	return filepath.Walk(filePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		return applyFile(path)
	})
}

func pluginServerBase() (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("CLI configuration is not initialized")
	}
	ctx, err := cfg.EffectiveContext()
	if err != nil {
		return "", err
	}
	if err := ValidateServerURL(ctx.MCPServer, "MCP", cfg.CurrentContext); err != nil {
		return "", err
	}
	return ctx.MCPServer, nil
}

var pluginApplyCmd = &cobra.Command{
	Use:   cliApplyFileUse,
	Short: "Apply one or more external plugin resources",
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
			plugins, err := decodePluginDocuments(data, path)
			if err != nil {
				return err
			}
			for _, plugin := range plugins {
				if err := plugin.Validate(); err != nil {
					return fmt.Errorf("invalid plugin (%s): %w", path, err)
				}
				payload, err := json.Marshal(plugin)
				if err != nil {
					return fmt.Errorf("marshal plugin (%s): %w", path, err)
				}
				resp, err := doBridgeRequestWithHeaders(cmd, http.MethodPost, baseURL+"/apis/erpbridge.io/v1/plugins", bytes.NewReader(payload), http.Header{cliContentTypeHeader: []string{cliJSONContentType}})
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
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "plugin %s@%s applied successfully\n", plugin.Metadata.Name, plugin.Metadata.Version)
			}
			return nil
		})
	},
}

var pluginGetCmd = &cobra.Command{
	Use:   "get [name@version]",
	Short: "Display one or more external plugin resources",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseURL, err := pluginServerBase()
		if err != nil {
			return err
		}
		listURL := baseURL + "/apis/erpbridge.io/v1/plugins"
		if len(args) == 1 {
			name, version := mcp.ParseToolIdentifier(args[0])
			if name == "" || version == "" {
				return fmt.Errorf("plugin identity must use name@version")
			}
			query := url.Values{cliNameField: []string{name}}
			query.Set("version", version)
			listURL += "?" + query.Encode()
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
		var plugins []*mcp.Plugin
		if err := json.NewDecoder(resp.Body).Decode(&plugins); err != nil {
			return err
		}
		if len(args) == 1 {
			name, version := mcp.ParseToolIdentifier(args[0])
			var target *mcp.Plugin
			for _, plugin := range plugins {
				if plugin.Metadata.Name == name && (version == "" || plugin.Metadata.Version == version) {
					target = plugin
					break
				}
			}
			if target == nil {
				return fmt.Errorf("plugin %s not found", args[0])
			}
			return renderPlugin(cmd.OutOrStdout(), cmd, target, nil)
		}
		return renderPlugin(cmd.OutOrStdout(), cmd, nil, plugins)
	},
}

var pluginValidateCmd = &cobra.Command{
	Use:   cliValidateFileUse,
	Short: "Locally validate plugin resource definitions",
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
		plugins, err := decodePluginDocuments(data, filePath)
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
		for _, plugin := range plugins {
			if err := plugin.Validate(); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ plugin %s@%s is locally valid\n", plugin.Metadata.Name, plugin.Metadata.Version)
		}
		return nil
	},
}

var pluginDeleteCmd = &cobra.Command{
	Use:   "delete [name@version]",
	Short: "Remove an external plugin resource",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, version := mcp.ParseToolIdentifier(args[0])
		if name == "" || version == "" {
			return fmt.Errorf("plugin identity must use name@version")
		}
		hard, _ := cmd.Flags().GetBool("hard")
		yes, _ := cmd.Flags().GetBool("yes")
		if hard && !yes {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "WARNING: This will permanently delete plugin '%s@%s'. Are you sure? (y/N): ", name, version)
			var response string
			_, _ = fmt.Fscan(cmd.InOrStdin(), &response)
			if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Operation aborted.")
				return nil
			}
		}
		baseURL, err := pluginServerBase()
		if err != nil {
			return err
		}
		query := url.Values{cliNameField: []string{name}, "version": []string{version}}
		if hard {
			query.Set("hard", "true")
		}
		resp, err := doBridgeRequest(cmd, http.MethodDelete, baseURL+"/apis/erpbridge.io/v1/plugins?"+query.Encode())
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("delete failed (%d): %s", resp.StatusCode, string(body))
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "plugin %s@%s deleted successfully\n", name, version)
		return nil
	},
}

func renderPlugin(out io.Writer, cmd *cobra.Command, target *mcp.Plugin, plugins []*mcp.Plugin) error {
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
			return (&PluginListResponse{Plugins: []*mcp.Plugin{target}}).RenderTable(out)
		}
	}
	response := &PluginListResponse{Plugins: plugins}
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

// PluginListResponse wraps plugin resources for output formatting.
type PluginListResponse struct {
	Plugins []*mcp.Plugin `json:"plugins" yaml:"plugins"`
}

// RenderTable renders plugin identities and lifecycle state.
func (r *PluginListResponse) RenderTable(w io.Writer) error {
	tw := output.NewTabWriter(w)
	_, _ = fmt.Fprintln(tw, "NAME\tVERSION\tTYPE\tENDPOINT\tSTATUS")
	for _, plugin := range r.Plugins {
		status := "INACTIVE"
		if plugin.Metadata.IsActive {
			status = "ACTIVE"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", plugin.Metadata.Name, plugin.Metadata.Version, plugin.Metadata.Type, plugin.Spec.Endpoint, status)
	}
	return tw.Flush()
}

func init() {
	RootCmd.AddCommand(pluginCmd)
	pluginCmd.AddCommand(pluginApplyCmd, pluginGetCmd, pluginValidateCmd, pluginDeleteCmd)
	pluginApplyCmd.Flags().StringP("file", "f", "", "Path to the plugin resource file or directory")
	pluginGetCmd.Flags().StringP("output", "o", "table", "Output format (table|yaml|json)")
	pluginValidateCmd.Flags().StringP("file", "f", "", "Path to the plugin resource file")
	pluginDeleteCmd.Flags().Bool("hard", false, "Permanently delete the plugin")
	pluginDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt for hard delete")
}
