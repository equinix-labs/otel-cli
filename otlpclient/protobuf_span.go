package otlpclient

// Implements just enough sugar on the OTel Protocol Buffers span definition
// to support otel-cli and no more.
//
// otel-cli does a few things that are awkward via the opentelemetry-go APIs
// which are restricted for good reasons.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/equinix-labs/otel-cli/w3c/traceparent"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

var emptyTraceId = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
var emptySpanId = []byte{0, 0, 0, 0, 0, 0, 0, 0}

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
	span.Attributes = StringMapAttrsToProtobuf(c.Attributes)

	now := time.Now()
	if c.SpanStartTime != "" {
		st := c.ParsedSpanStartTime()
		span.StartTimeUnixNano = uint64(st.UnixNano())
	} else {
		span.StartTimeUnixNano = uint64(now.UnixNano())
	}

	if c.SpanEndTime != "" {
		et := c.ParsedSpanEndTime()
		span.EndTimeUnixNano = uint64(et.UnixNano())
	} else {
		span.EndTimeUnixNano = uint64(now.UnixNano())
	}

	if c.IsRecording() {
		tp := LoadTraceparent(c, span)
		if tp.Initialized {
			span.TraceId = tp.TraceId
			span.ParentSpanId = tp.SpanId
		}
	} else {
		span.TraceId = emptyTraceId
		span.SpanId = emptySpanId
	}

	// --force-trace-id and --force-span-id let the user set their own trace & span ids
	// these work in non-recording mode and will stomp trace id from the traceparent
	var err error
	if c.ForceTraceId != "" {
		span.TraceId, err = parseHex(c.ForceTraceId, 16)
		c.SoftFailIfErr(err)
	}
	if c.ForceSpanId != "" {
		span.SpanId, err = parseHex(c.ForceSpanId, 8)
		c.SoftFailIfErr(err)
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
			c.SoftFail("Failed to generate random data for trace id: %s", err)
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
			c.SoftFail("Failed to generate random data for span id: %s", err)
		}
		return buf
	} else {
		return emptySpanId
	}
}

// parseHex parses hex into a []byte of length provided. Errors if the input is
// not valid hex or the converted hex is not the right number of bytes.
func parseHex(in string, expectedLen int) ([]byte, error) {
	out, err := hex.DecodeString(in)
	if err != nil {
		return nil, fmt.Errorf("error parsing hex string %q: %w", in, err)
	}
	if len(out) != expectedLen {
		return nil, fmt.Errorf("hex string %q is the wrong length, expected %d bytes but got %d", in, expectedLen, len(out))
	}
	return out, nil
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

// StringMapAttrsToProtobuf takes a map of string:string, such as that from --attrs
// and returns them in an []*commonpb.KeyValue
func StringMapAttrsToProtobuf(attributes map[string]string) []*commonpb.KeyValue {
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

// SpanAttributesToStringMap converts the span's attributes to a string map.
// Only used by tests for now.
func SpanAttributesToStringMap(span *tracepb.Span) map[string]string {
	out := make(map[string]string)
	for _, attr := range span.Attributes {
		out[attr.Key] = AttrValueToString(attr)
	}
	return out
}

// ResourceAttributesToStringMap converts the ResourceSpan's resource attributes to a string map.
// Only used by tests for now.
func ResourceAttributesToStringMap(rss *tracepb.ResourceSpans) map[string]string {
	out := make(map[string]string)
	for _, attr := range rss.Resource.Attributes {
		out[attr.Key] = AttrValueToString(attr)
	}
	return out
}

// AttrValueToString coverts a commonpb.KeyValue attribute to a string.
// Only used by tests for now.
func AttrValueToString(attr *commonpb.KeyValue) string {
	v := attr.GetValue()
	v.GetIntValue()
	if _, ok := v.Value.(*commonpb.AnyValue_StringValue); ok {
		return v.GetStringValue()
	} else if _, ok := v.Value.(*commonpb.AnyValue_IntValue); ok {
		return strconv.FormatInt(v.GetIntValue(), 10)
	} else if _, ok := v.Value.(*commonpb.AnyValue_DoubleValue); ok {
		return strconv.FormatFloat(v.GetDoubleValue(), byte('f'), -1, 64)
	}

	return ""
}

// SpanToStringMap converts a span with some extra data into a stringmap.
// Only used by tests for now.
func SpanToStringMap(span *tracepb.Span, rss *tracepb.ResourceSpans) map[string]string {
	if span == nil {
		return map[string]string{}
	}
	return map[string]string{
		"trace_id":           hex.EncodeToString(span.GetTraceId()),
		"span_id":            hex.EncodeToString(span.GetSpanId()),
		"parent":             hex.EncodeToString(span.GetParentSpanId()),
		"name":               span.Name,
		"kind":               SpanKindIntToString(span.GetKind()),
		"start":              strconv.FormatUint(span.StartTimeUnixNano, 10),
		"end":                strconv.FormatUint(span.EndTimeUnixNano, 10),
		"attributes":         flattenStringMap(SpanAttributesToStringMap(span), "{}"),
		"service_attributes": flattenStringMap(ResourceAttributesToStringMap(rss), "{}"),
		"status_code":        strconv.FormatInt(int64(span.Status.GetCode()), 10),
		"status_description": span.Status.GetMessage(),
	}
}

// TraceparentFromProtobufSpan builds a Traceparent struct from the provided span.
func TraceparentFromProtobufSpan(c Config, span *tracepb.Span) traceparent.Traceparent {
	return traceparent.Traceparent{
		Version:     0,
		TraceId:     span.TraceId,
		SpanId:      span.SpanId,
		Sampling:    c.IsRecording(),
		Initialized: true,
	}
}

// PropagateTraceparent saves the traceparent to file if necessary, then prints
// span info to the console according to command-line args.
func PropagateTraceparent(c Config, span *tracepb.Span, target io.Writer) {
	var tp traceparent.Traceparent
	if c.IsRecording() {
		tp = TraceparentFromProtobufSpan(c, span)
	} else {
		// when in non-recording mode, and there is a TP available, propagate that
		tp = LoadTraceparent(c, span)
	}

	if c.TraceparentCarrierFile != "" {
		err := tp.SaveToFile(c.TraceparentCarrierFile, c.TraceparentPrintExport)
		c.SoftFailIfErr(err)
	}

	if c.TraceparentPrint {
		tp.Fprint(target, c.TraceparentPrintExport)
	}
}

// LoadTraceparent follows otel-cli's loading rules, start with envvar then file.
// If both are set, the file will override env.
// When in non-recording mode, the previous traceparent will be returned if it's
// available, otherwise, a zero-valued traceparent is returned.
func LoadTraceparent(c Config, span *tracepb.Span) traceparent.Traceparent {
	tp := traceparent.Traceparent{
		Version:     0,
		TraceId:     emptyTraceId,
		SpanId:      emptySpanId,
		Sampling:    false,
		Initialized: true,
	}

	if !c.TraceparentIgnoreEnv {
		var err error
		tp, err = traceparent.LoadFromEnv()
		if err != nil {
			Diag.Error = err.Error()
		}
	}

	if c.TraceparentCarrierFile != "" {
		fileTp, err := traceparent.LoadFromFile(c.TraceparentCarrierFile)
		if err != nil {
			Diag.Error = err.Error()
		} else if fileTp.Initialized {
			tp = fileTp
		}
	}

	if c.TraceparentRequired {
		if tp.Initialized {
			return tp
		} else {
			c.SoftFail("failed to find a valid traceparent carrier in either environment for file '%s' while it's required by --tp-required", c.TraceparentCarrierFile)
		}
	}

	return tp
}
