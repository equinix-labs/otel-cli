package otelcli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

var detectBrokenRFC3339PrefixRe *regexp.Regexp
var epochNanoTimeRE *regexp.Regexp

func init() {
	detectBrokenRFC3339PrefixRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} `)
	epochNanoTimeRE = regexp.MustCompile(`^\d+\.\d+$`)
}

// cliAttrsToOtelPb takes a map of string:string, such as that from --attrs
// and returns them in an []*commonpb.KeyValue
func cliAttrsToOtelPb(attributes map[string]string) []*commonpb.KeyValue {
	out := []*commonpb.KeyValue{}

	for k, v := range attributes {
		av := new(commonpb.AnyValue)

		// try to parse as numbers, and fall through to string
		if i, err := strconv.ParseInt(v, 0, 64); err == nil {
			av.Value = &commonpb.AnyValue_IntValue{IntValue: i}
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			av.Value = &commonpb.AnyValue_DoubleValue{DoubleValue: f}
		} else if b, err := strconv.ParseBool(v); err == nil {
			av.Value = &commonpb.AnyValue_BoolValue{BoolValue: b}
		} else {
			av.Value = &commonpb.AnyValue_StringValue{StringValue: v}
		}

		akv := commonpb.KeyValue{
			Key:   k,
			Value: av,
		}

		out = append(out, &akv)
	}

	return out
}

// parseTime tries to parse Unix epoch, then RFC3339, both with/without nanoseconds
func parseTime(ts, which string) (time.Time, error) {
	var uterr, utnerr, utnnerr, rerr, rnerr error

	if ts == "now" {
		return time.Now(), nil
	}

	// Unix epoch time
	if i, uterr := strconv.ParseInt(ts, 10, 64); uterr == nil {
		return time.Unix(i, 0), nil
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
				return time.Unix(secs, nsecs), nil
			}
		}
	}

	// try RFC3339 then again with nanos
	t, rerr := time.Parse(time.RFC3339, ts)
	if rerr != nil {
		t, rnerr := time.Parse(time.RFC3339Nano, ts)
		if rnerr == nil {
			return t, nil
		}
	} else {
		return t, nil
	}

	// none of the formats worked, print whatever errors are remaining
	if uterr != nil {
		return time.Time{}, fmt.Errorf("Could not parse span %s time %q as Unix Epoch: %s", which, ts, uterr)
	}
	if utnerr != nil || utnnerr != nil {
		return time.Time{}, fmt.Errorf("Could not parse span %s time %q as Unix Epoch.Nano: %s | %s", which, ts, utnerr, utnnerr)
	}
	if rerr != nil {
		return time.Time{}, fmt.Errorf("Could not parse span %s time %q as RFC3339: %s", which, ts, rerr)
	}
	if rnerr != nil {
		return time.Time{}, fmt.Errorf("Could not parse span %s time %q as RFC3339Nano: %s", which, ts, rnerr)
	}

	return time.Time{}, fmt.Errorf("Could not parse span %s time %q as any supported format", which, ts)
}
