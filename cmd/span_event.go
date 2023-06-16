package otelcmd

import (
	"os"
	"time"

	"github.com/equinix-labs/otel-cli/otelcli"
	"github.com/spf13/cobra"
)

// spanEventCmd represents the span event command
func spanEventCmd(config *otelcli.Config) *cobra.Command {
	cmd := cobra.Command{
		Use:   "event",
		Short: "create an OpenTelemetry span event and add it to the background span",
		Long: `Create an OpenTelemetry span event as specified and send it out.

See: otel-cli span background

    sd=$(mktemp -d)
	otel-cli span background --sockdir $sd
	otel-cli span event \
	    --sockdir $sd \
		--name "did a cool thing" \
		--time $(date +%s.%N) \
		--attrs "os.kernel=$(uname -r)"
`,
		Run: doSpanEvent,
	}

	defaults := otelcli.DefaultConfig()

	cmd.Flags().SortFlags = false

	cmd.Flags().BoolVar(&config.Verbose, "verbose", defaults.Verbose, "print errors on failure instead of always being silent")
	// TODO
	//spanEventCmd.Flags().StringVar(&config.Timeout, "timeout", defaults.Timeout, "timeout for otel-cli operations, all timeouts in otel-cli use this value")
	cmd.Flags().StringVarP(&config.EventName, "name", "e", defaults.EventName, "set the name of the event")
	cmd.Flags().StringVarP(&config.EventTime, "time", "t", defaults.EventTime, "the precise time of the event in RFC3339Nano or Unix.nano format")
	cmd.Flags().StringVar(&config.BackgroundSockdir, "sockdir", "", "a directory where a socket can be placed safely")
	cmd.MarkFlagRequired("sockdir")

	addAttrParams(&cmd, config)

	return &cmd
}

func doSpanEvent(cmd *cobra.Command, args []string) {
	config := getConfig(cmd.Context())
	timestamp := otelcli.DefaultConfig().ParsedEventTime()
	rpcArgs := BgSpanEvent{
		Name:       config.EventName,
		Timestamp:  timestamp.Format(time.RFC3339Nano),
		Attributes: config.Attributes,
	}

	res := BgSpan{}
	client, shutdown := createBgClient(config)
	defer shutdown()
	err := client.Call("BgSpan.AddEvent", rpcArgs, &res)
	if err != nil {
		config.SoftFail("error while calling background server rpc BgSpan.AddEvent: %s", err)
	}

	if config.TraceparentPrint {
		tp, err := otelcli.ParseTraceparent(res.Traceparent)
		if err != nil {
			config.SoftFail("Could not parse traceparent: %s", err)
		}
		otelcli.PrintSpanData(os.Stdout, tp, nil, config.TraceparentPrintExport)
	}
}
