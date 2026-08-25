package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/credentials"
	"github.com/nmdra/ERPBridge/internal/idp"
	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Manage ERP API endpoints",
	Long: `The api command allows you to manage the inventory of ERP API endpoints. 
You can register new endpoints, list existing ones, and test connectivity 
and authentication to ensure the middleware can correctly proxy requests to the legacy ERP.`,
}

var apiRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a new ERP API endpoint",
	Long: `Register an ERP API endpoint in the local registry. This step is usually 
the first part of the workflow, defining how the middleware connects to the ERP. 
Once registered, you can generate an MCP tool schema from this API definition.`,
	Example: `  bridgectl api register \
    --name get-invoices \
    --url "http://erp.local/api/v1/invoices" \
    --method GET \
    --module finance \
    --description "Fetch all invoices" \
    --auth-type api-key \
    --credential-ref ERP_API_KEY`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := idp.NewRegistry("", RootLog)
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		url, _ := cmd.Flags().GetString("url")
		method, _ := cmd.Flags().GetString("method")
		module, _ := cmd.Flags().GetString("module")
		desc, _ := cmd.Flags().GetString("description")
		authType, _ := cmd.Flags().GetString("auth-type")
		authHeader, _ := cmd.Flags().GetString("auth-header")
		credentialRef, _ := cmd.Flags().GetString("credential-ref")

		api := &idp.API{
			Name:          name,
			URL:           url,
			Method:        method,
			Module:        module,
			Description:   desc,
			AuthType:      authType,
			AuthHeader:    authHeader,
			CredentialRef: credentialRef,
		}

		if err := reg.Register(api); err != nil {
			return err
		}

		// Wrap in a response struct for formatting
		resp := &APIRegistrationResponse{API: *api}
		return formatter.Print(resp)
	},
}

// APIRegistrationResponse wraps an idp.API for table rendering after registration.
type APIRegistrationResponse struct {
	API idp.API `json:"api" yaml:"api"`
}

// RenderTable implements the output.TableRenderer interface.
func (r *APIRegistrationResponse) RenderTable(w io.Writer) error {
	_, _ = fmt.Fprintf(w, "Registered API  %s\n", r.API.Name)
	_, _ = fmt.Fprintf(w, "ID              %s\n", r.API.ID)
	_, _ = fmt.Fprintf(w, "Module          %s\n", r.API.Module)
	_, _ = fmt.Fprintf(w, "Method          %s\n", r.API.Method)
	_, _ = fmt.Fprintf(w, "URL             %s\n", r.API.URL)
	_, _ = fmt.Fprintf(w, "Status          %s\n", r.API.Status)
	_, err := fmt.Fprintln(w, "\nNext: run \"bridgectl tool generate --api "+r.API.Name+"\" to create an MCP tool schema.")
	return err
}

var apiListCmd = &cobra.Command{
	Use:   cliListUse,
	Short: "List all registered APIs",
	Long:  `Display a table of all ERP API endpoints currently registered in the local registry.`,
	Example: `  bridgectl api list
  bridgectl api list -o json`,
	RunE: func(_ *cobra.Command, _ []string) error {
		reg, err := idp.NewRegistry("", RootLog)
		if err != nil {
			return err
		}

		apis := reg.List()
		resp := &APIListResponse{Items: apis}
		return formatter.Print(resp)
	},
}

// APIListResponse wraps a list of idp.API for table rendering.
type APIListResponse struct {
	Items []idp.API `json:"items" yaml:"items"`
}

// RenderTable implements the output.TableRenderer interface.
func (r *APIListResponse) RenderTable(w io.Writer) error {
	tw := output.NewTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tMODULE\tMETHOD\tSTATUS")
	for _, api := range r.Items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", api.ID, api.Name, api.Module, api.Method, api.Status)
	}
	return tw.Flush()
}

var apiSetCredentialRefCmd = &cobra.Command{
	Use:     "set-credential-ref [name]",
	Short:   "Set the environment credential reference for an API",
	Example: `  bridgectl api set-credential-ref get-invoices --credential-ref ERP_API_KEY`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := idp.NewRegistry("", RootLog)
		if err != nil {
			return err
		}
		ref, err := cmd.Flags().GetString("credential-ref")
		if err != nil {
			return err
		}
		if err := reg.SetCredentialRef(args[0], ref); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Credential reference set for API %s\n", args[0])
		return nil
	},
}

var apiScrubCredentialsCmd = &cobra.Command{
	Use:   "scrub-credentials",
	Short: "Remove legacy plaintext credentials from the local registry",
	Long:  "Atomically remove legacy credential fields. This operation does not create a backup and requires explicit confirmation.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		yes, err := cmd.Flags().GetBool("yes")
		if err != nil {
			return err
		}
		if !yes {
			return fmt.Errorf("scrub-credentials is destructive; repeat with --yes")
		}
		reg, err := idp.NewRegistry("", RootLog)
		if err != nil {
			return err
		}
		if err := reg.ScrubCredentials(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Legacy credentials scrubbed.")
		return nil
	},
}

var apiTestCmd = &cobra.Command{
	Use:   "test [name]",
	Short: "Send a test request to a registered API",
	Long: `Verify connectivity to a registered ERP API endpoint. 
This command performs a real HTTP request using the configured method, 
URL, and authentication headers, and displays the response status and latency.`,
	Example: `  bridgectl api test get-invoices`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := idp.NewRegistry("", RootLog)
		if err != nil {
			return err
		}

		if reg.HasLegacyCredentials() {
			return idp.ErrLegacyCredentials
		}

		name := args[0]
		api, ok := reg.Get(name)
		if !ok {
			return NewError(CodeNotFound, "API_NOT_FOUND",
				fmt.Sprintf("API %s not found in local registry", name),
				"Run 'bridgectl api list' to see available APIs.")
		}

		credential, err := resolveAPICredential(api)
		if err != nil {
			return err
		}

		client := connector.NewClient(RootLog)
		ep := connector.EndpointConfig{
			Method:  api.Method,
			Path:    api.URL, // In this case, URL is absolute as per register
			BaseURL: "",      // Empty because Path is the full URL
			Auth: connector.AuthConfig{
				Type: api.AuthType,
				Key:  credential,
			},
		}

		start := time.Now()
		resp, err := client.Call(cmd.Context(), ep, nil, nil)
		latency := time.Since(start)

		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		var body any
		_ = json.NewDecoder(resp.Body).Decode(&body)

		testResp := &APITestResponse{
			API:       api,
			Status:    resp.Status,
			Code:      resp.StatusCode,
			Latency:   latency,
			Response:  body,
			IsSuccess: resp.StatusCode >= 200 && resp.StatusCode < 300,
		}

		return formatter.Print(testResp)
	},
}

// APITestResponse contains the results of an API connectivity test.
type APITestResponse struct {
	API       idp.API       `json:"api"`
	Status    string        `json:"status"`
	Code      int           `json:"code"`
	Latency   time.Duration `json:"latency"`
	Response  any           `json:"response"`
	IsSuccess bool          `json:"isSuccess"`
}

// RenderTable implements the output.TableRenderer interface.
func (r *APITestResponse) RenderTable(w io.Writer) error {
	_, _ = fmt.Fprintf(w, "Testing  %s\n", r.API.Name)
	_, _ = fmt.Fprintf(w, "URL      %s %s\n", r.API.Method, r.API.URL)
	_, _ = fmt.Fprintf(w, "Auth     %s (%s)\n\n", r.API.AuthType, r.API.AuthHeader)
	_, _ = fmt.Fprintf(w, "Status   %s\n", r.Status)
	_, _ = fmt.Fprintf(w, "Latency  %v\n\n", r.Latency)

	if r.IsSuccess {
		_, _ = fmt.Fprintln(w, "✓ API is reachable and auth is valid.")
	} else {
		_, _ = fmt.Fprintln(w, "✗ API test failed")
	}
	return nil
}

func resolveAPICredential(api idp.API) (string, error) {
	if api.AuthType == "" {
		return "", nil
	}
	if api.CredentialRef == "" {
		return "", fmt.Errorf("API %q requires a credential reference before testing", api.Name)
	}
	return credentials.Resolve(api.CredentialRef)
}

func init() {
	RootCmd.AddCommand(apiCmd)
	apiCmd.AddCommand(apiRegisterCmd)
	apiCmd.AddCommand(apiListCmd)
	apiCmd.AddCommand(apiSetCredentialRefCmd)
	apiCmd.AddCommand(apiScrubCredentialsCmd)
	apiCmd.AddCommand(apiTestCmd)

	apiRegisterCmd.Flags().String("name", "", "Unique name for this API")
	apiRegisterCmd.Flags().String("url", "", "Full URL of the ERP endpoint")
	apiRegisterCmd.Flags().String("method", "GET", "HTTP method")
	apiRegisterCmd.Flags().String("module", "", "ERP module")
	apiRegisterCmd.Flags().String("description", "", "Human-readable description")
	apiRegisterCmd.Flags().String("auth-type", "api-key", "Auth type")
	apiRegisterCmd.Flags().String("auth-header", "X-API-Key", "Auth header")
	apiRegisterCmd.Flags().String("credential-ref", "", "Environment variable containing the auth credential")
	apiSetCredentialRefCmd.Flags().String("credential-ref", "", "Environment variable containing the auth credential")
	apiScrubCredentialsCmd.Flags().Bool("yes", false, "Confirm destructive credential removal")
	_ = apiSetCredentialRefCmd.MarkFlagRequired("credential-ref")

	_ = apiRegisterCmd.MarkFlagRequired("name")
	_ = apiRegisterCmd.MarkFlagRequired("url")
	_ = apiRegisterCmd.MarkFlagRequired("module")
	_ = apiRegisterCmd.MarkFlagRequired("description")
}
