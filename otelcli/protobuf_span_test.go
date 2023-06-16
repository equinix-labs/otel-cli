package otelcli

import (
	"bytes"
	"strconv"
	"testing"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestNewProtobufSpan(t *testing.T) {
	span := NewProtobufSpan()

	// no tmuch to test since it's just an initialized struct
	if len(span.Name) < 1 {
		t.Error("span name should default to non-empty string")
	}

	if span.ParentSpanId == nil {
		t.Error("span parent must not be nil")
	}

	if span.Attributes == nil {
		t.Error("span attributes must not be nil")
	}

	if span.Events == nil {
		t.Error("span events must not be nil")
	}

	if span.Links == nil {
		t.Error("span links must not be nil")
	}
}

func TestNewProtobufSpanEvent(t *testing.T) {
	evt := NewProtobufSpanEvent()

	// similar to span above, just run the code and make sure
	// it doesn't blow up
	if evt.Attributes == nil {
		t.Error("span event attributes must not be nil")
	}
}

func TestNewProtobufSpanWithConfig(t *testing.T) {
	c := DefaultConfig().WithSpanName("test span 123")
	span := NewProtobufSpanWithConfig(c)

	if span.Name != "test span 123" {
		t.Error("span event attributes must not be nil")
	}
}

func TestGenerateTraceId(t *testing.T) {
	c := DefaultConfig()
	// non-recording
	tid := generateTraceId(c)

	if !bytes.Equal(tid, emptyTraceId) {
		t.Error("generated trace id must always be zeroes in non-recording mode")
	}

	tid = generateTraceId(c.WithEndpoint("localhost:4317"))
	if bytes.Equal(tid, emptyTraceId) {
		t.Error("generated trace id must not be zeroes in recording mode")
	}

	if len(tid) != 16 {
		t.Error("generated trace id must be 16 bytes")
	}
}

func TestGenerateSpanId(t *testing.T) {
	c := DefaultConfig()
	// non-recording
	sid := generateSpanId(c)

	if !bytes.Equal(sid, emptySpanId) {
		t.Error("generated span id must always be zeroes in non-recording mode")
	}

	sid = generateSpanId(c.WithEndpoint("localhost:4317"))
	if bytes.Equal(sid, emptySpanId) {
		t.Error("generated span id must not be zeroes in recording mode")
	}

	if len(sid) != 8 {
		t.Error("generated span id must be 8 bytes")
	}
}

func TestSpanKindStringToInt(t *testing.T) {
	for _, testcase := range []struct {
		name string
		want tracepb.Span_SpanKind
	}{
		{
			name: "client",
			want: tracepb.Span_SPAN_KIND_CLIENT,
		},
		{
			name: "server",
			want: tracepb.Span_SPAN_KIND_SERVER,
		},
		{
			name: "producer",
			want: tracepb.Span_SPAN_KIND_PRODUCER,
		},
		{
			name: "consumer",
			want: tracepb.Span_SPAN_KIND_CONSUMER,
		},
		{
			name: "internal",
			want: tracepb.Span_SPAN_KIND_INTERNAL,
		},
		{
			name: "unspecified",
			want: tracepb.Span_SPAN_KIND_UNSPECIFIED,
		},
		{
			name: "speledwrong",
			want: tracepb.Span_SPAN_KIND_UNSPECIFIED,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			out := SpanKindStringToInt(testcase.name)
			if out != testcase.want {
				t.Errorf("returned the wrong value, '%q', for '%s'", out, testcase.name)
			}
		})
	}
}

func TestSpanKindIntToString(t *testing.T) {
	for _, testcase := range []struct {
		want string
		have tracepb.Span_SpanKind
	}{
		{
			have: tracepb.Span_SPAN_KIND_CLIENT,
			want: "client",
		},
		{
			have: tracepb.Span_SPAN_KIND_SERVER,
			want: "server",
		},
		{
			have: tracepb.Span_SPAN_KIND_PRODUCER,
			want: "producer",
		},
		{
			have: tracepb.Span_SPAN_KIND_CONSUMER,
			want: "consumer",
		},
		{
			have: tracepb.Span_SPAN_KIND_INTERNAL,
			want: "internal",
		},
		{
			have: tracepb.Span_SPAN_KIND_UNSPECIFIED,
			want: "unspecified",
		},
	} {
		name := strconv.Itoa(int(testcase.have)) + " => " + testcase.want
		t.Run(name, func(t *testing.T) {
			out := SpanKindIntToString(testcase.have)
			if out != testcase.want {
				t.Errorf("returned the wrong value, '%q', for %d", out, int(testcase.have))
			}
		})
	}
}

func TestSpanStatusStringToInt(t *testing.T) {

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
			out := SpanStatusStringToInt(testcase.name)
			if out != testcase.want {
				t.Errorf("otelSpanStatus returned the wrong value, '%q', for '%s'", out, testcase.name)
			}
		})
	}
}

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

	otelAttrs := StringMapAttrsToProtobuf(testAttrs)

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
