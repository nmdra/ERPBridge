// Package main is the entry point for the bridgectl CLI.
package main

import (
	"github.com/nmdra/ERPBridge/internal/cli"
)

var version = "dev"

func main() {
	cli.Version = version
	cli.RootCmd.Version = version
	cli.Execute()
}
