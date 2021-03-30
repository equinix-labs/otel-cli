package cmd

import (
	"log"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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

// otelSpanKind takes a supported string span kind and returns the otel
// constant for it. Returns default of KindUnspecified on no match.
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
