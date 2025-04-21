package main

import (
	"os"

	"github.com/equinix-labs/otel-cli/otelcli"
)

// these will be set by goreleaser & ldflags at build time
var (
	version = ""
	commit  = ""
	date    = ""
)

func main() {
	otelcli.Execute(otelcli.FormatVersion(version, commit, date))
	os.Exit(otelcli.GetExitCode())
}
