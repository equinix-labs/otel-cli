package otelcli

// Implements just enough sugar on the OTel Protocol Buffers span definition
// to support otel-cli and no more.
//
// otel-cli does a few things that are awkward via the opentelemetry-go APIs
// which are restricted for good reasons.

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// NewProtobufSpan returns an initialized OpenTelemetry protobuf Span.
func NewProtobufSpan() *tracepb.Span {
	now := time.Now()
	span := tracepb.Span{
		TraceId:                emptyTraceId,
		SpanId:                 emptySpanId,
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

	return &span
}

// NewProtobufSpanEvent creates a new span event protobuf struct with reasonable
// defaults and returns it.
func NewProtobufSpanEvent() *tracepb.Span_Event {
	now := time.Now()
	return &tracepb.Span_Event{
		TimeUnixNano: uint64(now.UnixNano()),
		Attributes:   []*commonpb.KeyValue{},
	}
}

// NewProtobufSpanWithConfig creates a new span and populates it with information
// from the provided config struct.
func NewProtobufSpanWithConfig(c Config) *tracepb.Span {
	span := NewProtobufSpan()
	span.TraceId = generateTraceId(c)
	span.SpanId = generateSpanId(c)
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

	if config.IsRecording() {
		if tp := loadTraceparent(c.TraceparentCarrierFile); tp.initialized {
			span.TraceId = tp.TraceId
			span.ParentSpanId = tp.SpanId
		}

	} else {
		span.TraceId = emptyTraceId
		span.SpanId = emptySpanId
	}

	// --force-trace-id and --force-span-id let the user set their own trace & span ids
	// these work in non-recording mode and will stomp trace id from the traceparent
	if config.ForceTraceId != "" {
		span.TraceId = parseHex(config.ForceTraceId, 16)
	}
	if config.ForceSpanId != "" {
		span.SpanId = parseHex(config.ForceSpanId, 8)
	}

	SetSpanStatus(span, c)

	return span
}

// SetSpanStatus checks for status code error in the config and sets the
// span's 2 values as appropriate.
// Only set status description when an error status.
// https://github.com/open-telemetry/opentelemetry-specification/blob/480a19d702470563d32a870932be5ddae798079c/specification/trace/api.md#set-status
func SetSpanStatus(span *tracepb.Span, c Config) {
	statusCode := SpanStatusStringToInt(c.StatusCode)
	if statusCode != tracepb.Status_STATUS_CODE_UNSET {
		span.Status.Code = statusCode
		span.Status.Message = c.StatusDescription
	}
}

// generateTraceId generates a random 16 byte trace id
func generateTraceId(c Config) []byte {
	if c.IsRecording() {
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
func generateSpanId(c Config) []byte {
	if c.IsRecording() {
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

// parseHex parses hex into a []byte of length provided. Errors if the input is
// not valid hex or the converted hex is not the right number of bytes.
func parseHex(in string, expectedLen int) []byte {
	out, err := hex.DecodeString(in)
	if err != nil {
		softFail("error parsing hex string %q: %s", in, err)
	}
	if len(out) != expectedLen {
		softFail("hex string %q is the wrong length, expected %d bytes but got %d", in, expectedLen, len(out))
	}
	return out
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

// SpanStatusStringToInt takes a supported string span status and returns the otel
// constant for it. Returns default of Unset on no match.
func SpanStatusStringToInt(status string) tracepb.Status_StatusCode {
	switch status {
	case "unset":
		return tracepb.Status_STATUS_CODE_UNSET
	case "ok":
		return tracepb.Status_STATUS_CODE_OK
	case "error":
		return tracepb.Status_STATUS_CODE_ERROR
	default:
		return tracepb.Status_STATUS_CODE_UNSET
	}
}
