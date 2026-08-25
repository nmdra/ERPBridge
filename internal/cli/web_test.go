package cli

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWebCommandFlags(t *testing.T) {
	for _, name := range []string{"no-open", "url", "listen", "dev"} {
		if webCmd.Flags().Lookup(name) == nil {
			t.Fatalf("web command is missing --%s", name)
		}
	}
}

func TestWebCommandStartsLoopbackServerAndStops(t *testing.T) {
	oldListen, oldNoOpen, oldURL := webListen, webNoOpen, webURLOnly
	defer func() {
		webListen, webNoOpen, webURLOnly = oldListen, oldNoOpen, oldURL
	}()
	webListen = "127.0.0.1:0"
	webNoOpen = true
	webURLOnly = false

	ctx, cancel := context.WithCancel(context.Background())
	command := webCmd
	command.SetContext(ctx)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	result := make(chan error, 1)
	go func() { result <- runWeb(command, nil) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	require.NoError(t, <-result)
}

func TestWebCommandRejectsNonLoopbackListener(t *testing.T) {
	oldListen, oldNoOpen := webListen, webNoOpen
	defer func() {
		webListen, webNoOpen = oldListen, oldNoOpen
	}()
	webListen = ":0"
	webNoOpen = true

	err := runWeb(webCmd, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loopback")
}

func TestWebCommandPrintsCapabilityURL(t *testing.T) {
	oldListen, oldNoOpen, oldURL := webListen, webNoOpen, webURLOnly
	defer func() {
		webListen, webNoOpen, webURLOnly = oldListen, oldNoOpen, oldURL
	}()
	webListen = "127.0.0.1:0"
	webNoOpen = true
	webURLOnly = false

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output strings.Builder
	command := webCmd
	command.SetContext(ctx)
	command.SetOut(&output)
	command.SetErr(io.Discard)

	result := make(chan error, 1)
	go func() { result <- runWeb(command, nil) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	require.NoError(t, <-result)
	require.Contains(t, output.String(), "#cap=")
}
