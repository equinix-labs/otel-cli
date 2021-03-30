package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/trace"
)

var spanEventName, spanEventTime string

// eventCmd represents the event command
var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "(doesn't work yet) create an OpenTelemetry span event and send it",
	Long: `Create an OpenTelemetry span event as specified and send it out.
Note: this doesn't work

Note: this only works when TRACEPARENT is set to the parent span!

Example:
    otel-cli span --print-traceparent > $tmpfile
	source $tempfile
	export TRACEPARENT

	otel-cli span event \
		--event-name "send data to the server" \
		--time $(date +%s.%N) \
		--attrs "os.kernel=$(uname -r)"
`,
	Run: doSpanEvent,
}

func init() {
	spanCmd.AddCommand(eventCmd)
	spanCmd.Flags().SortFlags = false

	// --event-name / -e
	eventCmd.Flags().StringVarP(&spanEventName, "event-name", "e", "todo-generate-default-event-names", "set the name of the event")

	// --time / -t
	eventCmd.Flags().StringVarP(&spanEventTime, "time", "t", "now", "the precise time of the event in RFC3339Nano or Unix.nano format")
}

func doSpanEvent(cmd *cobra.Command, args []string) {
	timestamp := parseTime(spanEventTime, "event")

	options := []trace.EventOption{
		trace.WithTimestamp(timestamp),
		trace.WithAttributes(cliAttrsToOtel(spanAttrs)...),
	}

	ctx, shutdown := initTracer()
	defer shutdown()
	ctx = loadTraceparentFromEnv(ctx)

	// loadTraceParentFromEnv read the envvar and loaded it into the context
	// but otel-go does not set it as the current span automatically, so there's
	// a little more work here and no easy to find helpers...
	tp := getTraceparent(ctx)                // get the text representation again
	parts := strings.Split(tp, "-")          // e.g. 00-9765b2f71c68b04dc0ad2a4d73027d6f-1881444346b6296e-01
	tid, _ := trace.TraceIDFromHex(parts[1]) // TODO: handle error
	sid, _ := trace.SpanIDFromHex(parts[2])  // TODO: handle error
	scc := trace.SpanContextConfig{
		TraceID: tid,
		SpanID:  sid,
	}
	sc := trace.NewSpanContext(scc)
	// print debug json to console
	js, _ := json.Marshal(sc)
	fmt.Printf("sc: %s\n", string(js))

	// set the current span context to the one with the right span & trace ids
	// this is the closest thing I could find, but does not seem to work...
	// might need to ask otel-go for a trace.ContextWithSpanContext()
	// or really a trace.SpanWithSpanContext()?
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	//span := trace.SpanFromContext(ctx)
	//fmt.Printf("is it recording?: %b\n", span.IsRecording())

	// doesn't work
	span := trace.SpanFromContext(ctx)
	fmt.Printf("is it recording?: %q\n", span.IsRecording())

	// create a slug span using the usual plumbing so it has its parent context
	// all set up to steal, but we'll throw the span away after that
	// ... also this does not help or work
	//tracer := otel.Tracer("otel-cli/exec")
	//ctx, slugSpan := tracer.Start(ctx, "[otel-cli/span/event has a bug if you see this]")

	span.AddEvent(spanEventName, options...)

	printSpanStdout(ctx, span)
}
