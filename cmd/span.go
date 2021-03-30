package cmd

import (
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
		--system-name "my-application" \
		--span-name "send data to the server" \
		--start 2021-03-24T07:28:05.12345Z \
		--end $(date +%s.%N) \
		--attrs "os.kernel=$(uname -r)" \
		--print-traceparent
`,
	Run: doSpan,
}

var spanStartTime, spanEndTime string
var epochNanoTimeRE *regexp.Regexp

func init() {
	rootCmd.AddCommand(spanCmd)
	spanCmd.Flags().SortFlags = false

	// --span-name / -s
	spanCmd.Flags().StringVarP(&spanName, "span-name", "s", "todo-generate-default-span-names", "set the name of the span")

	// --start $timestamp (RFC3339 or Unix_Epoch.Nanos)
	spanCmd.Flags().StringVar(&spanStartTime, "start", "", "a Unix epoch or RFC3339 timestamp for the start of the span")

	// --end $timestamp
	spanCmd.Flags().StringVar(&spanEndTime, "end", "", "an Unix epoch or RFC3339 timestamp for the end of the span")

	epochNanoTimeRE = regexp.MustCompile(`^\d+\.\d+$`)
}

func doSpan(cmd *cobra.Command, args []string) {
	startOpts := []trace.SpanOption{trace.WithSpanKind(otelSpanKind(spanKind))}
	endOpts := []trace.SpanOption{}

	if spanStartTime != "" {
		t := parseTime(spanStartTime, "start")
		startOpts = append(startOpts, trace.WithTimestamp(t))
	}

	if spanEndTime != "" {
		t := parseTime(spanEndTime, "end")
		endOpts = append(endOpts, trace.WithTimestamp(t))
	}

	ctx, shutdown := initTracer()
	defer shutdown()
	ctx = loadTraceparentFromEnv(ctx)
	tracer := otel.Tracer("otel-cli/span")

	ctx, span := tracer.Start(ctx, spanName, startOpts...)
	span.SetAttributes(cliAttrsToOtel(spanAttrs)...) // applies CLI attributes to the span
	span.End(endOpts...)

	printSpanStdout(ctx, span)
}
