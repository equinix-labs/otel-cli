package otlpclient

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestConfig_ToStringMap(t *testing.T) {
	c := Config{}
	c.Headers = map[string]string{
		"123test": "deadbeefcafe",
	}

	fsm := c.ToStringMap()

	if _, ok := fsm["headers"]; !ok {
		t.Errorf("missing key 'headers' in returned string map: %q", fsm)
		t.Fail()
	}

	if fsm["headers"] != "123test=deadbeefcafe" {
		t.Errorf("expected header value not found in flattened string map: %q", fsm)
		t.Fail()
	}
}

func TestIsRecording(t *testing.T) {
	c := DefaultConfig()
	if c.IsRecording() {
		t.Fail()
	}
	c.Endpoint = "https://localhost:4318"

	if !c.IsRecording() {
		t.Fail()
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
			out, _ := DefaultConfig().parseTime(testcase.input, "test")
			if !out.Equal(testcase.want) {
				t.Errorf("got wrong time from parseTime: %s", out.Format(time.RFC3339Nano))
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
			config := DefaultConfig().WithTimeout(testcase.input)
			got := config.ParseCliTimeout()
			if got != testcase.expected {
				ed := testcase.expected.String()
				gd := got.String()
				t.Errorf("duration string %q was expected to return %s but returned %s", config.Timeout, ed, gd)
			}
		})
	}
}

func TestWithEndpoint(t *testing.T) {
	if DefaultConfig().WithEndpoint("foobar").Endpoint != "foobar" {
		t.Fail()
	}
}
func TestWithTracesEndpoint(t *testing.T) {
	if DefaultConfig().WithTracesEndpoint("foobar").TracesEndpoint != "foobar" {
		t.Fail()
	}
}
func TestWithTimeout(t *testing.T) {
	if DefaultConfig().WithTimeout("foobar").Timeout != "foobar" {
		t.Fail()
	}
}
func TestWithHeaders(t *testing.T) {
	attr := map[string]string{"foo": "bar"}
	c := DefaultConfig().WithHeaders(attr)
	if diff := cmp.Diff(attr, c.Headers); diff != "" {
		t.Errorf("Headers did not match (-want +got):\n%s", diff)
	}
}
func TestWithInsecure(t *testing.T) {
	if DefaultConfig().WithInsecure(true).Insecure != true {
		t.Fail()
	}
}
func TestWithBlocking(t *testing.T) {
	if DefaultConfig().WithBlocking(true).Blocking != true {
		t.Fail()
	}
}
func TestWithTlsNoVerify(t *testing.T) {
	if DefaultConfig().WithTlsNoVerify(true).TlsNoVerify != true {
		t.Fail()
	}
}
func TestWithTlsCACert(t *testing.T) {
	if DefaultConfig().WithTlsCACert("/a/b/c").TlsCACert != "/a/b/c" {
		t.Fail()
	}
}
func TestWithTlsClientKey(t *testing.T) {
	if DefaultConfig().WithTlsClientKey("/c/b/a").TlsClientKey != "/c/b/a" {
		t.Fail()
	}
}
func TestWithTlsClientCert(t *testing.T) {
	if DefaultConfig().WithTlsClientCert("/b/c/a").TlsClientCert != "/b/c/a" {
		t.Fail()
	}
}
func TestWithServiceName(t *testing.T) {
	if DefaultConfig().WithServiceName("foobar").ServiceName != "foobar" {
		t.Fail()
	}
}
func TestWithSpanName(t *testing.T) {
	if DefaultConfig().WithSpanName("foobar").SpanName != "foobar" {
		t.Fail()
	}
}
func TestWithKind(t *testing.T) {
	if DefaultConfig().WithKind("producer").Kind != "producer" {
		t.Fail()
	}
}
func TestWithAttributes(t *testing.T) {
	attr := map[string]string{"foo": "bar"}
	c := DefaultConfig().WithAttributes(attr)
	if diff := cmp.Diff(attr, c.Attributes); diff != "" {
		t.Errorf("Attributes did not match (-want +got):\n%s", diff)
	}
}

func TestWithStatusCode(t *testing.T) {
	if diff := cmp.Diff(DefaultConfig().WithStatusCode("unset").StatusCode, "unset"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(DefaultConfig().WithStatusCode("ok").StatusCode, "ok"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(DefaultConfig().WithStatusCode("error").StatusCode, "error"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestWithStatusDescription(t *testing.T) {
	if diff := cmp.Diff(DefaultConfig().WithStatusDescription("Set SCE To AUX").StatusDescription, "Set SCE To AUX"); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestWithTraceparentCarrierFile(t *testing.T) {
	if DefaultConfig().WithTraceparentCarrierFile("foobar").TraceparentCarrierFile != "foobar" {
		t.Fail()
	}
}
func TestWithTraceparentIgnoreEnv(t *testing.T) {
	if DefaultConfig().WithTraceparentIgnoreEnv(true).TraceparentIgnoreEnv != true {
		t.Fail()
	}
}
func TestWithTraceparentPrint(t *testing.T) {
	if DefaultConfig().WithTraceparentPrint(true).TraceparentPrint != true {
		t.Fail()
	}
}
func TestWithTraceparentPrintExport(t *testing.T) {
	if DefaultConfig().WithTraceparentPrintExport(true).TraceparentPrintExport != true {
		t.Fail()
	}
}
func TestWithTraceparentRequired(t *testing.T) {
	if DefaultConfig().WithTraceparentRequired(true).TraceparentRequired != true {
		t.Fail()
	}
}
func TestWithBackgroundParentPollMs(t *testing.T) {
	if DefaultConfig().WithBackgroundParentPollMs(1111).BackgroundParentPollMs != 1111 {
		t.Fail()
	}
}
func TestWithBackgroundSockdir(t *testing.T) {
	if DefaultConfig().WithBackgroundSockdir("foobar").BackgroundSockdir != "foobar" {
		t.Fail()
	}
}
func TestWithBackgroundWait(t *testing.T) {
	if DefaultConfig().WithBackgroundWait(true).BackgroundWait != true {
		t.Fail()
	}
}
func TestWithStatusCanaryCount(t *testing.T) {
	if DefaultConfig().WithStatusCanaryCount(1337).StatusCanaryCount != 1337 {
		t.Fail()
	}
}
func TestWithStatusCanaryInterval(t *testing.T) {
	if DefaultConfig().WithStatusCanaryInterval("1337ms").StatusCanaryInterval != "1337ms" {
		t.Fail()
	}
}
func TestWithSpanStartTime(t *testing.T) {
	if DefaultConfig().WithSpanStartTime("foobar").SpanStartTime != "foobar" {
		t.Fail()
	}
}
func TestWithSpanEndTime(t *testing.T) {
	if DefaultConfig().WithSpanEndTime("foobar").SpanEndTime != "foobar" {
		t.Fail()
	}
}
func TestWithEventName(t *testing.T) {
	if DefaultConfig().WithEventName("foobar").EventName != "foobar" {
		t.Fail()
	}
}
func TestWithEventTime(t *testing.T) {
	if DefaultConfig().WithEventTime("foobar").EventTime != "foobar" {
		t.Fail()
	}
}
func TestWithCfgFile(t *testing.T) {
	if DefaultConfig().WithCfgFile("foobar").CfgFile != "foobar" {
		t.Fail()
	}
}
func TestWithVerbose(t *testing.T) {
	if DefaultConfig().WithVerbose(true).Verbose != true {
		t.Fail()
	}
}
