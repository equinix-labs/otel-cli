package main

import (
	"os"

	otelcli "github.com/equinix-labs/otel-cli/cmd"
)

func main() {
	otelcli.Execute()
	os.Exit(otelcli.GetExitCode())
}
