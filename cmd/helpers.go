package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var detectBrokenRFC3339PrefixRe *regexp.Regexp

func init() {
	detectBrokenRFC3339PrefixRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} `)
}

// cliAttrsToOtel takes a map of string:string, such as that from --attrs
// and returns them in an []attribute.KeyValue.
func cliAttrsToOtel(attributes map[string]string) []attribute.KeyValue {
	otAttrs := []attribute.KeyValue{}
	for k, v := range attributes {

		// try to parse as numbers, and fall through to string
		var av attribute.Value
		if i, err := strconv.ParseInt(v, 0, 64); err == nil {
			av = attribute.Int64Value(i)
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			av = attribute.Float64Value(f)
		} else if b, err := strconv.ParseBool(v); err == nil {
			av = attribute.BoolValue(b)
		} else {
			av = attribute.StringValue(v)
		}

		akv := attribute.KeyValue{
			Key:   attribute.Key(k),
			Value: av,
		}

		otAttrs = append(otAttrs, akv)
	}

	return otAttrs
}

// parseTime tries to parse Unix epoch, then RFC3339, both with/without nanoseconds
func parseTime(ts, which string) time.Time {
	var uterr, utnerr, utnnerr, rerr, rnerr error

	if ts == "now" {
		return time.Now()
	}

	// Unix epoch time
	if i, uterr := strconv.ParseInt(ts, 10, 64); uterr == nil {
		return time.Unix(i, 0)
	}

	// date --rfc-3339 returns an invalid format for Go because it has a
	// space instead of 'T' between date and time
	if detectBrokenRFC3339PrefixRe.MatchString(ts) {
		ts = strings.Replace(ts, " ", "T", 1)
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
		softFail("Could not parse span %s time %q as Unix Epoch: %s", which, ts, uterr)
	}
	if utnerr != nil || utnnerr != nil {
		softFail("Could not parse span %s time %q as Unix Epoch.Nano: %s | %s", which, ts, utnerr, utnnerr)
	}
	if rerr != nil {
		softFail("Could not parse span %s time %q as RFC3339: %s", which, ts, rerr)
	}
	if rnerr != nil {
		softFail("Could not parse span %s time %q as RFC3339Nano: %s", which, ts, rnerr)
	}

	softFail("Could not parse span %s time %q as any supported format", which, ts)
	return time.Now() // never happens, just here to make compiler happy
}

// otelSpanKind takes a supported string span kind and returns the otel
// constant for it. Returns default of KindUnspecified on no match.
// TODO: figure out the best way to report invalid values
func otelSpanKind(kind string) trace.SpanKind {
	switch kind {
	case "client":
		return trace.SpanKindClient
	case "server":
		return trace.SpanKindServer
	case "producer":
		return trace.SpanKindProducer
	case "consumer":
		return trace.SpanKindConsumer
	case "internal":
		return trace.SpanKindInternal
	default:
		return trace.SpanKindUnspecified
	}
}

// GetExitCode() returns the exitCode value which is mainly used in exec.go
// so that the exit code of otel-cli matches the child program's exit code.
func GetExitCode() int {
	return exitCode
}

// propagateOtelCliSpan saves the traceparent to file if necessary, then prints
// span info to the console according to command-line args.
func propagateOtelCliSpan(ctx context.Context, span trace.Span, target io.Writer) {
	saveTraceparentToFile(ctx, config.TraceparentCarrierFile)

	if config.TraceparentPrint {
		sc := trace.SpanContextFromContext(ctx)
		traceId := sc.TraceID().String()
		spanId := sc.SpanID().String()

		tp := getTraceparent(ctx)
		printSpanData(target, traceId, spanId, tp)
	}
}

// printSpanData takes the provided strings and prints them in a consitent format,
// depending on which command line arguments were set.
func printSpanData(target io.Writer, traceId, spanId, tp string) {

	// --tp-export will print "export TRACEPARENT" so it's
	// one less step to print to a file & source, or eval
	var exported string
	if config.TraceparentPrintExport {
		exported = "export "
	}

	fmt.Fprintf(target, "# trace id: %s\n#  span id: %s\n%sTRACEPARENT=%s\n", traceId, spanId, exported, tp)
}

// parseCliTimeout parses the cliTimeout global string value to a time.Duration.
// When no duration letter is provided (e.g. ms, s, m, h), seconds are assumed.
// It logs an error and returns time.Duration(0) if the string is empty or unparseable.
func parseCliTimeout() time.Duration {
	if config.Timeout == "" {
		return time.Duration(0)
	}

	if d, err := time.ParseDuration(config.Timeout); err == nil {
		return d
	} else if secs, serr := strconv.ParseInt(config.Timeout, 10, 0); serr == nil {
		return time.Second * time.Duration(secs)
	} else {
		softLog("unable to parse --timeout %q: %s", config.Timeout, err)
		return time.Duration(0)
	}
}

// softLog only calls through to log if otel-cli was run with the --verbose flag.
func softLog(format string, a ...interface{}) {
	if !config.Verbose {
		return
	}
	log.Printf(format, a...)
}

// softFail only calls through to log if otel-cli was run with the --verbose
// flag, then immediately exits with status 0.
func softFail(format string, a ...interface{}) {
	if !config.Verbose {
		return
	}
	log.Printf(format, a...)
	os.Exit(0)
}
