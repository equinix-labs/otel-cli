package otelcli

import (
	"os"
	"time"

	"github.com/spf13/cobra"
)

// spanEventCmd represents the span event command
var spanEventCmd = &cobra.Command{
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

func init() {
	spanCmd.AddCommand(spanEventCmd)
	spanEventCmd.Flags().SortFlags = false

	spanEventCmd.Flags().BoolVar(&config.Verbose, "verbose", defaults.Verbose, "print errors on failure instead of always being silent")
	// TODO
	//spanEventCmd.Flags().StringVar(&config.Timeout, "timeout", defaults.Timeout, "timeout for otel-cli operations, all timeouts in otel-cli use this value")
	spanEventCmd.Flags().StringVarP(&config.EventName, "name", "e", defaults.EventName, "set the name of the event")
	spanEventCmd.Flags().StringVarP(&config.EventTime, "time", "t", defaults.EventTime, "the precise time of the event in RFC3339Nano or Unix.nano format")
	spanEventCmd.Flags().StringVar(&config.BackgroundSockdir, "sockdir", "", "a directory where a socket can be placed safely")
	spanEventCmd.MarkFlagRequired("sockdir")

	addAttrParams(spanEventCmd)
}

func doSpanEvent(cmd *cobra.Command, args []string) {
	timestamp := parseTime(config.EventTime, "event")
	rpcArgs := BgSpanEvent{
		Name:       config.EventName,
		Timestamp:  timestamp.Format(time.RFC3339Nano),
		Attributes: config.Attributes,
	}

	res := BgSpan{}
	client, shutdown := createBgClient()
	defer shutdown()
	err := client.Call("BgSpan.AddEvent", rpcArgs, &res)
	if err != nil {
		softFail("error while calling background server rpc BgSpan.AddEvent: %s", err)
	}

	if config.TraceparentPrint {
		printSpanData(os.Stdout, res.TraceID, res.SpanID, res.Traceparent)
	}
}
