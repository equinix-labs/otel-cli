package main

import (
	"os"

	otelcmd "github.com/equinix-labs/otel-cli/cmd"
	"github.com/equinix-labs/otel-cli/otelcli"
)

// these will be set by goreleaser & ldflags at build time
var (
	version = ""
	commit  = ""
	date    = ""
)

var ExitCode int

func main() {
	otelcmd.Execute(otelcmd.FormatVersion(version, commit, date))
	os.Exit(otelcli.GetExitCode())
}
