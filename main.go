package main

import (
	"os"

	"github.com/equinix-labs/otel-cli/cmd"
)

func main() {
	cmd.Execute()
	os.Exit(cmd.GetExitCode())
}
