package otelcli

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

var detectBrokenRFC3339PrefixRe *regexp.Regexp

func init() {
	detectBrokenRFC3339PrefixRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} `)
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

// parseCliTimeout parses the cliTimeout global string value to a time.Duration.
// When no duration letter is provided (e.g. ms, s, m, h), seconds are assumed.
// It logs an error and returns time.Duration(0) if the string is empty or unparseable.
func parseCliTimeout(config Config) time.Duration {
	var out time.Duration
	if config.Timeout == "" {
		out = time.Duration(0)
	} else if d, err := time.ParseDuration(config.Timeout); err == nil {
		out = d
	} else if secs, serr := strconv.ParseInt(config.Timeout, 10, 0); serr == nil {
		out = time.Second * time.Duration(secs)
	} else {
		softLog("unable to parse --timeout %q: %s", config.Timeout, err)
		out = time.Duration(0)
	}

	diagnostics.ParsedTimeoutMs = out.Milliseconds()
	return out
}

// softLog only calls through to log if otel-cli was run with the --verbose flag.
// TODO: does it make any sense to support %w? probably yes, can clean up some
// diagnostics.Error touch points.
func softLog(format string, a ...interface{}) {
	if !config.Verbose {
		return
	}
	log.Printf(format, a...)
}

// softFail calls through to softLog (which logs only if otel-cli was run with the --verbose
// flag), then immediately exits - with status 0 by default, or 1 if --fail was
// set (a la `curl --fail`)
func softFail(format string, a ...interface{}) {
	softLog(format, a...)

	if !config.Fail {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}

// flattenStringMap takes a string map and returns it flattened into a string with
// keys sorted lexically so it should be mostly consistent enough for comparisons
// and printing. Output is k=v,k=v style like attributes input.
func flattenStringMap(mp map[string]string, emptyValue string) string {
	if len(mp) == 0 {
		return emptyValue
	}

	var out string
	keys := make([]string, len(mp)) // for sorting
	var i int
	for k := range mp {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for i, k := range keys {
		out = out + k + "=" + mp[k]
		if i == len(keys)-1 {
			break
		}
		out = out + ","
	}

	return out
}

// parseCkvStringMap parses key=value,foo=bar formatted strings as a line of CSV
// and returns it as a string map.
func parseCkvStringMap(in string) (map[string]string, error) {
	r := csv.NewReader(strings.NewReader(in))
	pairs, err := r.Read()
	if err != nil {
		return map[string]string{}, err
	}

	out := make(map[string]string)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if parts[0] != "" && parts[1] != "" {
			out[parts[0]] = parts[1]
		} else {
			return map[string]string{}, fmt.Errorf("kv pair %s must be in key=value format", pair)
		}
	}

	return out, nil
}
