package otelcli

// Implements just enough sugar on the OTel Protocol Buffers span definition
// to support otel-cli and no more.
//
// otel-cli does a few things that are awkward via the opentelemetry-go APIs
// which are restricted for good reasons.

import (
	"crypto/rand"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// NewProtobufSpan returns an initialized OpenTelemetry protobuf Span.
func NewProtobufSpan() tracepb.Span {
	now := time.Now()
	span := tracepb.Span{
		TraceId:                generateTraceId(),
		SpanId:                 generateSpanId(),
		TraceState:             "",
		ParentSpanId:           []byte{},
		Name:                   "BUG IN OTEL-CLI: unset",
		Kind:                   tracepb.Span_SPAN_KIND_CLIENT,
		StartTimeUnixNano:      uint64(now.UnixNano()),
		EndTimeUnixNano:        uint64(now.UnixNano()),
		Attributes:             []*commonpb.KeyValue{},
		DroppedAttributesCount: 0,
		Events:                 []*tracepb.Span_Event{},
		DroppedEventsCount:     0,
		Links:                  []*tracepb.Span_Link{},
		DroppedLinksCount:      0,
		Status: &tracepb.Status{
			Code:    tracepb.Status_STATUS_CODE_UNSET,
			Message: "",
		},
	}

	return span
}

func NewProtobufSpanEvent() tracepb.Span_Event {
	now := time.Now()
	return tracepb.Span_Event{
		TimeUnixNano: uint64(now.UnixNano()),
		Attributes:   []*v1.KeyValue{},
	}
}

func NewProtobufSpanWithConfig(c Config) tracepb.Span {
	span := NewProtobufSpan()
	span.Name = c.SpanName
	span.Kind = SpanKindStringToInt(c.Kind)
	span.Attributes = cliAttrsToOtelPb(c.Attributes)

	now := time.Now()
	if c.SpanStartTime != "" {
		st := parseTime(c.SpanStartTime, "start")
		span.StartTimeUnixNano = uint64(st.UnixNano())
	} else {
		span.StartTimeUnixNano = uint64(now.UnixNano())
	}

	if c.SpanEndTime != "" {
		et := parseTime(c.SpanEndTime, "end")
		span.EndTimeUnixNano = uint64(et.UnixNano())
	} else {
		span.EndTimeUnixNano = uint64(now.UnixNano())
	}

	// TODO: switch to setting the parent span
	if tp := loadTraceparent(c.TraceparentCarrierFile); tp.initialized {
		span.ParentSpanId = tp.SpanId
	}

	// Only set status description when an error status.
	// https://github.com/open-telemetry/opentelemetry-specification/blob/480a19d702470563d32a870932be5ddae798079c/specification/trace/api.md#set-status
	statusCode := otelSpanStatus(c.StatusCode)
	if statusCode == tracepb.Status_STATUS_CODE_ERROR {
		span.Status.Code = statusCode
		span.Status.Message = c.StatusDescription
	}

	return span
}

// generateTraceId generates a random 16 byte trace id
func generateTraceId() []byte {
	if config.IsRecording() {
		buf := make([]byte, 16)
		_, err := rand.Read(buf)
		if err != nil {
			softFail("Failed to generate random data for trace id: %s", err)
		}
		return buf
	} else {
		return emptyTraceId
	}
}

// generateSpanId generates a random 8 byte span id
func generateSpanId() []byte {
	if config.IsRecording() {
		buf := make([]byte, 8)
		_, err := rand.Read(buf)
		if err != nil {
			softFail("Failed to generate random data for span id: %s", err)
		}
		return buf
	} else {
		return emptySpanId
	}
}

// SpanKindIntToString takes an integer/constant protobuf span kind value
// and returns the string representation used in otel-cli.
func SpanKindIntToString(kind tracepb.Span_SpanKind) string {
	switch kind {
	case tracepb.Span_SPAN_KIND_CLIENT:
		return "client"
	case tracepb.Span_SPAN_KIND_SERVER:
		return "server"
	case tracepb.Span_SPAN_KIND_PRODUCER:
		return "producer"
	case tracepb.Span_SPAN_KIND_CONSUMER:
		return "consumer"
	case tracepb.Span_SPAN_KIND_INTERNAL:
		return "internal"
	default:
		return "unspecified"
	}
}

// SpanKindIntToString takes a string representation of a span kind and
// returns the OTel protobuf integer/constant.
func SpanKindStringToInt(kind string) tracepb.Span_SpanKind {
	switch kind {
	case "client":
		return tracepb.Span_SPAN_KIND_CLIENT
	case "server":
		return tracepb.Span_SPAN_KIND_SERVER
	case "producer":
		return tracepb.Span_SPAN_KIND_PRODUCER
	case "consumer":
		return tracepb.Span_SPAN_KIND_CONSUMER
	case "internal":
		return tracepb.Span_SPAN_KIND_INTERNAL
	default:
		return tracepb.Span_SPAN_KIND_UNSPECIFIED
	}
}
