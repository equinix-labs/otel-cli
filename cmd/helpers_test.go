package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func TestCliAttrsToOtel(t *testing.T) {

	testAttrs := map[string]string{
		"test 1 - string":      "isn't testing fun?",
		"test 2 - int64":       "111111111",
		"test 3 - float":       "2.4391111",
		"test 4 - bool, true":  "true",
		"test 5 - bool, false": "false",
		"test 6 - bool, True":  "True",
		"test 7 - bool, False": "False",
	}

	otelAttrs := cliAttrsToOtel(testAttrs)

	// can't count on any ordering from map -> array
	for _, attr := range otelAttrs {
		key := string(attr.Key)
		switch key {
		case "test 1 - string":
			if attr.Value.AsString() != testAttrs[key] {
				t.Errorf("expected value '%s' for key '%s' but got '%s'", testAttrs[key], key, attr.Value.AsString())
			}
		case "test 2 - int64":
			if attr.Value.AsInt64() != 111111111 {
				t.Errorf("expected value '%s' for key '%s' but got %d", testAttrs[key], key, attr.Value.AsInt64())
			}
		case "test 3 - float":
			if attr.Value.AsFloat64() != 2.4391111 {
				t.Errorf("expected value '%s' for key '%s' but got %f", testAttrs[key], key, attr.Value.AsFloat64())
			}
		case "test 4 - bool, true":
			if attr.Value.AsBool() != true {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.AsBool())
			}
		case "test 5 - bool, false":
			if attr.Value.AsBool() != false {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.AsBool())
			}
		case "test 6 - bool, True":
			if attr.Value.AsBool() != true {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.AsBool())
			}
		case "test 7 - bool, False":
			if attr.Value.AsBool() != false {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.AsBool())
			}
		}
	}
}

func TestParseTime(t *testing.T) {
	mustParse := func(layout, value string) time.Time {
		out, err := time.Parse(layout, value)
		if err != nil {
			t.Fatalf("failed to parse time '%s' as format '%s': %s", value, layout, err)
		}
		return out
	}

	for _, testcase := range []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "Unix epoch time without nanoseconds",
			input: "1617739561", // date +%s
			want:  time.Unix(1617739561, 0),
		},
		{
			name:  "Unix epoch time with nanoseconds",
			input: "1617739615.759793032", // date +%s.%N
			want:  time.Unix(1617739615, 759793032),
		},
		{
			name:  "RFC3339",
			input: "2021-04-06T13:07:54Z",
			want:  mustParse(time.RFC3339, "2021-04-06T13:07:54Z"),
		},
		{
			name:  "RFC3339 with nanoseconds",
			input: "2021-04-06T13:12:40.792426395Z",
			want:  mustParse(time.RFC3339Nano, "2021-04-06T13:12:40.792426395Z"),
		},
		// date(1) RFC3339 format is incompatible with Go's formats
		// so parseTime takes care of that automatically
		{
			name:  "date(1) RFC3339 output, with timezone",
			input: "2021-04-06 13:07:54-07:00", //date --rfc-3339=seconds
			want:  mustParse(time.RFC3339, "2021-04-06T13:07:54-07:00"),
		},
		{
			name:  "date(1) RFC3339 with nanoseconds and timezone",
			input: "2021-04-06 13:12:40.792426395-07:00", // date --rfc-3339=ns
			want:  mustParse(time.RFC3339Nano, "2021-04-06T13:12:40.792426395-07:00"),
		},
		// TODO: maybe refactor parseTime to make failures easier to validate?
		// @tobert: gonna leave that for functional tests for now
	} {
		t.Run(testcase.name, func(t *testing.T) {
			out := parseTime(testcase.input, "test")
			if !out.Equal(testcase.want) {
				t.Errorf("got wrong time from parseTime: %s", out.Format(time.RFC3339Nano))
			}
		})
	}
}

func TestOtelSpanKind(t *testing.T) {

	for _, testcase := range []struct {
		name string
		want trace.SpanKind
	}{
		{
			name: "client",
			want: trace.SpanKindClient,
		},
		{
			name: "server",
			want: trace.SpanKindServer,
		},
		{
			name: "producer",
			want: trace.SpanKindProducer,
		},
		{
			name: "consumer",
			want: trace.SpanKindConsumer,
		},
		{
			name: "internal",
			want: trace.SpanKindInternal,
		},
		{
			name: "invalid",
			want: trace.SpanKindUnspecified,
		},
		{
			name: "speledwrong",
			want: trace.SpanKindUnspecified,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			out := otelSpanKind(testcase.name)
			if out != testcase.want {
				t.Errorf("otelSpanKind returned the wrong value, '%q', for '%s'", out, testcase.name)
			}
		})
	}
}

func TestPropagateOtelCliSpan(t *testing.T) {
	// TODO: should this noop the tracing backend?

	// set package globals to a known state
	traceparentCarrierFile = ""
	traceparentPrint = false
	traceparentPrintExport = false

	tp := "00-3433d5ae39bdfee397f44be5146867b3-8a5518f1e5c54d0a-01"
	tid := "3433d5ae39bdfee397f44be5146867b3"
	sid := "8a5518f1e5c54d0a"
	os.Setenv("TRACEPARENT", tp)
	tracer := otel.Tracer("testing/propagateOtelCliSpan")
	ctx, span := tracer.Start(context.Background(), "testing propagateOtelCliSpan")

	buf := new(bytes.Buffer)
	// mostly smoke testing this, will validate printSpanData output
	// TODO: maybe validate the file write works, but that's tested elsewhere...
	propagateOtelCliSpan(ctx, span, buf)
	if buf.Len() != 0 {
		t.Errorf("nothing was supposed to be written but %d bytes were", buf.Len())
	}

	traceparentPrint = true
	traceparentPrintExport = true
	buf = new(bytes.Buffer)
	printSpanData(buf, tid, sid, tp)
	if buf.Len() == 0 {
		t.Error("expected more than zero bytes but got none")
	}
	expected := fmt.Sprintf("# trace id: %s\n#  span id: %s\nexport TRACEPARENT=%s\n", tid, sid, tp)
	if buf.String() != expected {
		t.Errorf("got unexpected output, expected '%s', got '%s'", expected, buf.String())
	}
}

func TestParseCliTime(t *testing.T) {
	for _, testcase := range []struct {
		name     string
		input    string
		expected time.Duration
	}{
		// otel-cli will still timeout but it will be the default timeouts for
		// each component
		{
			name:     "empty string returns 0 duration",
			input:    "",
			expected: time.Duration(0),
		},
		{
			name:     "0 returns 0 duration",
			input:    "0",
			expected: time.Duration(0),
		},
		{
			name:     "1s returns 1 second",
			input:    "1s",
			expected: time.Second,
		},
		{
			name:     "100ms returns 100 milliseconds",
			input:    "100ms",
			expected: time.Millisecond * 100,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			cliTimeout = testcase.input
			got := parseCliTimeout()
			if got != testcase.expected {
				ed := testcase.expected.String()
				gd := got.String()
				t.Errorf("duration string %q was expected to return %s but returned %s", cliTimeout, ed, gd)
			}
		})
	}
}
