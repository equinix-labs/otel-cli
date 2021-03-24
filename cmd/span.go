package cmd

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

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

var startTime, endTime string
var epochNanoTimeRE *regexp.Regexp

func init() {
	rootCmd.AddCommand(spanCmd)
	spanCmd.PersistentFlags().StringVar(&startTime, "start", "", "a Unix epoch or RFC3339 timestamp for the start of the span")
	spanCmd.PersistentFlags().StringVar(&endTime, "end", "", "an Unix epoch or RFC3339 timestamp for the end of the span")

	epochNanoTimeRE = regexp.MustCompile(`^\d+\.\d+$`)
}

func doSpan(cmd *cobra.Command, args []string) {
	startOpts := []trace.SpanOption{trace.WithSpanKind(otelSpanKind())}
	endOpts := []trace.SpanOption{}

	if startTime != "" {
		t := parseTime(startTime, "start")
		startOpts = append(startOpts, trace.WithTimestamp(t))
	}

	if endTime != "" {
		t := parseTime(endTime, "end")
		endOpts = append(startOpts, trace.WithTimestamp(t))
	}

	ctx, shutdown := initTracer()
	defer shutdown()
	ctx = loadTraceparentFromEnv(ctx)
	tracer := otel.Tracer("otel-cli/span")

	ctx, span := tracer.Start(ctx, spanName, startOpts...)
	span.SetAttributes(cliAttrsToOtel()...) // applies CLI attributes to the span
	span.End(endOpts...)

	printSpanStdout(ctx, span)
}

// parseTime tries to parse Unix epoch, then RFC3339, both with/without nanoseconds
func parseTime(ts, which string) time.Time {
	var uterr, utnerr, utnnerr, rerr, rnerr error

	// Unix epoch time
	if i, uterr := strconv.ParseInt(ts, 10, 64); uterr == nil {
		return time.Unix(i, 0)
	}

	// Unix epoch time with nanoseconds
	if epochNanoTimeRE.MatchString(ts) {
		parts := strings.Split(ts, ".")
		if len(parts) == 2 {
			secs, utnerr := strconv.ParseInt(parts[0], 10, 64)
			nsecs, utnnerr := strconv.ParseInt(parts[1], 10, 64)
			if utnerr == nil && utnnerr == nil && secs > 0 {
				return time.Unix(secs, nsecs)
			}
		}
	}

	// try RFC3339 then again with nanos
	t, rerr := time.Parse(time.RFC3339, ts)
	if rerr != nil {
		t, rnerr := time.Parse(time.RFC3339Nano, ts)
		if rnerr == nil {
			return t
		}
	} else {
		return t
	}

	// none of the formats worked, print whatever errors are remaining
	if uterr != nil {
		log.Fatalf("Could not parse span %s time %q as Unix Epoch: %s", which, ts, uterr)
	}
	if utnerr != nil || utnnerr != nil {
		log.Fatalf("Could not parse span %s time %q as Unix Epoch.Nano: %s | %s", which, ts, utnerr, utnnerr)
	}
	if rerr != nil {
		log.Fatalf("Could not parse span %s time %q as RFC3339: %s", which, ts, rerr)
	}
	if rnerr != nil {
		log.Fatalf("Could not parse span %s time %q as RFC3339Nano: %s", which, ts, rnerr)
	}

	log.Fatalf("Could not parse span %s time %q as any supported format", which, ts)
	return time.Now() // never happens, just here to make compiler happy
}
