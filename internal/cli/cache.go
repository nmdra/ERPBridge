// Package cli implements the command line interface commands for bridgectl.
package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/nmdra/ERPBridge/internal/output"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage tool cache",
	Long: `The cache command provides tools to monitor and manage the middleware's 
	backend-independent caching system. You can view real-time statistics
and manually flush entries by tool, module, or for the entire system.`,
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache key counts and memory usage",
	Long: `Display high-level statistics for the tool cache, 
including total key counts and backend memory usage when available.`,
	Example: `  bridgectl cache stats`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, ok := cfg.Contexts[cfg.CurrentContext]
		if !ok {
			return NewError(CodePrecondFail, "NO_CONTEXT",
				"no context selected",
				"Use 'bridgectl context set' to select an active environment.")
		}

		resp, err := doBridgeRequest(cmd, http.MethodGet, ctx.Server+"/api/cache/stats")
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return NewError(CodeGeneralErr, "SERVER_ERROR",
				fmt.Sprintf("server returned error: %s", resp.Status),
				"Verify the middleware server is running and reachable.")
		}

		var result any
		return formatter.Print(output.NewRawResponse(resp.Body, &result))
	},
}

var (
	flushModule string
	flushAll    bool
)

var cacheFlushCmd = &cobra.Command{
	Use:   "flush [tool]",
	Short: "Delete cache entries",
	Long: `Manually invalidate cache entries stored by the configured cache backend.
You can target a specific tool by name, an entire module using the --module flag, 
or clear the entire cache with --all.`,
	Example: `  bridgectl cache flush finance.get-invoices
  bridgectl cache flush --module hr
  bridgectl cache flush --all`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, ok := cfg.Contexts[cfg.CurrentContext]
		if !ok {
			return NewError(CodePrecondFail, "NO_CONTEXT",
				"no context selected",
				"Use 'bridgectl context set' to select an active environment.")
		}

		u, _ := url.Parse(ctx.Server + "/api/cache/flush")
		q := u.Query()
		if len(args) > 0 {
			q.Set("tool", args[0])
		}
		if flushModule != "" {
			q.Set("module", flushModule)
		}
		if flushAll {
			q.Set("all", "true")
		}
		u.RawQuery = q.Encode()

		resp, err := doBridgeRequest(cmd, http.MethodGet, u.String())
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		var result FlushResponse
		return formatter.Print(output.NewRawResponse(resp.Body, &result))
	},
}

// FlushResponse contains the results of a cache flush operation.
type FlushResponse struct {
	// Deleted is the number of cache entries removed.
	Deleted int `json:"deleted"`
	// Status indicates the outcome of the flush request.
	Status string `json:"status"`
}

// RenderTable implements the output.TableRenderer interface.
func (r *FlushResponse) RenderTable(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Deleted %d cache entries.\n", r.Deleted)
	return err
}

func init() {
	RootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cacheFlushCmd)

	cacheFlushCmd.Flags().StringVarP(&flushModule, "module", "m", "", "Flush entire module")
	cacheFlushCmd.Flags().BoolVarP(&flushAll, "all", "a", false, "Flush everything")
}
