package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage ERPBridge API tokens",
}

var (
	tokenName    string
	tokenScopes  []string
	tokenRoles   []string
	tokenExpires string
)

// TokenInfo contains token metadata. The raw token is present only in the
// create response because the server never returns it again.
type TokenInfo struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	Roles     []string   `json:"roles"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type tokenCreateResponse struct {
	TokenInfo
	Token string `json:"token"`
}

var tokenCreateCmd = &cobra.Command{
	Use:   "create --name <name>",
	Short: "Create an API token",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, err := cfg.EffectiveContext()
		if err != nil {
			return err
		}
		if err := ValidateServerURL(ctx.Server, "BRIDGE", cfg.CurrentContext); err != nil {
			return err
		}
		payload := map[string]any{
			cliNameField: tokenName,
			"scopes":     tokenScopes,
			"roles":      tokenRoles,
		}
		if tokenExpires != "" {
			expiresAt, err := parseTokenExpiry(tokenExpires)
			if err != nil {
				return err
			}
			payload["expiresAt"] = expiresAt.Format(time.RFC3339Nano)
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		resp, err := doBridgeRequestWithHeaders(cmd, http.MethodPost, tokenURL(ctx.Server, ""), bytes.NewReader(body), http.Header{cliContentTypeHeader: []string{cliJSONContentType}})
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return bridgeResponseError(resp)
		}
		var result tokenCreateResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return err
		}
		return formatter.Print(&result)
	},
}

var tokenListCmd = &cobra.Command{
	Use:   cliListUse,
	Short: "List API token metadata",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, err := cfg.EffectiveContext()
		if err != nil {
			return err
		}
		if err := ValidateServerURL(ctx.Server, "BRIDGE", cfg.CurrentContext); err != nil {
			return err
		}
		resp, err := doBridgeRequest(cmd, http.MethodGet, tokenURL(ctx.Server, ""))
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return bridgeResponseError(resp)
		}
		var result []TokenInfo
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return err
		}
		return formatter.Print(&TokenListResponse{Items: result})
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke an API token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := cfg.EffectiveContext()
		if err != nil {
			return err
		}
		if err := ValidateServerURL(ctx.Server, "BRIDGE", cfg.CurrentContext); err != nil {
			return err
		}
		resp, err := doBridgeRequest(cmd, http.MethodDelete, tokenURL(ctx.Server, args[0]))
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return bridgeResponseError(resp)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "API token revoked")
		return nil
	},
}

// TokenListResponse wraps token metadata for table and structured output.
type TokenListResponse struct {
	Items []TokenInfo `json:"items"`
}

// RenderTable renders token metadata without any credential value.
func (r *TokenListResponse) RenderTable(w io.Writer) error {
	tw := output.NewTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSCOPES\tROLES\tEXPIRES")
	for _, item := range r.Items {
		expires := "never"
		if item.ExpiresAt != nil {
			expires = item.ExpiresAt.Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Name, strings.Join(item.Scopes, ","), strings.Join(item.Roles, ","), expires)
	}
	return tw.Flush()
}

func tokenURL(base, id string) string {
	base = strings.TrimRight(base, "/") + "/api/auth/tokens"
	if id == "" {
		return base
	}
	return base + "/" + url.PathEscape(id)
}

func parseTokenExpiry(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return time.Time{}, fmt.Errorf("invalid token expiry %q: use RFC3339 or a positive duration", value)
	}
	return time.Now().UTC().Add(duration), nil
}

func init() {
	RootCmd.AddCommand(tokenCmd)
	tokenCmd.AddCommand(tokenCreateCmd, tokenListCmd, tokenRevokeCmd)
	tokenCreateCmd.Flags().StringVar(&tokenName, "name", "", "Token display name")
	tokenCreateCmd.Flags().StringArrayVar(&tokenScopes, "scope", nil, "Token scope (repeatable: mcp, metrics, logs)")
	tokenCreateCmd.Flags().StringArrayVar(&tokenRoles, "role", nil, "Token role (repeatable)")
	tokenCreateCmd.Flags().StringVar(&tokenExpires, "expires", "", "Expiry as RFC3339 or a positive duration")
	_ = tokenCreateCmd.MarkFlagRequired("name")
}
