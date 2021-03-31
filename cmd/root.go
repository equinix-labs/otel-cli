package cmd

import (
	"github.com/spf13/cobra"
)

var serviceName, spanName, spanKind, traceparentCarrierFile string
var spanAttrs map[string]string
var traceparentIgnoreEnv, traceparentPrint, traceparentPrintExport bool
var traceparentCarrierRequired bool
var exitCode int

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "otel-cli",
	Short: "CLI for creating and sending OpenTelemetry spans and events.",
	Long:  `A command-line interface for generating OpenTelemetry data on the command line.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	spanAttrs = make(map[string]string)
	cobra.EnableCommandSorting = false
	rootCmd.Flags().SortFlags = false

	// TODO: put in global flags for the otel endpoint and stuff like that here
	rootCmd.PersistentFlags().StringVarP(&serviceName, "service", "n", "otel-cli", "set the name of the application sent on the traces")

	// all commands and subcommands accept attributes, some might ignore
	// e.g. `--attrs "foo=bar,baz=inga"`
	rootCmd.PersistentFlags().StringToStringVarP(&spanAttrs, "attrs", "a", map[string]string{}, "a comma-separated list of key=value attributes")

	rootCmd.PersistentFlags().StringVarP(&spanKind, "kind", "k", "client", "set the trace kind, e.g. internal, server, client, producer, consumer")

	rootCmd.PersistentFlags().StringVar(&traceparentCarrierFile, "tp-carrier", "", "a file for reading and WRITING traceparent across invocations")
	rootCmd.PersistentFlags().BoolVar(&traceparentCarrierRequired, "tp-carrier-required", false, "when set to true, fail and log when the carrier file doesn't already exist or TRACEPARENT isn't in the environment")
	rootCmd.PersistentFlags().BoolVar(&traceparentIgnoreEnv, "tp-ignore-env", false, "ignore the TRACEPARENT envvar even if it's set")
	rootCmd.PersistentFlags().BoolVar(&traceparentPrint, "tp-print", false, "print the trace id, span id, and the w3c-formatted traceparent representation of the new span")
	// TOOD: probably remove this
	rootCmd.PersistentFlags().BoolVarP(&traceparentPrintExport, "tp-export", "p", false, "same as --tp-print but it puts an 'export ' in front so it's more convinenient to source in scripts")
}
