package cmd

import (
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TODO: that's a lot of globals, maybe move this into a struct
var cfgFile, serviceName, spanName, spanKind, traceparentCarrierFile string
var spanAttrs, otlpHeaders map[string]string
var spanStatus bool
var otlpEndpoint string
var otlpInsecure, otlpBlocking bool
var traceparentIgnoreEnv, traceparentPrint, traceparentPrintExport bool
var traceparentRequired, testMode bool
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
	spanStatus = false
	otlpHeaders = make(map[string]string)
	cobra.OnInitialize(initViperConfig)
	cobra.EnableCommandSorting = false
	rootCmd.Flags().SortFlags = false

	// --config / -c a viper configuration file
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.otel-cli.yaml)")

	// envvars are named according to the otel specs, others use the OTEL_CLI prefix
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/sdk-environment-variables.md
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md

	rootCmd.PersistentFlags().StringVar(&otlpEndpoint, "endpoint", "localhost:4317", "dial address for the desired OTLP/gRPC endpoint")
	viper.BindPFlag("endpoint", rootCmd.PersistentFlags().Lookup("endpoint"))
	viper.BindEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "endpoint")

	rootCmd.PersistentFlags().BoolVar(&otlpInsecure, "insecure", false, "refuse to connect if TLS is unavailable (true by default when endpoint is localhost)")
	viper.BindPFlag("insecure", rootCmd.PersistentFlags().Lookup("insecure"))
	viper.BindEnv("OTEL_EXPORTER_OTLP_INSECURE", "insecure")

	rootCmd.PersistentFlags().StringToStringVar(&otlpHeaders, "otlp-headers", map[string]string{}, "a comma-sparated list of key=value headers to send on OTLP connection")
	viper.BindPFlag("otlp-headers", rootCmd.PersistentFlags().Lookup("otlp-headers"))
	viper.BindEnv("OTEL_EXPORTER_OTLP_HEADERS", "otlp-headers")

	rootCmd.PersistentFlags().BoolVar(&otlpBlocking, "otlp-blocking", false, "block on connecting to the OTLP server before proceding")
	viper.BindPFlag("otlp-blocking", rootCmd.PersistentFlags().Lookup("otlp-blocking"))
	viper.BindEnv("OTEL_EXPORTER_OTLP_BLOCKING", "otlp-blocking")

	rootCmd.PersistentFlags().StringVarP(&serviceName, "service", "n", "otel-cli", "set the name of the application sent on the traces")
	viper.BindPFlag("service", rootCmd.PersistentFlags().Lookup("service"))
	viper.BindEnv("OTEL_CLI_SERVICE_NAME", "service")

	rootCmd.PersistentFlags().StringVarP(&spanKind, "kind", "k", "client", "set the trace kind, e.g. internal, server, client, producer, consumer")
	viper.BindPFlag("kind", rootCmd.PersistentFlags().Lookup("kind"))
	viper.BindEnv("OTEL_CLI_TRACE_KIND", "kind")

	rootCmd.PersistentFlags().StringToStringVarP(&spanAttrs, "attrs", "a", map[string]string{}, "a comma-separated list of key=value attributes")
	viper.BindPFlag("attrs", rootCmd.PersistentFlags().Lookup("attrs"))
	viper.BindEnv("OTEL_CLI_ATTRIBUTES", "attrs")

	rootCmd.PersistentFlags().StringToStringVarP(&spanStatus, "status", "s", false, "when set to true, mark span status as Error")
	viper.BindPFlag("status", rootCmd.PersistentFlags().Lookup("status"))
	viper.BindEnv("OTEL_SPAN_STATUS", "status")

	// trace propagation options
	rootCmd.PersistentFlags().BoolVar(&traceparentRequired, "tp-required", false, "when set to true, fail and log if a traceparent can't be picked up from TRACEPARENT ennvar or a carrier file")
	viper.BindPFlag("tp-required", rootCmd.PersistentFlags().Lookup("tp-required"))
	viper.BindEnv("OTEL_CLI_TRACEPARENT_REQUIRED", "tp-required")

	rootCmd.PersistentFlags().StringVar(&traceparentCarrierFile, "tp-carrier", "", "a file for reading and WRITING traceparent across invocations")
	viper.BindPFlag("tp-carrier", rootCmd.PersistentFlags().Lookup("tp-carrier"))
	viper.BindEnv("OTEL_CLI_CARRIER_FILE", "tp-carrier")

	rootCmd.PersistentFlags().BoolVar(&traceparentIgnoreEnv, "tp-ignore-env", false, "ignore the TRACEPARENT envvar even if it's set")
	viper.BindPFlag("tp-ignore-env", rootCmd.PersistentFlags().Lookup("tp-ignore-env"))
	viper.BindEnv("OTEL_CLI_IGNORE_ENV", "tp-ignore-env")

	rootCmd.PersistentFlags().BoolVar(&traceparentPrint, "tp-print", false, "print the trace id, span id, and the w3c-formatted traceparent representation of the new span")
	viper.BindPFlag("tp-print", rootCmd.PersistentFlags().Lookup("tp-print"))
	viper.BindEnv("OTEL_CLI_PRINT_TRACEPARENT", "tp-print")

	rootCmd.PersistentFlags().BoolVarP(&traceparentPrintExport, "tp-export", "p", false, "same as --tp-print but it puts an 'export ' in front so it's more convinenient to source in scripts")
	viper.BindPFlag("tp-export", rootCmd.PersistentFlags().Lookup("tp-export"))
	viper.BindEnv("OTEL_CLI_EXPORT_TRACEPARENT", "tp-export")

	rootCmd.PersistentFlags().BoolVar(&testMode, "test", false, "configure noop exporter and dump data to json on stdout instead of sending")
}

func initViperConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.SetConfigName(".otel-cli") // e.g. ~/.otel-cli.yaml
	}

	if err := viper.ReadInConfig(); err != nil {
		// We want to suppress errors here if the config is not found, but only if the user has not expressly given us a location to search.
		// Otherwise, we'll raise any config-reading error up to the user.
		_, cfgNotFound := err.(viper.ConfigFileNotFoundError)
		if cfgFile != "" || !cfgNotFound {
			cobra.CheckErr(err)
		}
	}
}
