package otelcli

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/trace"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
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

	otelAttrs := cliAttrsToOtelPb(testAttrs)

	// can't count on any ordering from map -> array
	for _, attr := range otelAttrs {
		key := string(attr.Key)
		switch key {
		case "test 1 - string":
			if attr.Value.GetStringValue() != testAttrs[key] {
				t.Errorf("expected value '%s' for key '%s' but got '%s'", testAttrs[key], key, attr.Value.GetStringValue())
			}
		case "test 2 - int64":
			if attr.Value.GetIntValue() != 111111111 {
				t.Errorf("expected value '%s' for key '%s' but got %d", testAttrs[key], key, attr.Value.GetIntValue())
			}
		case "test 3 - float":
			if attr.Value.GetDoubleValue() != 2.4391111 {
				t.Errorf("expected value '%s' for key '%s' but got %f", testAttrs[key], key, attr.Value.GetDoubleValue())
			}
		case "test 4 - bool, true":
			if attr.Value.GetBoolValue() != true {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
			}
		case "test 5 - bool, false":
			if attr.Value.GetBoolValue() != false {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
			}
		case "test 6 - bool, True":
			if attr.Value.GetBoolValue() != true {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
			}
		case "test 7 - bool, False":
			if attr.Value.GetBoolValue() != false {
				t.Errorf("expected value '%s' for key '%s' but got %t", testAttrs[key], key, attr.Value.GetBoolValue())
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

func TestOtelSpanStatus(t *testing.T) {

	for _, testcase := range []struct {
		name string
		want tracepb.Status_StatusCode
	}{
		{
			name: "unset",
			want: tracepb.Status_STATUS_CODE_UNSET,
		},
		{
			name: "ok",
			want: tracepb.Status_STATUS_CODE_OK,
		},
		{
			name: "error",
			want: tracepb.Status_STATUS_CODE_ERROR,
		},
		{
			name: "cromulent",
			want: tracepb.Status_STATUS_CODE_UNSET,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			out := otelSpanStatus(testcase.name)
			if out != testcase.want {
				t.Errorf("otelSpanStatus returned the wrong value, '%q', for '%s'", out, testcase.name)
			}
		})
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
			config = Config{Timeout: testcase.input}
			got := parseCliTimeout()
			if got != testcase.expected {
				ed := testcase.expected.String()
				gd := got.String()
				t.Errorf("duration string %q was expected to return %s but returned %s", config.Timeout, ed, gd)
			}
		})
	}
}

func TestFlattenStringMap(t *testing.T) {
	in := map[string]string{
		"sample1": "value1",
		"more":    "stuff",
		"getting": "bored",
		"okay":    "that's enough",
	}

	out := flattenStringMap(in, "{}")

	if out != "getting=bored,more=stuff,okay=that's enough,sample1=value1" {
		t.Fail()
	}
}

func TestParseCkvStringMap(t *testing.T) {
	expect := map[string]string{
		"sample1": "value1",
		"more":    "stuff",
		"getting": "bored",
		"okay":    "that's enough",
		"1":       "324",
	}

	got, err := parseCkvStringMap("1=324,getting=bored,more=stuff,okay=that's enough,sample1=value1")
	if err != nil {
		t.Errorf("error on valid input: %s", err)
	}

	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("maps didn't match (-want +got):\n%s", diff)
	}
}
