package otelcli

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "otel-cli",
	Short: "CLI for creating and sending OpenTelemetry spans and events.",
	Long:  `A command-line interface for generating OpenTelemetry data on the command line.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if err := config.LoadFile(); err != nil {
			softFail("Error while loading configuration file %s: %s", config.CfgFile, err)
		}
		if err := config.LoadEnv(os.Getenv); err != nil {
			// will need to specify --fail --verbose flags to see these errors
			softFail("Error while loading environment variables: %s", err)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(version string) {
	rootCmd.Version = version
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.EnableCommandSorting = false
	rootCmd.Flags().SortFlags = false

	diagnostics.NumArgs = len(os.Args) - 1
	diagnostics.CliArgs = []string{}
	if len(os.Args) > 1 {
		diagnostics.CliArgs = os.Args[1:]
	}
}

// addCommonParams adds the --config and --endpoint params to the command.
func addCommonParams(cmd *cobra.Command) {
	// --config / -c a JSON configuration file
	cmd.Flags().StringVarP(&config.CfgFile, "config", "c", defaults.CfgFile, "JSON configuration file")
	// --endpoint an endpoint to send otlp output to
	cmd.Flags().StringVar(&config.Endpoint, "endpoint", defaults.Endpoint, "host and port for the desired OTLP/gRPC or OTLP/HTTP endpoint (use http:// or https:// for OTLP/HTTP)")
	// --protocol allows setting the OTLP protocol instead of relying on auto-detection from URI
	cmd.Flags().StringVar(&config.Protocol, "protocol", defaults.Protocol, "desired OTLP protocol: grpc or http/protobuf")
	// --timeout a default timeout to use in all otel-cli operations (default 1s)
	cmd.Flags().StringVar(&config.Timeout, "timeout", defaults.Timeout, "timeout for otel-cli operations, all timeouts in otel-cli use this value")
	// --verbose tells otel-cli to actually log errors to stderr instead of failing silently
	cmd.Flags().BoolVar(&config.Verbose, "verbose", defaults.Verbose, "print errors on failure instead of always being silent")
	// --fail causes a non-zero exit status on error
	cmd.Flags().BoolVar(&config.Fail, "fail", defaults.Fail, "on failure, exit with a non-zero status")
}

// addClientParams adds the common CLI flags for e.g. span and exec to the command.
// envvars are named according to the otel specs, others use the OTEL_CLI prefix
// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/sdk-environment-variables.md
// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
func addClientParams(cmd *cobra.Command) {
	config.Headers = make(map[string]string)
	// OTEL_EXPORTER standard env and variable params
	cmd.Flags().BoolVar(&config.Insecure, "insecure", defaults.Insecure, "refuse to connect if TLS is unavailable (true by default when endpoint is localhost)")
	cmd.Flags().StringToStringVar(&config.Headers, "otlp-headers", defaults.Headers, "a comma-sparated list of key=value headers to send on OTLP connection")
	cmd.Flags().BoolVar(&config.Blocking, "otlp-blocking", defaults.Blocking, "block on connecting to the OTLP server before proceeding")

	cmd.Flags().StringVar(&config.Certificate, "cacert", defaults.Certificate, "a file containing the server's CA certificate")
	cmd.Flags().StringVar(&config.ClientCert, "client-cert", defaults.ClientCert, "a file containing the client certificate")
	cmd.Flags().StringVar(&config.ClientKey, "client-key", defaults.ClientKey, "a file containing the client certificate key")

	// OTEL_CLI trace propagation options
	cmd.Flags().BoolVar(&config.TraceparentRequired, "tp-required", defaults.TraceparentRequired, "when set to true, fail and log if a traceparent can't be picked up from TRACEPARENT ennvar or a carrier file")
	cmd.Flags().StringVar(&config.TraceparentCarrierFile, "tp-carrier", defaults.TraceparentCarrierFile, "a file for reading and WRITING traceparent across invocations")
	cmd.Flags().BoolVar(&config.TraceparentIgnoreEnv, "tp-ignore-env", defaults.TraceparentIgnoreEnv, "ignore the TRACEPARENT envvar even if it's set")
	cmd.Flags().BoolVar(&config.TraceparentPrint, "tp-print", defaults.TraceparentPrint, "print the trace id, span id, and the w3c-formatted traceparent representation of the new span")
	cmd.Flags().BoolVarP(&config.TraceparentPrintExport, "tp-export", "p", defaults.TraceparentPrintExport, "same as --tp-print but it puts an 'export ' in front so it's more convinenient to source in scripts")
	cmd.Flags().BoolVar(&config.NoTlsVerify, "no-tls-verify", defaults.NoTlsVerify, "enable it when TLS is enabled and you want to ignore the certificate validation. This is common when you are testing with self-signed certificates.")
}

func addSpanParams(cmd *cobra.Command) {
	// --name / -s
	cmd.Flags().StringVarP(&config.SpanName, "name", "n", defaults.SpanName, "set the name of the span")
	// --service / -n
	cmd.Flags().StringVarP(&config.ServiceName, "service", "s", defaults.ServiceName, "set the name of the application sent on the traces")
	// --kind / -k
	cmd.Flags().StringVarP(&config.Kind, "kind", "k", defaults.Kind, "set the trace kind, e.g. internal, server, client, producer, consumer")
	// --status-code / -sc
	cmd.Flags().StringVar(&config.StatusCode, "status-code", defaults.StatusCode, "set the span status code, e.g. unset|ok|error")
	// --status-description / -sd
	cmd.Flags().StringVar(&config.StatusDescription, "status-description", defaults.StatusDescription, "set the span status description when a span status code of error is set, e.g. 'cancelled'")
}

func addAttrParams(cmd *cobra.Command) {
	// --attrs key=value,foo=bar
	config.Attributes = make(map[string]string)
	cmd.Flags().StringToStringVarP(&config.Attributes, "attrs", "a", defaults.Attributes, "a comma-separated list of key=value attributes")
}
