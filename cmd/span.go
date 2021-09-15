package cmd

import (
	"context"
	"os"
	"regexp"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// spanCmd represents the span command
var spanCmd = &cobra.Command{
	Use:   "span",
	Short: "create an OpenTelemetry span and send it",
	Long: `Create an OpenTelemetry span as specified and send it along. The
span can be customized with a start/end time via RFC3339 or Unix epoch format,
with support for nanoseconds on both.

Example:
	otel-cli span \
		--system "my-application" \
		--name "send data to the server" \
		--start 2021-03-24T07:28:05.12345Z \
		--end $(date +%s.%N) \
		--attrs "os.kernel=$(uname -r)" \
		--tp-print
`,
	Run: doSpan,
}

var epochNanoTimeRE *regexp.Regexp

func init() {
	rootCmd.AddCommand(spanCmd)
	spanCmd.Flags().SortFlags = false

	// --start $timestamp (RFC3339 or Unix_Epoch.Nanos)
	spanCmd.Flags().StringVar(&config.SpanStartTime, "start", defaults.SpanStartTime, "a Unix epoch or RFC3339 timestamp for the start of the span")

	// --end $timestamp
	spanCmd.Flags().StringVar(&config.SpanEndTime, "end", defaults.SpanEndTime, "an Unix epoch or RFC3339 timestamp for the end of the span")

	addCommonParams(spanCmd)
	addSpanParams(spanCmd)
	addAttrParams(spanCmd)
	addClientParams(spanCmd)

	epochNanoTimeRE = regexp.MustCompile(`^\d+\.\d+$`)
}

func doSpan(cmd *cobra.Command, args []string) {
	ctx, span, shutdown := startSpan()
	endSpan(span)
	propagateOtelCliSpan(ctx, span, os.Stdout)
	shutdown()
}

// startSpan processes the optional --start option, starts a span, and returns a
// context, the span, and a deferrable function for clean shutdown (it ends the
// span).
func startSpan() (context.Context, trace.Span, func()) {
	t := parseTime(config.SpanStartTime, "start")
	startOpts := []trace.SpanStartOption{
		trace.WithSpanKind(otelSpanKind(config.Kind)),
		trace.WithTimestamp(t),
	}

	ctx, shutdown := initTracer()
	ctx = loadTraceparent(ctx, config.TraceparentCarrierFile)
	tracer := otel.Tracer("otel-cli/span")

	ctx, span := tracer.Start(ctx, config.SpanName, startOpts...)
	span.SetAttributes(cliAttrsToOtel(config.Attributes)...) // applies CLI attributes to the span

	return ctx, span, shutdown
}

// endSpan takes a span, checks for a --end command-line option, and ends the span.
func endSpan(span trace.Span) {
	t := parseTime(config.SpanEndTime, "end")
	endOpts := []trace.SpanEndOption{trace.WithTimestamp(t)}
	span.End(endOpts...)
}
