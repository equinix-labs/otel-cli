package otelcli

import (
	"os"
	"regexp"

	"github.com/spf13/cobra"
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
		--service "my-application" \
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
	defaults := DefaultConfig()

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
	ctx, client := StartClient(config)
	span := NewProtobufSpanWithConfig(config)
	SendSpan(ctx, client, config, span)
	propagateTraceparent(span, os.Stdout)
}
