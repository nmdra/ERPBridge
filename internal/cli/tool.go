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
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/nmdra/ERPBridge/internal/idp"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Manage MCP tool resources (V2 Control Plane)",
}

func decodeToolDocuments(data []byte, filePath string) ([]mcp.Tool, error) {
	if strings.HasSuffix(filePath, ".json") {
		var tools []mcp.Tool
		if err := json.Unmarshal(data, &tools); err == nil {
			return tools, nil
		}
		var tool mcp.Tool
		if err := json.Unmarshal(data, &tool); err != nil {
			return nil, fmt.Errorf("unmarshal json (%s): %w", filePath, err)
		}
		return []mcp.Tool{tool}, nil
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var tools []mcp.Tool
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
			var tool mcp.Tool
			if err := yaml.Unmarshal(encoded, &tool); err != nil {
				return nil, fmt.Errorf("unmarshal yaml document (%s): %w", filePath, err)
			}
			tools = append(tools, tool)
		}
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("unmarshal yaml (%s): no tool documents found", filePath)
	}
	return tools, nil
}

var toolApplyCmd = &cobra.Command{
	Use:   "apply -f [file]",
	Short: "Apply a tool schema to the registry (declarative)",
	Example: `  bridgectl tool apply -f list_employees.yaml
  bridgectl tool apply -f schemas/hr/`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			return fmt.Errorf("file path is required")
		}

		info, err := os.Stat(filePath)
		if err != nil {
			return err
		}

		ctx := cfg.ActiveContext()
		if err := ValidateServerURL(ctx.MCPServer, "MCP", cfg.CurrentContext); err != nil {
			return err
		}
		url := ctx.MCPServer + "/apis/erpbridge.io/v1/tools"

		applyFile := func(path string) error {
			if !strings.HasSuffix(path, ".json") && !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
				return nil
			}

			// #nosec G304 -- path is provided via CLI flag or directory traversal
			data, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				return err
			}

			tools, err := decodeToolDocuments(data, path)
			if err != nil {
				return err
			}
			for _, tool := range tools {
				payload, err := json.Marshal(tool)
				if err != nil {
					return fmt.Errorf("marshal tool (%s): %w", path, err)
				}
				resp, err := doBridgeRequestWithHeaders(cmd, http.MethodPost, url, bytes.NewReader(payload), http.Header{"Content-Type": []string{"application/json"}})
				if err != nil {
					return fmt.Errorf("apply failed (%s): %w", path, err)
				}
				body, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr != nil {
					return fmt.Errorf("read apply response (%s): %w", path, readErr)
				}
				if resp.StatusCode >= 400 {
					return fmt.Errorf("server error (%d) for %s: %s", resp.StatusCode, path, string(body))
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "tool %s@%s applied successfully\n", tool.Metadata.Name, tool.Metadata.Version)
			}
			return nil
		}

		if info.IsDir() {
			return filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				return applyFile(path)
			})
		}

		return applyFile(filePath)
	},
}

var toolGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Display one or many tool resources",
	Example: `  bridgectl tool get
  bridgectl tool get list_employees -o yaml`,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		ctx := cfg.ActiveContext()
		if err := ValidateServerURL(ctx.MCPServer, "MCP", cfg.CurrentContext); err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		url := ctx.MCPServer + "/apis/erpbridge.io/v1/tools"

		// #nosec G107 -- URL is dynamically constructed from configured server context
		resp, err := doBridgeRequest(cmd, http.MethodGet, url)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		defer func() { _ = resp.Body.Close() }()

		var tools []*mcp.Tool
		if err := json.NewDecoder(resp.Body).Decode(&tools); err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var completions []string
		for _, t := range tools {
			if strings.HasPrefix(t.Metadata.Name, toComplete) {
				completions = append(completions, t.Metadata.Name)
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cfg.ActiveContext()
		if err := ValidateServerURL(ctx.MCPServer, "MCP", cfg.CurrentContext); err != nil {
			return err
		}
		listURL := ctx.MCPServer + "/apis/erpbridge.io/v1/tools"
		name, version := "", ""
		if len(args) > 0 {
			name, version = mcp.ParseToolIdentifier(args[0])
			query := url.Values{cliNameField: []string{name}}
			if version != "" {
				query.Set("version", version)
			}
			listURL += "?" + query.Encode()
		}

		// #nosec G107 -- URL is dynamically constructed from configured server context
		resp, err := doBridgeRequest(cmd, http.MethodGet, listURL)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		var tools []*mcp.Tool
		if err := json.NewDecoder(resp.Body).Decode(&tools); err != nil {
			return err
		}

		outputFormat, _ := cmd.Flags().GetString("output")

		if len(args) > 0 {
			var target *mcp.Tool
			for _, t := range tools {
				if t.Metadata.Name == name && (version == "" || t.Metadata.Version == version) {
					target = t
					break
				}
			}

			if target == nil {
				return fmt.Errorf("tool %s not found", args[0])
			}

			switch outputFormat {
			case string(output.FormatYAML):
				y, _ := yaml.Marshal(target)
				fmt.Println(string(y))
				return nil
			case string(output.FormatJSON):
				j, _ := json.MarshalIndent(target, "", "  ")
				fmt.Println(string(j))
				return nil
			}
			tools = []*mcp.Tool{target}
		}

		res := &ToolListResponse{Tools: tools}
		return formatter.Print(res)
	},
}

// ToolListResponse wraps a slice of MCP tools for table formatting.
type ToolListResponse struct {
	Tools []*mcp.Tool `json:"tools"`
}

// RenderTable implements the output.TableRenderer interface.
func (r *ToolListResponse) RenderTable(w io.Writer) error {
	tw := output.NewTabWriter(w)
	_, _ = fmt.Fprintln(tw, "NAME\tMODULE\tVERSION\tSTATUS")
	for _, t := range r.Tools {
		status := t.Metadata.Status
		if !t.Metadata.IsActive {
			status = "HIDDEN"
		} else if status == "" {
			status = "READY"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.Metadata.Name, t.Metadata.Module, t.Metadata.Version, status)
	}
	return tw.Flush()
}

var toolDescribeCmd = &cobra.Command{
	Use:     "describe [name]",
	Short:   "Describe a tool in human-readable detail",
	Long:    `Fetch a tool definition from the server and render its parameters, execution method, endpoint, and description.`,
	Example: `  bridgectl tool describe get_customer_balance`,
	Args:    cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return toolGetCmd.ValidArgsFunction(cmd, args, toComplete)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cfg.ActiveContext()
		if err := ValidateServerURL(ctx.MCPServer, "MCP", cfg.CurrentContext); err != nil {
			return err
		}

		name, version := mcp.ParseToolIdentifier(args[0])
		listURL := fmt.Sprintf("%s/apis/erpbridge.io/v1/tools", ctx.MCPServer)
		query := url.Values{cliNameField: []string{name}}
		if version != "" {
			query.Set("version", version)
		}
		listURL += "?" + query.Encode()

		// #nosec G107 -- URL is dynamically constructed from configured server context
		resp, err := doBridgeRequest(cmd, http.MethodGet, listURL)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		var tools []*mcp.Tool
		if err := json.NewDecoder(resp.Body).Decode(&tools); err != nil {
			return err
		}

		var target *mcp.Tool
		for _, t := range tools {
			if t.Metadata.Name == name && (version == "" || t.Metadata.Version == version) {
				target = t
				break
			}
		}

		if target == nil {
			return fmt.Errorf("tool %s not found", args[0])
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "Name:        %s\n", target.Metadata.Name)
		_, _ = fmt.Fprintf(out, "Version:     %s\n", target.Metadata.Version)
		_, _ = fmt.Fprintf(out, "Module:      %s\n", target.Metadata.Module)
		_, _ = fmt.Fprintf(out, "Description: %s\n", target.Spec.Description.Short)
		_, _ = fmt.Fprintf(out, "\nInput Parameters:\n")
		for propName, prop := range target.Spec.InputSchema.Properties {
			required := ""
			if slices.Contains(target.Spec.InputSchema.Required, propName) {
				required = "*"
			}
			_, _ = fmt.Fprintf(out, "  %-12s [%s]%s %s\n", propName, prop.Type, required, prop.Description)
		}

		_, _ = fmt.Fprintf(out, "\nExecution:\n")
		_, _ = fmt.Fprintf(out, "  Method:    %s\n", target.Spec.Execution.Method)
		_, _ = fmt.Fprintf(out, "  Endpoint:  %s\n", target.Spec.Execution.Endpoint)

		return nil
	},
}

var toolValidateCmd = &cobra.Command{
	Use:   "validate -f [file]",
	Short: "Locally validate a tool schema",
	RunE: func(cmd *cobra.Command, _ []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		// #nosec G304 -- filePath is supplied via CLI file flag
		data, err := os.ReadFile(filepath.Clean(filePath))
		if err != nil {
			return err
		}

		var tool mcp.Tool
		if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
			if err := yaml.Unmarshal(data, &tool); err != nil {
				return fmt.Errorf("validation failed: parse YAML: %w", err)
			}
		} else {
			if err := json.Unmarshal(data, &tool); err != nil {
				return fmt.Errorf("validation failed: parse JSON: %w", err)
			}
		}

		if tool.Metadata.Name == "" {
			return fmt.Errorf("validation failed: metadata.name is missing")
		}
		if tool.Metadata.Version == "" {
			return fmt.Errorf("validation failed: metadata.version is missing")
		}

		fmt.Printf("✓ tool %s@%s is locally valid\n", tool.Metadata.Name, tool.Metadata.Version)
		return nil
	},
}

var toolGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Auto-generate an MCP tool schema from a registered API or OpenAPI spec",
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiName, _ := cmd.Flags().GetString("api")
		openapiURL, _ := cmd.Flags().GetString("openapi")

		reg, err := idp.NewRegistry("", RootLog)
		if err != nil {
			return err
		}

		api, ok := reg.Get(apiName)
		if !ok {
			return NewError(CodeNotFound, "API_NOT_FOUND",
				fmt.Sprintf("API %q not found in local registry", apiName),
				"A registered API is required to provide base URL and authentication templates. Run 'bridgectl api register' first.")
		}

		gen := idp.NewGenerator("", RootLog)

		if openapiURL != "" {
			tools, err := gen.GenerateFromOpenAPI(cmd.Context(), api, openapiURL)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "generated %d tools from OpenAPI spec\n", len(tools))

			outputFormat, _ := cmd.Flags().GetString("output")
			var out []byte
			if outputFormat == "yaml" {
				out, _ = yaml.Marshal(tools)
			} else {
				out, _ = json.MarshalIndent(tools, "", "  ")
			}
			fmt.Println(string(out))
			return nil
		}

		tool, err := gen.Generate(api)
		if err != nil {
			return err
		}

		outputFormat, _ := cmd.Flags().GetString("output")
		var out []byte
		if outputFormat == "yaml" {
			out, _ = yaml.Marshal(tool)
		} else {
			out, _ = json.MarshalIndent(tool, "", "  ")
		}
		fmt.Println(string(out))
		return nil
	},
}

var toolDeleteCmd = &cobra.Command{
	Use:   "delete [name] [version]",
	Short: "Remove a tool from the registry",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("missing required arguments.\nUsage: %s\nExample: bridgectl tool delete list_invoices v1.0.0", cmd.UseLine())
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		version := args[1]
		hard, _ := cmd.Flags().GetBool("hard")
		yes, _ := cmd.Flags().GetBool("yes")

		if hard && !yes {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "WARNING: This will permanently delete the tool '%s@%s' from the database. Are you sure? (y/N): ", name, version)
			var response string
			_, _ = fmt.Scanln(&response)
			if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Operation aborted.")
				return nil
			}
		}

		ctx := cfg.ActiveContext()
		if err := ValidateServerURL(ctx.MCPServer, "MCP", cfg.CurrentContext); err != nil {
			return err
		}
		url := fmt.Sprintf("%s/apis/erpbridge.io/v1/tools?name=%s&version=%s", ctx.MCPServer, name, version)
		if hard {
			url += "&hard=true"
		}

		resp, err := doBridgeRequest(cmd, http.MethodDelete, url)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("delete failed (%d): %s", resp.StatusCode, string(body))
		}

		if hard {
			fmt.Printf("✓ tool %s@%s has been permanently deleted from the registry.\n", name, version)
		} else {
			fmt.Printf("✓ tool %s@%s is now in an inactive state and hidden from MCP clients.\n", name, version)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(toolCmd)
	toolCmd.AddCommand(toolApplyCmd)
	toolCmd.AddCommand(toolGetCmd)
	toolCmd.AddCommand(toolDescribeCmd)
	toolCmd.AddCommand(toolValidateCmd)
	toolCmd.AddCommand(toolGenerateCmd)
	toolCmd.AddCommand(toolDeleteCmd)

	toolApplyCmd.Flags().StringP("file", "f", "", "Path to the tool schema file")
	toolGetCmd.Flags().StringP("output", "o", "table", "Output format (table|yaml|json)")
	toolValidateCmd.Flags().StringP("file", "f", "", "Path to the tool schema file")
	toolGenerateCmd.Flags().String("api", "", "Name of the registered API to generate from")
	toolGenerateCmd.Flags().String("openapi", "", "URL or path to an OpenAPI spec")
	_ = toolGenerateCmd.MarkFlagRequired("api")

	toolDeleteCmd.Flags().Bool("hard", false, "Permanently delete the tool from the database")
	toolDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt for hard delete")
}
