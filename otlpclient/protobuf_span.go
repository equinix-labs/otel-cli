package otlpclient

// Implements just enough sugar on the OTel Protocol Buffers span definition
// to support otel-cli and no more.
//
// otel-cli does a few things that are awkward via the opentelemetry-go APIs
// which are restricted for good reasons.

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tobert/otel-cli/w3c/traceparent"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type SpanConfig interface {
}

// NewProtobufSpan returns an initialized OpenTelemetry protobuf Span.
func NewProtobufSpan() *tracepb.Span {
	now := time.Now()
	span := tracepb.Span{
		TraceId:                GetEmptyTraceId(),
		SpanId:                 GetEmptySpanId(),
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

// SetSpanStatus checks for status code error in the config and sets the
// span's 2 values as appropriate.
// Only set status description when an error status.
// https://github.com/open-telemetry/opentelemetry-specification/blob/480a19d702470563d32a870932be5ddae798079c/specification/trace/api.md#set-status
func SetSpanStatus(span *tracepb.Span, status string, message string) {
	statusCode := SpanStatusStringToInt(status)
	if statusCode != tracepb.Status_STATUS_CODE_UNSET {
		span.Status.Code = statusCode
		span.Status.Message = message
	}
}

// GetEmptyTraceId returns a 16-byte trace id that's all zeroes.
func GetEmptyTraceId() []byte {
	return []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
}

// GetEmptySpanId returns an 8-byte span id that's all zeroes.
func GetEmptySpanId() []byte {
	return []byte{0, 0, 0, 0, 0, 0, 0, 0}
}

// GenerateTraceId generates a random 16 byte trace id
func GenerateTraceId() []byte {
	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		// should never happen, crash when it does
		panic("failed to generate random data for trace id: " + err.Error())
	}
	return buf
}

// GenerateSpanId generates a random 8 byte span id
func GenerateSpanId() []byte {
	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		// should never happen, crash when it does
		panic("failed to generate random data for span id: " + err.Error())
	}
	return buf
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
func SpanAttributesToStringMap(span *tracepb.Span) map[string]string {
	out := make(map[string]string)
	for _, attr := range span.Attributes {
		out[attr.Key] = AnyValueToString(attr.GetValue())
	}
	return out
}

// ResourceAttributesToStringMap converts the ResourceSpan's resource attributes to a string map.
// Only used by tests for now.
func ResourceAttributesToStringMap(rss *tracepb.ResourceSpans) map[string]string {
	if rss == nil {
		return map[string]string{}
	}

	out := make(map[string]string)
	for _, attr := range rss.Resource.Attributes {
		out[attr.Key] = AnyValueToString(attr.GetValue())
	}
	return out
}

// AnyValueToString coverts a commonpb.KeyValue attribute to a string.
func AnyValueToString(v *commonpb.AnyValue) string {
	if _, ok := v.Value.(*commonpb.AnyValue_StringValue); ok {
		return v.GetStringValue()
	} else if _, ok := v.Value.(*commonpb.AnyValue_IntValue); ok {
		return strconv.FormatInt(v.GetIntValue(), 10)
	} else if _, ok := v.Value.(*commonpb.AnyValue_DoubleValue); ok {
		return strconv.FormatFloat(v.GetDoubleValue(), byte('f'), -1, 64)
	} else if _, ok := v.Value.(*commonpb.AnyValue_ArrayValue); ok {
		values := v.GetArrayValue().GetValues()
		strValues := make([]string, len(values))
		for i, v := range values {
			// recursively convert to string
			strValues[i] = AnyValueToString(v)
		}
		return strings.Join(strValues, ",")
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
		"parent_span_id":     hex.EncodeToString(span.GetParentSpanId()),
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
func TraceparentFromProtobufSpan(span *tracepb.Span, recording bool) traceparent.Traceparent {
	return traceparent.Traceparent{
		Version:     0,
		TraceId:     span.TraceId,
		SpanId:      span.SpanId,
		Sampling:    recording,
		Initialized: true,
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
