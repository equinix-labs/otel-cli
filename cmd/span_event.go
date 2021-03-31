package cmd

import (
	"log"
	"time"

	"github.com/spf13/cobra"
)

var spanEventName, spanEventTime string

// spanEventCmd represents the span event command
var spanEventCmd = &cobra.Command{
	Use:   "event",
	Short: "create an OpenTelemetry span event and add it to the background span",
	Long: `Create an OpenTelemetry span event as specified and send it out.

See: otel-cli span background

	otel-cli span event \
	    --sockdir $sockdir \
		--event-name "did a cool thing" \
		--time $(date +%s.%N) \
		--attrs "os.kernel=$(uname -r)"
`,
	Run: doSpanEvent,
}

func init() {
	spanCmd.AddCommand(spanEventCmd)
	spanEventCmd.Flags().SortFlags = false

	// --event-name / -e
	spanEventCmd.Flags().StringVarP(&spanEventName, "event-name", "e", "todo-generate-default-event-names", "set the name of the event")

	// --time / -t
	spanEventCmd.Flags().StringVarP(&spanEventTime, "time", "t", "now", "the precise time of the event in RFC3339Nano or Unix.nano format")

	// --sockdir
	// TODO: make this required for events
	spanEventCmd.Flags().StringVar(&spanBgSockdir, "sockdir", "", "a directory where a socket can be placed safely")
}

func doSpanEvent(cmd *cobra.Command, args []string) {
	timestamp := parseTime(spanEventTime, "event")
	rpcArgs := BgSpanEvent{
		Name:       spanEventName,
		Timestamp:  timestamp.Format(time.RFC3339Nano),
		Attributes: spanAttrs,
	}

	res := BgSpan{}
	client, shutdown := createBgClient()
	defer shutdown()
	err := client.Call("BgSpan.AddEvent", rpcArgs, &res)
	if err != nil {
		log.Fatalf("error while calling background server rpc BgSpan.AddEvent: %s", err)
	}

	printSpanData(res.TraceID, res.SpanID, res.Traceparent)
}
