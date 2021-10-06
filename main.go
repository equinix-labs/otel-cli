package main

import (
	"os"

	"github.com/equinix-labs/otel-cli/otelcli"
)

func main() {
	otelcli.Execute()
	os.Exit(otelcli.GetExitCode())
}
