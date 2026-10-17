package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReleaseVersionMatchesMCPIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping release binary build in short mode")
	}
	const expectedVersion = "0.6.0-alpha.1"
	binary := filepath.Join(t.TempDir(), "bridgectl")
	buildContext, stopBuild := context.WithTimeout(t.Context(), 2*time.Minute)
	defer stopBuild()
	// #nosec G204 -- fixed Go build arguments and a test-owned output path.
	build := exec.CommandContext(buildContext, "go", "build", "-ldflags=-X main.version="+expectedVersion, "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build release CLI: %v\n%s", err, output)
	}

	// Isolate runtime configuration without moving the builder's module cache
	// into a fixture HOME or inheriting the developer's bridge credentials.
	environment := []string{"HOME=" + t.TempDir()}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "HOME" && !strings.HasPrefix(key, "BRIDGE_") && key != "BRIDGECTL_MCP_CHILD_SNAPSHOT" {
			environment = append(environment, entry)
		}
	}
	// #nosec G204 -- run only the release CLI compiled by this test.
	versionCommand := exec.CommandContext(t.Context(), binary, "--version")
	versionCommand.Env = environment
	output, err := versionCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("read CLI version: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "bridgectl version "+expectedVersion {
		t.Fatalf("CLI version = %q, want %q", got, expectedVersion)
	}

	callContext, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	// #nosec G204 -- run the same test-built binary with fixed stdio arguments.
	command := exec.CommandContext(callContext, binary, "mcp", "--stdio")
	command.Env = environment
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		cancel()
		_ = command.Wait()
	}()
	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"release-test","version":"1"}}}`
	if _, err := stdin.Write([]byte(initialize + "\n")); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			ServerInfo struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.NewDecoder(stdout).Decode(&response); err != nil {
		t.Fatalf("decode MCP initialize: %v", err)
	}
	if got := response.Result.ServerInfo; got.Name != "bridgectl" || got.Version != expectedVersion {
		t.Fatalf("MCP server identity = %+v, want bridgectl %s", got, expectedVersion)
	}
}
