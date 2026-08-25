package web

import (
	"fmt"
	"os/exec"
	"runtime"
)

func defaultBrowserOpener() BrowserOpener {
	return func(url string) error {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("open", url) //nolint:gosec // URL is passed as an argument, never through a shell
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // URL is passed as an argument, never through a shell
		default:
			command = exec.Command("xdg-open", url) //nolint:gosec // URL is passed as an argument, never through a shell
		}
		if err := command.Start(); err != nil {
			return fmt.Errorf("open browser: %w", err)
		}
		return nil
	}
}
