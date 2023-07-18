package otelcli

import (
	"os"

	"github.com/equinix-labs/otel-cli/w3c/traceparent"
	"github.com/spf13/cobra"
)

// spanEndCmd represents the span event command
func spanEndCmd(config *Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "end",
		Short: "Make a span background to end itself and exit gracefully",
		Long: `Gracefully end a background span and have its process exit.

See: otel-cli span background

	otel-cli span end --sockdir $sockdir
`,
		Run: doSpanEnd,
	}

	defaults := DefaultConfig()

	cmd.Flags().BoolVar(&config.Verbose, "verbose", defaults.Verbose, "print errors on failure instead of always being silent")
	// TODO
	//cmd.Flags().StringVar(&config.Timeout, "timeout", defaults.Timeout, "timeout for otel-cli operations, all timeouts in otel-cli use this value")
	cmd.Flags().StringVar(&config.BackgroundSockdir, "sockdir", defaults.BackgroundSockdir, "a directory where a socket can be placed safely")
	cmd.MarkFlagRequired("sockdir")

	cmd.Flags().StringVar(&config.SpanEndTime, "end", defaults.SpanEndTime, "an Unix epoch or RFC3339 timestamp for the end of the span")

	addSpanStatusParams(&cmd, config)

	return &cmd
}

func doSpanEnd(cmd *cobra.Command, args []string) {
	config := getConfig(cmd.Context())
	client, shutdown := createBgClient(config)

	rpcArgs := BgEnd{
		StatusCode: config.StatusCode,
		StatusDesc: config.StatusDescription,
	}

	res := BgSpan{}
	err := client.Call("BgSpan.End", rpcArgs, &res)
	if err != nil {
		config.SoftFail("error while calling background server rpc BgSpan.End: %s", err)
	}
	shutdown()

	tp, _ := traceparent.Parse(res.Traceparent)
	if config.TraceparentPrint {
		tp.Fprint(os.Stdout, config.TraceparentPrintExport)
	}
}
