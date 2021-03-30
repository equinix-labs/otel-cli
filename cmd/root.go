package cmd

import (
	"fmt"
	"os"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var cfgFile, appName, spanName, spanKind string
var spanAttrs map[string]string
var ignoreTraceparentEnv, printTraceparent, printTraceparentExport bool
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
	cobra.OnInitialize(initConfig)

	rootCmd.Flags().SortFlags = false

	// global parameters
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.otel-cli.yaml)")
	// TODO: put in global flags for the otel endpoint and stuff like that here

	rootCmd.PersistentFlags().StringVarP(&appName, "service-name", "n", "otel-cli", "set the name of the application sent on the traces")
	// TODO: probably want to bind this to viper? seems handy...

	// all commands and subcommands accept attributes, some might ignore
	// e.g. `--attrs "foo=bar,baz=inga"`
	rootCmd.PersistentFlags().StringToStringVarP(&spanAttrs, "attrs", "a", map[string]string{}, "a comma-separated list of key=value attributes")
	// TODO: this is just a guess for now
	viperBindFlag("otel-cli.attributes", rootCmd.PersistentFlags().Lookup("attrs"))

	rootCmd.PersistentFlags().StringVarP(&spanKind, "kind", "k", "client", "set the trace kind, e.g. internal, server, client, producer, consumer")

	rootCmd.PersistentFlags().BoolVar(&ignoreTraceparentEnv, "ignore-tp-env", false, "ignore the TRACEPARENT envvar even if it's set")

	rootCmd.PersistentFlags().BoolVar(&printTraceparent, "print-tp", false, "print the trace id, span id, and the w3c-formatted traceparent representation of the new span")
	rootCmd.PersistentFlags().BoolVarP(&printTraceparentExport, "print-tp-export", "p", false, "same as --print-tp but it puts an 'export ' in front so it's more convinenient to source in scripts")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".otel-cli" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".otel-cli")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// viperBindFlag provides a wrapper around the viper bindings that handles error checks
func viperBindFlag(name string, flag *pflag.Flag) {
	err := viper.BindPFlag(name, flag)
	if err != nil {
		panic(err)
	}
}
