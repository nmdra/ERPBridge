package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/idp"
	"github.com/nmdra/ERPBridge/internal/web"
	"github.com/spf13/cobra"
)

var (
	webListen  string
	webNoOpen  bool
	webURLOnly bool
	webDev     bool
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the local ERPBridge Console",
	Long: `Start a read-only local web console for monitoring configured ERPBridge contexts.

The console binds to loopback by default, keeps upstream credentials in the CLI,
and stops when this command receives an interrupt or termination signal. It
refreshes the context snapshot while running and retains the last valid snapshot
when a later configuration read fails. Context selection is stored only in the
browser session; it does not change the CLI configuration.`,
	Example: `  bridgectl web
  bridgectl web --no-open
  bridgectl web --url`,
	Args: cobra.NoArgs,
	RunE: runWeb,
}

func runWeb(cmd *cobra.Command, _ []string) error {
	assetOptions := web.AssetOptions{}
	configProvider := func() (*config.Config, error) {
		refreshed, err := config.Load()
		if err != nil {
			return nil, err
		}
		if ctxOverride != "" {
			refreshed.CurrentContext = ctxOverride
		}
		if _, err := refreshed.EffectiveContext(); err != nil {
			return nil, err
		}
		return refreshed, nil
	}
	if webDev {
		assetOptions.DevServerURL = "http://127.0.0.1:5173"
	}
	server, err := web.NewServer(web.Options{
		ListenAddress: webListen,
		Handler: web.NewConsoleHandler(web.HandlerOptions{
			Config:         cfg,
			ConfigProvider: configProvider,
			TokenOverride:  tokenOverride,
			Assets:         web.NewAssetHandler(assetOptions),
			RegistryProvider: func(contextName string) (*idp.Registry, error) {
				return idp.NewRegistryForContext(contextName, RootLog)
			},
		}),
	})
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "ERPBridge Console: %s\n", server.CapabilityURL()); err != nil {
		return err
	}
	if !webNoOpen && !webURLOnly {
		if err := server.OpenBrowser(); err != nil {
			return err
		}
	}

	requestContext := cmd.Context()
	if requestContext == nil {
		requestContext = context.Background()
	}
	requestContext, stop := signal.NotifyContext(requestContext, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve() }()
	select {
	case err := <-serveErr:
		return err
	case <-requestContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	}
}

func init() {
	webCmd.Flags().StringVar(&webListen, "listen", "127.0.0.1:0", "Loopback listen address")
	webCmd.Flags().BoolVar(&webNoOpen, "no-open", false, "Do not open the default browser")
	webCmd.Flags().BoolVar(&webURLOnly, "url", false, "Print the URL without opening the browser")
	webCmd.Flags().BoolVar(&webDev, "dev", false, "Proxy frontend development traffic to Vite on loopback")
	RootCmd.AddCommand(webCmd)
}
