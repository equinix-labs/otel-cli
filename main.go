package main

import (
	"os"

	"github.com/packethost/otel-cli/cmd"
)

func main() {
	cmd.Execute()
	os.Exit(cmd.GetExitCode())
}
