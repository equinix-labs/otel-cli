package cmd

import (
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run an embedded OTLP server",
	Long:  "Run otel-cli as an OTLP server. See subcommands.",
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
